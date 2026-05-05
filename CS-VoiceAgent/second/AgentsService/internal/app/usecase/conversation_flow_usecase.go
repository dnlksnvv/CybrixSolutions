package usecase

import (
	"context"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	"github.com/cybrix-solutions/agents-service/internal/domain/validation"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
)

// ConversationFlowUsecase содержит бизнес-логику версий workflow и публикации.
type ConversationFlowUsecase struct {
	deps Deps
}

func NewConversationFlowUsecase(deps Deps) *ConversationFlowUsecase {
	return &ConversationFlowUsecase{deps: deps}
}

func (u *ConversationFlowUsecase) ListVersions(ctx context.Context, workspaceID, agentID string) ([]repo.LightConversationFlowVersion, error) {
	// Проверяем существование агента (контракт API ожидает agent_not_found).
	if _, ok, err := u.deps.Agents.GetByID(ctx, workspaceID, agentID); err != nil {
		return nil, derr.NewInternal("mongo_error", "failed to get agent")
	} else if !ok {
		return nil, derr.NewNotFound("agent_not_found", "agent not found")
	}

	vers, err := u.deps.ConversationFlows.ListVersionsLight(ctx, workspaceID, agentID)
	if err != nil {
		return nil, derr.NewInternal("mongo_error", "failed to list conversation flows")
	}
	pw, ok, err := u.deps.Published.Get(ctx, workspaceID, agentID)
	if err != nil {
		return nil, derr.NewInternal("mongo_error", "failed to get published workflow")
	}
	if ok {
		for i := range vers {
			if vers[i].ConversationFlowID == pw.ConversationFlowID && vers[i].Version == pw.Version {
				vers[i].Published = true
			}
		}
	}
	return vers, nil
}

func (u *ConversationFlowUsecase) GetVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (models.ConversationFlow, bool, error) {
	if _, ok, err := u.deps.Agents.GetByID(ctx, workspaceID, agentID); err != nil {
		return models.ConversationFlow{}, false, derr.NewInternal("mongo_error", "failed to get agent")
	} else if !ok {
		return models.ConversationFlow{}, false, derr.NewNotFound("agent_not_found", "agent not found")
	}

	cf, ok, err := u.deps.ConversationFlows.GetVersion(ctx, workspaceID, agentID, conversationFlowID, version)
	if err != nil {
		return models.ConversationFlow{}, false, derr.NewInternal("mongo_error", "failed to get conversation flow")
	}
	if !ok {
		return models.ConversationFlow{}, false, derr.NewNotFound("conversation_flow_not_found", "conversation flow not found")
	}
	return cf, true, nil
}

func (u *ConversationFlowUsecase) GetPublished(ctx context.Context, workspaceID, agentID string) (models.ConversationFlow, error) {
	if _, ok, err := u.deps.Agents.GetByID(ctx, workspaceID, agentID); err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to get agent")
	} else if !ok {
		return models.ConversationFlow{}, derr.NewNotFound("agent_not_found", "agent not found")
	}
	pw, ok, err := u.deps.Published.Get(ctx, workspaceID, agentID)
	if err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to get published workflow")
	}
	if !ok {
		return models.ConversationFlow{}, derr.NewNotFound("published_workflow_not_found", "published workflow not found")
	}
	cf, ok, err := u.deps.ConversationFlows.GetVersion(ctx, workspaceID, agentID, pw.ConversationFlowID, pw.Version)
	if err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to get conversation flow")
	}
	if !ok {
		return models.ConversationFlow{}, derr.NewNotFound("conversation_flow_not_found", "conversation flow not found")
	}
	return cf, nil
}

func (u *ConversationFlowUsecase) PatchVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int, patch repo.ConversationFlowPatch) (models.ConversationFlow, error) {
	// Нельзя редактировать опубликованную версию.
	if pw, ok, err := u.deps.Published.Get(ctx, workspaceID, agentID); err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to get published workflow")
	} else if ok && pw.ConversationFlowID == conversationFlowID && pw.Version == version {
		return models.ConversationFlow{}, derr.NewBusiness("published_version_is_readonly", "published version is read-only")
	}

	current, ok, err := u.deps.ConversationFlows.GetVersion(ctx, workspaceID, agentID, conversationFlowID, version)
	if err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to get conversation flow")
	}
	if !ok {
		return models.ConversationFlow{}, derr.NewNotFound("conversation_flow_not_found", "conversation flow not found")
	}

	// Применяем patch в памяти (для валидации), сохраняя правило top-level semantics.
	updated := current
	if patch.Nodes != nil {
		updated.Nodes = *patch.Nodes
	}
	if patch.StartNodeID != nil {
		updated.StartNodeID = *patch.StartNodeID
	}
	if patch.StartSpeaker != nil {
		updated.StartSpeaker = *patch.StartSpeaker
	}
	if patch.GlobalPrompt != nil {
		updated.GlobalPrompt = *patch.GlobalPrompt
	}
	if patch.ModelChoice != nil {
		updated.ModelChoice = *patch.ModelChoice
	}
	if patch.ModelTemperature != nil {
		updated.ModelTemperature = *patch.ModelTemperature
	}
	if patch.FlexMode != nil {
		updated.FlexMode = *patch.FlexMode
	}
	if patch.ToolCallStrictMode != nil {
		updated.ToolCallStrictMode = *patch.ToolCallStrictMode
	}
	if patch.KBConfig != nil {
		updated.KBConfig = *patch.KBConfig
	}
	if patch.BeginTagDisplayPosition != nil {
		updated.BeginTagDisplayPos = *patch.BeginTagDisplayPosition
	}
	if patch.IsTransferCF != nil {
		updated.IsTransferCF = *patch.IsTransferCF
	}
	nowMs := u.deps.now().UnixMilli()
	updated.UpdatedAt = nowMs
	patch.UpdatedAt = &nowMs

	// Валидация обновлённого workflow.
	if err := validation.ValidateConversationFlow(updated); err != nil {
		return models.ConversationFlow{}, err
	}

	out, ok, err := u.deps.ConversationFlows.UpdateVersion(ctx, workspaceID, agentID, conversationFlowID, version, patch)
	if err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to update conversation flow")
	}
	if !ok {
		return models.ConversationFlow{}, derr.NewNotFound("conversation_flow_not_found", "conversation flow not found")
	}
	return out, nil
}

func (u *ConversationFlowUsecase) CreateVersionFrom(ctx context.Context, workspaceID, agentID, conversationFlowID string, fromVersion int) (models.ConversationFlow, error) {
	// Лимит из ТЗ: максимум 100 версий на агента.
	maxV, ok, err := u.deps.ConversationFlows.MaxVersion(ctx, workspaceID, agentID, conversationFlowID)
	if err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to get max version")
	}
	if !ok {
		return models.ConversationFlow{}, derr.NewNotFound("conversation_flow_not_found", "conversation flow not found")
	}
	if maxV >= 99 {
		return models.ConversationFlow{}, derr.NewBusiness("workflow_version_limit_exceeded", "workflow version limit exceeded")
	}

	src, ok, err := u.deps.ConversationFlows.GetVersion(ctx, workspaceID, agentID, conversationFlowID, fromVersion)
	if err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to get source version")
	}
	if !ok {
		return models.ConversationFlow{}, derr.NewNotFound("source_version_not_found", "source version not found")
	}

	now := u.deps.now()
	nowMs := now.UnixMilli()
	newCF := src
	newCF.Version = maxV + 1
	newCF.CreatedAt = nowMs
	newCF.UpdatedAt = nowMs

	if err := validation.ValidateConversationFlow(newCF); err != nil {
		return models.ConversationFlow{}, err
	}

	if err := u.deps.ConversationFlows.CreateVersion(ctx, newCF); err != nil {
		return models.ConversationFlow{}, derr.NewInternal("mongo_error", "failed to create conversation flow version")
	}
	return newCF, nil
}

func (u *ConversationFlowUsecase) Publish(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (models.PublishedWorkflow, error) {
	// Проверяем существование версии.
	if _, ok, err := u.deps.ConversationFlows.GetVersion(ctx, workspaceID, agentID, conversationFlowID, version); err != nil {
		return models.PublishedWorkflow{}, derr.NewInternal("mongo_error", "failed to get conversation flow")
	} else if !ok {
		return models.PublishedWorkflow{}, derr.NewNotFound("workflow_version_not_found", "workflow version not found")
	}

	if u.deps.Tx == nil {
		return models.PublishedWorkflow{}, derr.NewInternal("tx_not_configured", "transactions are not configured")
	}
	now := u.deps.now()
	nowMs := now.UnixMilli()
	pw := models.PublishedWorkflow{
		WorkspaceID:        workspaceID,
		AgentID:            agentID,
		ConversationFlowID: conversationFlowID,
		Version:            version,
		PublishedAt:        nowMs,
		CreatedAt:          nowMs,
		UpdatedAt:          nowMs,
	}

	if err := u.deps.Tx.WithTransaction(ctx, func(txCtx context.Context) error {
		if err := u.deps.Published.Upsert(txCtx, pw); err != nil {
			return derr.NewInternal("mongo_error", "failed to upsert published workflow")
		}
		// Синхронно обновляем Agent.response_engine + updated_at + last_modified (ТЗ).
		patch := repo.AgentPatch{
			ResponseEngine: &models.ResponseEngine{
				Type:               "conversation-flow",
				ConversationFlowID: conversationFlowID,
				Version:            version,
			},
			UpdatedAt:    &nowMs,
			LastModified: &nowMs,
		}
		if _, ok, err := u.deps.Agents.Update(txCtx, workspaceID, agentID, patch); err != nil {
			return derr.NewInternal("mongo_error", "failed to update agent response_engine")
		} else if !ok {
			return derr.NewNotFound("agent_not_found", "agent not found")
		}
		return nil
	}); err != nil {
		return models.PublishedWorkflow{}, err
	}

	return pw, nil
}

func (u *ConversationFlowUsecase) Unpublish(ctx context.Context, workspaceID, agentID string) error {
	// Ваша правка: unpublish снимает только production-публикацию, удаляя PublishedWorkflow.
	// Agent.response_engine мы НЕ очищаем.
	ok, err := u.deps.Published.Delete(ctx, workspaceID, agentID)
	if err != nil {
		return derr.NewInternal("mongo_error", "failed to unpublish workflow")
	}
	if !ok {
		return derr.NewNotFound("published_workflow_not_found", "published workflow not found")
	}
	return nil
}

func (u *ConversationFlowUsecase) DeleteVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) error {
	if pw, ok, err := u.deps.Published.Get(ctx, workspaceID, agentID); err != nil {
		return derr.NewInternal("mongo_error", "failed to get published workflow")
	} else if ok && pw.ConversationFlowID == conversationFlowID && pw.Version == version {
		return derr.NewBusiness("cannot_delete_published_version", "cannot delete published version")
	}

	ok, err := u.deps.ConversationFlows.DeleteVersion(ctx, workspaceID, agentID, conversationFlowID, version)
	if err != nil {
		return derr.NewInternal("mongo_error", "failed to delete conversation flow")
	}
	if !ok {
		return derr.NewNotFound("conversation_flow_not_found", "conversation flow not found")
	}
	return nil
}
