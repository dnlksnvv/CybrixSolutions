package models

// PublishedWorkflow — единственный источник истины о том, какая версия workflow опубликована для агента.
// Уникальность обеспечивается на уровне БД индексом (workspace_id, agent_id).
type PublishedWorkflow struct {
	WorkspaceID        string `json:"workspace_id" bson:"workspace_id"`
	AgentID            string `json:"agent_id" bson:"agent_id"`
	ConversationFlowID string `json:"conversation_flow_id" bson:"conversation_flow_id"`
	Version            int    `json:"version" bson:"version"`
	PublishedAt        int64  `json:"published_at" bson:"published_at"`
	CreatedAt          int64  `json:"created_at" bson:"created_at"`
	UpdatedAt          int64  `json:"updated_at" bson:"updated_at"`
}

