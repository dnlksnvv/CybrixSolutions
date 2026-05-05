package models

// Node — элемент графа workflow. По ТЗ сервис обязан хранить и возвращать структуру максимально близко к Retell AI.
// Мы не интерпретируем динамические переменные внутри строк ({{current_time}} и т.п.) — это просто текст.
type Node struct {
	ID   string `json:"id" bson:"id"`
	Type string `json:"type" bson:"type"`
	Name string `json:"name" bson:"name"`

	Instruction Instruction `json:"instruction" bson:"instruction"`
	Edges       []Edge      `json:"edges" bson:"edges"`

	GlobalNodeSetting *GlobalNodeSetting `json:"global_node_setting" bson:"global_node_setting"`
	Variables         []Variable         `json:"variables" bson:"variables"`

	VoiceSpeed     *float64 `json:"voice_speed" bson:"voice_speed"`
	Responsiveness *float64 `json:"responsiveness" bson:"responsiveness"`

	FinetuneConversationExamples []FinetuneExample `json:"finetune_conversation_examples" bson:"finetune_conversation_examples"`
	FinetuneTransitionExamples   []FinetuneExample `json:"finetune_transition_examples" bson:"finetune_transition_examples"`

	DisplayPosition *Position `json:"display_position" bson:"display_position"`
	StartSpeaker    *string   `json:"start_speaker" bson:"start_speaker"` // "agent" | "user"
}

type Instruction struct {
	Type string `json:"type" bson:"type"` // "static_text" | "prompt"
	Text string `json:"text" bson:"text"`
}

type Edge struct {
	ID                string              `json:"id" bson:"id"`
	DestinationNodeID string              `json:"destination_node_id" bson:"destination_node_id"`
	TransitionCondition TransitionCondition `json:"transition_condition" bson:"transition_condition"`
}

type TransitionCondition struct {
	Type   string `json:"type" bson:"type"` // "prompt"
	Prompt string `json:"prompt" bson:"prompt"`
}

type GlobalNodeSetting struct {
	Condition           string             `json:"condition" bson:"condition"`
	GoBackConditions    []GoBackCondition  `json:"go_back_conditions" bson:"go_back_conditions"`
	PositiveExamples    []FinetuneExample  `json:"positive_finetune_examples" bson:"positive_finetune_examples"`
	NegativeExamples    []FinetuneExample  `json:"negative_finetune_examples" bson:"negative_finetune_examples"`
	CoolDown            int                `json:"cool_down" bson:"cool_down"`
}

type GoBackCondition struct {
	ID                 string              `json:"id" bson:"id"`
	TransitionCondition TransitionCondition `json:"transition_condition" bson:"transition_condition"`
}

type Variable struct {
	Name        string   `json:"name" bson:"name"`
	Description string   `json:"description" bson:"description"`
	Type        string   `json:"type" bson:"type"` // "string" | "number" | "boolean" | "enum"
	Choices     []string `json:"choices" bson:"choices"`
}

type FinetuneExample struct {
	Transcript []TranscriptItem `json:"transcript" bson:"transcript"`
}

type TranscriptItem struct {
	Role    string `json:"role" bson:"role"` // "user" | "agent"
	Content string `json:"content" bson:"content"`
}

