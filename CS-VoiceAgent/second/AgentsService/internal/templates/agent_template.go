package templates

import (
	"time"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// DefaultAgentTemplate возвращает дефолтные значения Agent.
// Это отдельная точка редактирования: менять поведение дефолтов можно здесь,
// не трогая бизнес-логику usecase-слоя.
func DefaultAgentTemplate(now time.Time) models.Agent {
	nowMs := now.UnixMilli()
	return models.Agent{
		Name:    "New Agent",
		Channel: "voice",

		VoiceID:  "retell-Cimo",
		Language: "ru-RU",

		TTS: models.TTSConfig{
			VoiceID:  "retell-Cimo",
			Language: "ru-RU",
			Emotion:  nil,
			Speed:    ptrFloat(1.0),
		},
		STT: models.STTConfig{
			ModelID:  "default",
			Language: "ru-RU",
		},

		InterruptionSensitivity: 0.9,
		MaxCallDurationMS:       3600000,
		NormalizeForSpeech:      true,
		AllowUserDTMF:           true,
		UserDTMFOptions:         map[string]any{},

		ResponseEngine: models.ResponseEngine{
			Type:               "conversation-flow",
			ConversationFlowID: "",
			Version:            0,
		},

		HandbookConfig: models.HandbookConfig{
			DefaultPersonality: true,
			AIDisclosure:       true,
		},
		PIIConfig: models.PIIConfig{
			Mode:       "none",
			Categories: []any{},
		},
		DataStorageSetting: "everything",

		LastModified: nowMs,
		CreatedAt:    nowMs,
		UpdatedAt:    nowMs,
	}
}

func ptrFloat(v float64) *float64 { return &v }

