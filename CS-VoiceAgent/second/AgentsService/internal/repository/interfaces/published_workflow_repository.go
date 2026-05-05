package interfaces

import (
	"context"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// PublishedWorkflowRepository — контракт работы с публикацией.
// Это единственный источник истины для production-публикации.
type PublishedWorkflowRepository interface {
	Get(ctx context.Context, workspaceID, agentID string) (models.PublishedWorkflow, bool, error)
	Upsert(ctx context.Context, pw models.PublishedWorkflow) error
	Delete(ctx context.Context, workspaceID, agentID string) (bool, error)

	// ListByWorkspace возвращает список опубликованных агентов в workspace.
	// Используется runtime endpoint'ами микросервиса звонков.
	ListByWorkspace(ctx context.Context, workspaceID string) ([]models.PublishedWorkflow, error)

	// ListGroupedByWorkspace возвращает агрегированный список: workspace -> published agents.
	// Используется runtime endpoint'ом, который возвращает все workspaces с опубликованными агентами.
	ListGroupedByWorkspace(ctx context.Context) ([]PublishedAgentsByWorkspace, error)
}

// PublishedAgentSummary — краткое описание опубликованного агента для runtime ответа.
type PublishedAgentSummary struct {
	AgentID            string `json:"agent_id" bson:"agent_id"`
	ConversationFlowID string `json:"conversation_flow_id" bson:"conversation_flow_id"`
	Version            int    `json:"version" bson:"version"`
	PublishedAt        int64  `json:"published_at" bson:"published_at"`
}

// PublishedAgentsByWorkspace — runtime DTO для группировки опубликованных агентов по workspace.
type PublishedAgentsByWorkspace struct {
	WorkspaceID string                  `json:"workspace_id" bson:"workspace_id"`
	Agents      []PublishedAgentSummary `json:"agents" bson:"agents"`
}
