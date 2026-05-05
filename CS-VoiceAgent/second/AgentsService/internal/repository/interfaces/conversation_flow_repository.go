package interfaces

import (
	"context"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// ConversationFlowRepository — контракт доступа к версиям workflow.
type ConversationFlowRepository interface {
	GetVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (models.ConversationFlow, bool, error)
	ListVersionsLight(ctx context.Context, workspaceID, agentID string) ([]LightConversationFlowVersion, error)
	CreateVersion(ctx context.Context, cf models.ConversationFlow) error
	UpdateVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int, patch ConversationFlowPatch) (models.ConversationFlow, bool, error)
	DeleteVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (bool, error)
	MaxVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string) (int, bool, error)
	DeleteAllForAgent(ctx context.Context, workspaceID, agentID string) error
}

// LightConversationFlowVersion — сокращённый DTO для списка версий (без nodes).
type LightConversationFlowVersion struct {
	ConversationFlowID string `json:"conversation_flow_id"`
	Version            int    `json:"version"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
	Published          bool   `json:"published"`
}

// ConversationFlowPatch — top-level patch для версии workflow.
// Важно: если передан вложенный объект, он заменяется целиком.
// Если передан nodes — заменяется весь массив nodes целиком.
type ConversationFlowPatch struct {
	Nodes                 *[]models.Node
	StartNodeID            *string
	StartSpeaker           *string
	GlobalPrompt           *string
	ModelChoice            *models.ModelChoice
	ModelTemperature       *float64
	FlexMode               *bool
	ToolCallStrictMode     *bool
	KBConfig               *models.KBConfig
	BeginTagDisplayPosition *models.Position
	IsTransferCF           *bool
	UpdatedAt              *int64
}

