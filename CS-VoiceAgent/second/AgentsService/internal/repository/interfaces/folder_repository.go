package interfaces

import (
	"context"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// FolderRepository — контракт доступа к папкам.
// Usecase-слой работает только через этот интерфейс и не знает о MongoDB.
type FolderRepository interface {
	List(ctx context.Context, workspaceID string) ([]models.Folder, error)
	GetByID(ctx context.Context, workspaceID, folderID string) (models.Folder, bool, error)
	Create(ctx context.Context, folder models.Folder) error
	UpdateName(ctx context.Context, workspaceID, folderID, name string, updatedAt int64) (models.Folder, bool, error)
	Delete(ctx context.Context, workspaceID, folderID string) (bool, error)
	Count(ctx context.Context, workspaceID string) (int64, error)
}

