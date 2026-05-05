package usecase

import (
	"context"
	"strings"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/ids"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	"github.com/cybrix-solutions/agents-service/internal/domain/validation"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	"github.com/cybrix-solutions/agents-service/internal/templates"
)

// AgentUsecase содержит бизнес-логику агентов.
type AgentUsecase struct {
	deps Deps
}

func NewAgentUsecase(deps Deps) *AgentUsecase {
	return &AgentUsecase{deps: deps}
}

type CreateAgentInput struct {
	// Поля top-level; если nil — используется backend default.
	Name      *string
	FolderID  **string // nil=не указано; &nil=явный null; &("id")=установить
	Channel   *string
	VoiceID   *string
	Language  *string
	TTS       *models.TTSConfig
	STT       *models.STTConfig

	InterruptionSensitivity *float64
	MaxCallDurationMS       *int64
	NormalizeForSpeech      *bool
	AllowUserDTMF           *bool
	UserDTMFOptions         *any
	HandbookConfig          *models.HandbookConfig
	PIIConfig               *models.PIIConfig
	DataStorageSetting      *string
}

func (u *AgentUsecase) ListLight(ctx context.Context, workspaceID string, q repo.ListAgentsQuery) ([]repo.LightAgent, int64, error) {
	if q.Limit > 100 {
		return nil, 0, derr.NewValidation("limit", "max_value_exceeded", "limit must be <= 100", nil)
	}
	if q.Page < 0 {
		return nil, 0, derr.NewValidation("page", "out_of_range", "page must be >= 1", nil)
	}
	return u.deps.Agents.ListLight(ctx, workspaceID, q)
}

func (u *AgentUsecase) Get(ctx context.Context, workspaceID, agentID string) (models.Agent, error) {
	a, ok, err := u.deps.Agents.GetByID(ctx, workspaceID, agentID)
	if err != nil {
		return models.Agent{}, derr.NewInternal("mongo_error", "failed to get agent")
	}
	if !ok {
		return models.Agent{}, derr.NewNotFound("agent_not_found", "agent not found")
	}
	return a, nil
}

func (u *AgentUsecase) Create(ctx context.Context, workspaceID string, in CreateAgentInput) (models.Agent, models.ConversationFlow, error) {
	// Бизнес-лимит из ТЗ: максимум 100 агентов на workspace.
	cnt, err := u.deps.Agents.Count(ctx, workspaceID)
	if err != nil {
		return models.Agent{}, models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to count agents")
	}
	if cnt >= 100 {
		return models.Agent{}, models.ConversationFlow{}, derr.NewBusiness("agent_limit_exceeded", "agent limit exceeded")
	}

	now := u.deps.now()

	agentID, err := ids.New("agent_", 14)
	if err != nil {
		return models.Agent{}, models.ConversationFlow{}, derr.NewInternal("id_generation_failed", "failed to generate agent id")
	}
	flowID, err := ids.New("conversation_flow_", 10)
	if err != nil {
		return models.Agent{}, models.ConversationFlow{}, derr.NewInternal("id_generation_failed", "failed to generate conversation flow id")
	}

	// Дефолты + overrides из POST body.
	a := templates.DefaultAgentTemplate(now)
	a.AgentID = agentID
	a.WorkspaceID = workspaceID
	if in.Name != nil {
		a.Name = strings.TrimSpace(*in.Name)
	}
	if in.Channel != nil {
		a.Channel = *in.Channel
	}
	if in.VoiceID != nil {
		a.VoiceID = *in.VoiceID
	}
	if in.Language != nil {
		a.Language = *in.Language
	}
	if in.TTS != nil {
		a.TTS = *in.TTS
	}
	if in.STT != nil {
		a.STT = *in.STT
	}
	if in.InterruptionSensitivity != nil {
		a.InterruptionSensitivity = *in.InterruptionSensitivity
	}
	if in.MaxCallDurationMS != nil {
		a.MaxCallDurationMS = *in.MaxCallDurationMS
	}
	if in.NormalizeForSpeech != nil {
		a.NormalizeForSpeech = *in.NormalizeForSpeech
	}
	if in.AllowUserDTMF != nil {
		a.AllowUserDTMF = *in.AllowUserDTMF
	}
	if in.UserDTMFOptions != nil {
		a.UserDTMFOptions = *in.UserDTMFOptions
	}
	if in.HandbookConfig != nil {
		a.HandbookConfig = *in.HandbookConfig
	}
	if in.PIIConfig != nil {
		a.PIIConfig = *in.PIIConfig
	}
	if in.DataStorageSetting != nil {
		a.DataStorageSetting = *in.DataStorageSetting
	}
	if in.FolderID != nil {
		a.FolderID = *in.FolderID
	}

	// Workflow v0
	cf := templates.DefaultConversationFlowV0Template(now)
	cf.ConversationFlowID = flowID
	cf.AgentID = agentID
	cf.WorkspaceID = workspaceID

	// Создание агента обязано сразу опубликовать v0:
	a.ResponseEngine.ConversationFlowID = flowID
	a.ResponseEngine.Version = 0

	// Валидация итоговых моделей (уже с дефолтами и overrides).
	if err := validation.ValidateAgentForCreate(a); err != nil {
		return models.Agent{}, models.ConversationFlow{}, err
	}
	if err := validation.ValidateConversationFlow(cf); err != nil {
		return models.Agent{}, models.ConversationFlow{}, err
	}

	// Если указали folder_id — проверим существование папки.
	if a.FolderID != nil && strings.TrimSpace(*a.FolderID) != "" {
		_, ok, err := u.deps.Folders.GetByID(ctx, workspaceID, *a.FolderID)
		if err != nil {
			return models.Agent{}, models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to check folder")
		}
		if !ok {
			return models.Agent{}, models.ConversationFlow{}, derr.NewNotFound("folder_not_found", "folder not found")
		}
	}

	if u.deps.Tx == nil {
		return models.Agent{}, models.ConversationFlow{}, derr.NewInternal("tx_not_configured", "transactions are not configured")
	}

	nowMs := now.UnixMilli()
	pw := models.PublishedWorkflow{
		WorkspaceID:        workspaceID,
		AgentID:            agentID,
		ConversationFlowID: flowID,
		Version:            0,
		PublishedAt:        nowMs,
		CreatedAt:          nowMs,
		UpdatedAt:          nowMs,
	}

	if err := u.deps.Tx.WithTransaction(ctx, func(txCtx context.Context) error {
		if err := u.deps.Agents.Create(txCtx, a); err != nil {
			return derr.NewInternal("mongo_error", "failed to create agent")
		}
		if err := u.deps.ConversationFlows.CreateVersion(txCtx, cf); err != nil {
			return derr.NewInternal("mongo_error", "failed to create workflow")
		}
		if err := u.deps.Published.Upsert(txCtx, pw); err != nil {
			return derr.NewInternal("mongo_error", "failed to publish workflow")
		}
		return nil
	}); err != nil {
		return models.Agent{}, models.ConversationFlow{}, err
	}

	return a, cf, nil
}

type UpdateAgentInput struct {
	Patch repo.AgentPatch
	// В usecase мы дополнительно проверяем существование folder_id (если он меняется).
	NewFolderID **string
}

func (u *AgentUsecase) Update(ctx context.Context, workspaceID, agentID string, in UpdateAgentInput) (models.Agent, error) {
	// Если folder_id меняется на конкретное значение — проверяем существование папки.
	if in.NewFolderID != nil && *in.NewFolderID != nil && strings.TrimSpace(**in.NewFolderID) != "" {
		_, ok, err := u.deps.Folders.GetByID(ctx, workspaceID, **in.NewFolderID)
		if err != nil {
			return models.Agent{}, derr.NewInternal("mongo_error", "failed to check folder")
		}
		if !ok {
			return models.Agent{}, derr.NewNotFound("folder_not_found", "folder not found")
		}
	}

	updated, ok, err := u.deps.Agents.Update(ctx, workspaceID, agentID, in.Patch)
	if err != nil {
		return models.Agent{}, derr.NewInternal("mongo_error", "failed to update agent")
	}
	if !ok {
		return models.Agent{}, derr.NewNotFound("agent_not_found", "agent not found")
	}

	// Валидация уже обновлённой модели.
	if err := validation.ValidateAgentForUpdate(updated); err != nil {
		return models.Agent{}, err
	}
	return updated, nil
}

func (u *AgentUsecase) Delete(ctx context.Context, workspaceID, agentID string) error {
	// Запрет из ТЗ: нельзя удалять агента, если у него есть опубликованный workflow.
	if _, ok, err := u.deps.Published.Get(ctx, workspaceID, agentID); err != nil {
		return derr.NewInternal("mongo_error", "failed to check published workflow")
	} else if ok {
		return derr.NewBusiness("agent_has_published_workflow", "agent has published workflow")
	}

	if u.deps.Tx == nil {
		return derr.NewInternal("tx_not_configured", "transactions are not configured")
	}
	if err := u.deps.Tx.WithTransaction(ctx, func(txCtx context.Context) error {
		if err := u.deps.ConversationFlows.DeleteAllForAgent(txCtx, workspaceID, agentID); err != nil {
			return derr.NewInternal("mongo_error", "failed to delete workflows")
		}
		ok, err := u.deps.Agents.Delete(txCtx, workspaceID, agentID)
		if err != nil {
			return derr.NewInternal("mongo_error", "failed to delete agent")
		}
		if !ok {
			return derr.NewNotFound("agent_not_found", "agent not found")
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

