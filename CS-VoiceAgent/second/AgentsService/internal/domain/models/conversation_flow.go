package models

// ConversationFlow — версионируемая логика разговора (workflow).
// Важно: поле публикации НЕ хранится в ConversationFlow. Источник истины — PublishedWorkflow.
type ConversationFlow struct {
	ConversationFlowID string `json:"conversation_flow_id" bson:"conversation_flow_id"`
	AgentID            string `json:"agent_id" bson:"agent_id"`
	WorkspaceID        string `json:"workspace_id" bson:"workspace_id"`
	Version            int    `json:"version" bson:"version"`

	Nodes        []Node `json:"nodes" bson:"nodes"`
	StartNodeID  string `json:"start_node_id" bson:"start_node_id"`
	StartSpeaker string `json:"start_speaker" bson:"start_speaker"` // "agent" | "user"

	GlobalPrompt string      `json:"global_prompt" bson:"global_prompt"`
	ModelChoice  ModelChoice `json:"model_choice" bson:"model_choice"`

	ModelTemperature    float64 `json:"model_temperature" bson:"model_temperature"`
	FlexMode            bool    `json:"flex_mode" bson:"flex_mode"`
	ToolCallStrictMode  bool    `json:"tool_call_strict_mode" bson:"tool_call_strict_mode"`
	KBConfig            KBConfig `json:"kb_config" bson:"kb_config"`
	BeginTagDisplayPos  Position `json:"begin_tag_display_position" bson:"begin_tag_display_position"`
	IsTransferCF        bool     `json:"is_transfer_cf" bson:"is_transfer_cf"`

	CreatedAt int64 `json:"created_at" bson:"created_at"`
	UpdatedAt int64 `json:"updated_at" bson:"updated_at"`
}

// ModelChoice — настройки выбора LLM модели.
type ModelChoice struct {
	Type         string `json:"type" bson:"type"` // обычно "cascading"
	Model        string `json:"model" bson:"model"`
	HighPriority bool   `json:"high_priority" bson:"high_priority"`
}

// KBConfig — настройки retrieval/RAG.
type KBConfig struct {
	TopK        int     `json:"top_k" bson:"top_k"`
	FilterScore float64 `json:"filter_score" bson:"filter_score"`
}

// Position — координаты элемента на canvas фронтенда.
type Position struct {
	X float64 `json:"x" bson:"x"`
	Y float64 `json:"y" bson:"y"`
}

