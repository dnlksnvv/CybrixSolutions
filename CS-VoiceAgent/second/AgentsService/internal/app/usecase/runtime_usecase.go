package usecase

import (
	"context"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
)

// RuntimeUsecase — read-only бизнес-логика для микросервиса звонков.
// Важно: runtime endpoints не доверяют `agent.response_engine` как источнику истины,
// а берут опубликованную версию строго из PublishedWorkflow.
type RuntimeUsecase struct {
	deps Deps
}

func NewRuntimeUsecase(deps Deps) *RuntimeUsecase {
	return &RuntimeUsecase{deps: deps}
}

func (u *RuntimeUsecase) ListPublishedAgentsByWorkspaces(ctx context.Context) ([]repo.PublishedAgentsByWorkspace, error) {
	items, err := u.deps.Published.ListGroupedByWorkspace(ctx)
	if err != nil {
		return nil, derr.NewInternal("mongo_error", "failed to list published agents by workspaces")
	}
	return items, nil
}

func (u *RuntimeUsecase) ListPublishedAgentsForWorkspace(ctx context.Context, workspaceID string) ([]models.PublishedWorkflow, error) {
	items, err := u.deps.Published.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, derr.NewInternal("mongo_error", "failed to list published agents for workspace")
	}
	return items, nil
}

// PublishedConfig — runtime response DTO для published-config.
type PublishedConfig struct {
	Agent            models.Agent            `json:"agent"`
	ConversationFlow models.ConversationFlow `json:"conversation_flow"`
	PublishedWorkflow models.PublishedWorkflow `json:"published_workflow"`
}

func (u *RuntimeUsecase) GetPublishedConfig(ctx context.Context, workspaceID, agentID string) (PublishedConfig, error) {
	agent, ok, err := u.deps.Agents.GetByID(ctx, workspaceID, agentID)
	if err != nil {
		return PublishedConfig{}, derr.NewInternal("mongo_error", "failed to get agent")
	}
	if !ok {
		return PublishedConfig{}, derr.NewNotFound("agent_not_found", "agent not found")
	}

	pw, ok, err := u.deps.Published.Get(ctx, workspaceID, agentID)
	if err != nil {
		return PublishedConfig{}, derr.NewInternal("mongo_error", "failed to get published workflow")
	}
	if !ok {
		return PublishedConfig{}, derr.NewNotFound("published_workflow_not_found", "published workflow not found")
	}

	// Истина для runtime — PublishedWorkflow.
	agent.ResponseEngine = models.ResponseEngine{
		Type:               "conversation-flow",
		ConversationFlowID: pw.ConversationFlowID,
		Version:            pw.Version,
	}

	cf, ok, err := u.deps.ConversationFlows.GetVersion(ctx, workspaceID, agentID, pw.ConversationFlowID, pw.Version)
	if err != nil {
		return PublishedConfig{}, derr.NewInternal("mongo_error", "failed to get conversation flow")
	}
	if !ok {
		return PublishedConfig{}, derr.NewNotFound("conversation_flow_not_found", "conversation flow not found")
	}

	return PublishedConfig{
		Agent:             agent,
		ConversationFlow:  cf,
		PublishedWorkflow: pw,
	}, nil
}

