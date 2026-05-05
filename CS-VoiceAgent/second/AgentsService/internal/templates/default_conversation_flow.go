package templates

import (
	"fmt"
	"time"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// DefaultConversationFlowV0Template возвращает минимальный валидный workflow v0.
// Важно: пользователь не должен получать \"пустого\" агента без нод — иначе редактор/исполнение ломаются.
func DefaultConversationFlowV0Template(now time.Time) models.ConversationFlow {
	nowMs := now.UnixMilli()
	startID := fmt.Sprintf("start-node-%d", nowMs)

	return models.ConversationFlow{
		Version:      0,
		Nodes: []models.Node{
			{
				ID:   startID,
				Type: "conversation",
				Name: "Start",
				Instruction: models.Instruction{
					Type: "static_text",
					Text: "Здравствуйте! Чем могу помочь?",
				},
				Edges:                []models.Edge{},
				GlobalNodeSetting:    nil,
				Variables:            []models.Variable{},
				VoiceSpeed:           nil,
				Responsiveness:       nil,
				FinetuneConversationExamples: []models.FinetuneExample{},
				FinetuneTransitionExamples:   []models.FinetuneExample{},
				DisplayPosition:      &models.Position{X: 120, Y: 80},
				StartSpeaker:         ptrStr("agent"),
			},
		},
		StartNodeID:  startID,
		StartSpeaker: "agent",
		GlobalPrompt: "Ты дружелюбный помощник поддержки. {{current_time}}",
		ModelChoice: models.ModelChoice{
			Type:         "cascading",
			Model:        "gpt-5.4",
			HighPriority: true,
		},
		ModelTemperature:   0.7,
		FlexMode:           true,
		ToolCallStrictMode: true,
		KBConfig: models.KBConfig{
			TopK:        3,
			FilterScore: 0.6,
		},
		BeginTagDisplayPos: models.Position{X: 120, Y: 80},
		IsTransferCF:       false,
		CreatedAt:          nowMs,
		UpdatedAt:          nowMs,
	}
}

func ptrStr(s string) *string { return &s }

