package usecase

import (
	"context"
	"strings"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/ids"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	"github.com/cybrix-solutions/agents-service/internal/domain/validation"
)

// FolderUsecase содержит бизнес-логику папок.
type FolderUsecase struct {
	deps Deps
}

func NewFolderUsecase(deps Deps) *FolderUsecase {
	return &FolderUsecase{deps: deps}
}

func (u *FolderUsecase) List(ctx context.Context, workspaceID string) ([]models.Folder, error) {
	return u.deps.Folders.List(ctx, workspaceID)
}

func (u *FolderUsecase) Create(ctx context.Context, workspaceID string, name string) (models.Folder, error) {
	if err := validation.ValidateFolderName(name); err != nil {
		return models.Folder{}, err
	}

	// Бизнес-лимит из ТЗ: максимум 50 обычных папок (виртуальная Template Agents не считается).
	cnt, err := u.deps.Folders.Count(ctx, workspaceID)
	if err != nil {
		return models.Folder{}, derr.NewInternal("mongo_error", "failed to count folders")
	}
	if cnt >= 50 {
		return models.Folder{}, derr.NewBusiness("folder_limit_exceeded", "folder limit exceeded")
	}

	idv, err := ids.New("folder_", 12)
	if err != nil {
		return models.Folder{}, derr.NewInternal("id_generation_failed", "failed to generate folder id")
	}
	nowMs := u.deps.now().UnixMilli()
	f := models.Folder{
		FolderID:    idv,
		WorkspaceID: workspaceID,
		Name:        strings.TrimSpace(name),
		CreatedAt:   nowMs,
		UpdatedAt:   nowMs,
	}
	if err := u.deps.Folders.Create(ctx, f); err != nil {
		return models.Folder{}, derr.NewInternal("mongo_error", "failed to create folder")
	}
	return f, nil
}

func (u *FolderUsecase) Rename(ctx context.Context, workspaceID, folderID, name string) (models.Folder, error) {
	// Если клиент пытается адресовать виртуальную папку напрямую, возвращаем бизнес-ошибку из ТЗ.
	if strings.TrimSpace(folderID) == "Template Agents" {
		return models.Folder{}, derr.NewBusiness("template_folder_is_virtual", "Template Agents is a virtual folder and cannot be modified")
	}
	if err := validation.ValidateFolderName(name); err != nil {
		return models.Folder{}, err
	}
	nowMs := u.deps.now().UnixMilli()
	updated, ok, err := u.deps.Folders.UpdateName(ctx, workspaceID, folderID, strings.TrimSpace(name), nowMs)
	if err != nil {
		return models.Folder{}, derr.NewInternal("mongo_error", "failed to update folder")
	}
	if !ok {
		return models.Folder{}, derr.NewNotFound("folder_not_found", "folder not found")
	}
	return updated, nil
}

func (u *FolderUsecase) Delete(ctx context.Context, workspaceID, folderID string) error {
	if strings.TrimSpace(folderID) == "Template Agents" {
		return derr.NewBusiness("template_folder_is_virtual", "Template Agents is a virtual folder and cannot be deleted")
	}
	nowMs := u.deps.now().UnixMilli()

	// При удалении папки нужно атомарно:
	// - удалить документ Folder
	// - сбросить folder_id у всех агентов этой папки
	if u.deps.Tx == nil {
		return derr.NewInternal("tx_not_configured", "transactions are not configured")
	}
	if err := u.deps.Tx.WithTransaction(ctx, func(txCtx context.Context) error {
		ok, err := u.deps.Folders.Delete(txCtx, workspaceID, folderID)
		if err != nil {
			return derr.NewInternal("mongo_error", "failed to delete folder")
		}
		if !ok {
			return derr.NewNotFound("folder_not_found", "folder not found")
		}
		if err := u.deps.Agents.DetachFromFolder(txCtx, workspaceID, folderID, nowMs); err != nil {
			return derr.NewInternal("mongo_error", "failed to detach agents from folder")
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

