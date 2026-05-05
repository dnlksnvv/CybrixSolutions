package models

// Agent — основная сущность пользователя: настройки голосового/чат-агента.
// Важно: Agent НЕ версионируется. Версионируется только ConversationFlow.
// Поэтому любые изменения настроек Agent (tts/stt/voice_id/и т.д.) применяются сразу
// ко всем будущим запускам, независимо от того, какая версия workflow опубликована.
type Agent struct {
	AgentID    string  `json:"agent_id" bson:"agent_id"`
	WorkspaceID string `json:"workspace_id" bson:"workspace_id"`
	FolderID   *string `json:"folder_id" bson:"folder_id"`

	Name    string `json:"name" bson:"name"`
	Channel string `json:"channel" bson:"channel"` // "voice" | "chat"

	VoiceID  string `json:"voice_id" bson:"voice_id"`
	Language string `json:"language" bson:"language"`

	TTS TTSConfig `json:"tts" bson:"tts"`
	STT STTConfig `json:"stt" bson:"stt"`

	InterruptionSensitivity float64 `json:"interruption_sensitivity" bson:"interruption_sensitivity"`
	MaxCallDurationMS       int64   `json:"max_call_duration_ms" bson:"max_call_duration_ms"`
	NormalizeForSpeech      bool    `json:"normalize_for_speech" bson:"normalize_for_speech"`
	AllowUserDTMF           bool    `json:"allow_user_dtmf" bson:"allow_user_dtmf"`
	UserDTMFOptions         any     `json:"user_dtmf_options" bson:"user_dtmf_options"`

	ResponseEngine ResponseEngine `json:"response_engine" bson:"response_engine"`

	HandbookConfig HandbookConfig `json:"handbook_config" bson:"handbook_config"`
	PIIConfig      PIIConfig      `json:"pii_config" bson:"pii_config"`

	DataStorageSetting string `json:"data_storage_setting" bson:"data_storage_setting"` // "everything" | "none" | "metadata_only"

	LastModified int64 `json:"last_modified" bson:"last_modified"`
	CreatedAt    int64 `json:"created_at" bson:"created_at"`
	UpdatedAt    int64 `json:"updated_at" bson:"updated_at"`
}

// TTSConfig — настройки Text-To-Speech на уровне Agent.
type TTSConfig struct {
	VoiceID  string   `json:"voice_id" bson:"voice_id"`
	Language string   `json:"language" bson:"language"`
	Emotion  *string  `json:"emotion" bson:"emotion"`
	Speed    *float64 `json:"speed" bson:"speed"`
}

// STTConfig — настройки Speech-To-Text на уровне Agent.
type STTConfig struct {
	ModelID  string `json:"model_id" bson:"model_id"`
	Language string `json:"language" bson:"language"`
}

// ResponseEngine — ссылка на выбранную версию workflow.
// В ТЗ это денормализованная копия, которую сервис обновляет при publish.
// Дополнение из вашей правки: после unpublish production-публикация снимается через PublishedWorkflow,
// но response_engine может оставаться заполненным для editor сценариев.
type ResponseEngine struct {
	Type               string `json:"type" bson:"type"` // всегда "conversation-flow"
	ConversationFlowID string `json:"conversation_flow_id" bson:"conversation_flow_id"`
	Version            int    `json:"version" bson:"version"`
}

// HandbookConfig — дополнительные настройки поведения агента.
type HandbookConfig struct {
	DefaultPersonality bool `json:"default_personality" bson:"default_personality"`
	AIDisclosure       bool `json:"ai_disclosure" bson:"ai_disclosure"`
}

// PIIConfig — настройки обработки персональных данных.
type PIIConfig struct {
	Mode       string `json:"mode" bson:"mode"` // "post_call" | "none"
	Categories any    `json:"categories" bson:"categories"`
}

