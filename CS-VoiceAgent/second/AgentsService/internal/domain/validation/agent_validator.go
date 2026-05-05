package validation

import (
	"fmt"
	"strings"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// ValidateAgentForCreate проверяет данные агента на создание.
// Важно: POST /agents может быть \"частичным\" и дополняется backend defaults, поэтому здесь
// мы валидируем только то, что уже присутствует в итоговой доменной модели после применения шаблона.
func ValidateAgentForCreate(a models.Agent) error {
	return validateAgentCommon(a)
}

// ValidateAgentForUpdate проверяет данные агента после применения PATCH (top-level semantics).
func ValidateAgentForUpdate(a models.Agent) error {
	return validateAgentCommon(a)
}

func validateAgentCommon(a models.Agent) error {
	if strings.TrimSpace(a.Name) == "" {
		return derr.NewValidation("name", "required", "name is required", nil)
	}
	if len(a.Name) > MaxLenName255 {
		limit := MaxLenName255
		return derr.NewValidation("name", "max_length_exceeded", fmt.Sprintf("name must be no longer than %d characters", limit), &limit)
	}

	if a.Channel != "voice" && a.Channel != "chat" {
		return derr.NewValidation("channel", "invalid_enum", "channel must be voice or chat", nil)
	}

	if a.InterruptionSensitivity < 0.0 || a.InterruptionSensitivity > 1.0 {
		return derr.NewValidation("interruption_sensitivity", "out_of_range", "interruption_sensitivity must be between 0.0 and 1.0", nil)
	}
	if a.MaxCallDurationMS < 60000 || a.MaxCallDurationMS > 14400000 {
		return derr.NewValidation("max_call_duration_ms", "out_of_range", "max_call_duration_ms must be between 60000 and 14400000", nil)
	}
	if a.TTS.Speed != nil {
		if *a.TTS.Speed < 0.5 || *a.TTS.Speed > 2.0 {
			return derr.NewValidation("tts.speed", "out_of_range", "tts.speed must be between 0.5 and 2.0", nil)
		}
	}

	// ResponseEngine.type фиксирован для этого сервиса.
	if strings.TrimSpace(a.ResponseEngine.Type) == "" {
		return derr.NewValidation("response_engine.type", "required", "response_engine.type is required", nil)
	}
	if a.ResponseEngine.Type != "conversation-flow" {
		return derr.NewValidation("response_engine.type", "invalid_enum", "response_engine.type must be conversation-flow", nil)
	}
	if strings.TrimSpace(a.ResponseEngine.ConversationFlowID) == "" {
		return derr.NewValidation("response_engine.conversation_flow_id", "required", "response_engine.conversation_flow_id is required", nil)
	}
	if a.ResponseEngine.Version < 0 {
		return derr.NewValidation("response_engine.version", "out_of_range", "response_engine.version must be >= 0", nil)
	}

	switch a.DataStorageSetting {
	case "everything", "none", "metadata_only":
	default:
		return derr.NewValidation("data_storage_setting", "invalid_enum", "data_storage_setting must be everything, none, or metadata_only", nil)
	}

	switch a.PIIConfig.Mode {
	case "post_call", "none":
	default:
		return derr.NewValidation("pii_config.mode", "invalid_enum", "pii_config.mode must be post_call or none", nil)
	}

	// FolderID валидируется на существование в usecase (через repository).
	_ = a.UserDTMFOptions // свободная структура
	return nil
}

