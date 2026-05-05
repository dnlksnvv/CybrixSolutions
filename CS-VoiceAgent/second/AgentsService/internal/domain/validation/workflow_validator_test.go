package validation

import (
	"testing"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	"github.com/stretchr/testify/require"
)

func TestValidateConversationFlow_OK_Minimal(t *testing.T) {
	cf := models.ConversationFlow{
		ConversationFlowID: "conversation_flow_x",
		AgentID:            "agent_x",
		WorkspaceID:        "ws_x",
		Version:            0,
		Nodes: []models.Node{
			{
				ID:   "n1",
				Type: "conversation",
				Name: "Start",
				Instruction: models.Instruction{
					Type: "static_text",
					Text: "hi",
				},
			},
		},
		StartNodeID:  "n1",
		StartSpeaker: "agent",
		GlobalPrompt: "prompt",
	}
	require.NoError(t, ValidateConversationFlow(cf))
}

func TestValidateConversationFlow_DuplicateNodeID(t *testing.T) {
	cf := models.ConversationFlow{
		Nodes: []models.Node{
			{ID: "n1", Type: "t", Name: "a"},
			{ID: "n1", Type: "t", Name: "b"},
		},
		StartNodeID: "n1",
	}
	require.Error(t, ValidateConversationFlow(cf))
}

func TestValidateConversationFlow_StartNodeNotFound(t *testing.T) {
	cf := models.ConversationFlow{
		Nodes:       []models.Node{{ID: "n1", Type: "t", Name: "a"}},
		StartNodeID:  "missing",
		StartSpeaker: "agent",
	}
	require.Error(t, ValidateConversationFlow(cf))
}

func TestValidateConversationFlow_SizeExceeded(t *testing.T) {
	// Делаем workflow заведомо больше 8MB за счёт одной большой строки.
	// Сама структура минимальна, чтобы ошибка была именно по размеру, а не по другим лимитам.
	big := make([]byte, MaxWorkflowJSONBytes+1024)
	for i := range big {
		big[i] = 'a'
	}
	cf := models.ConversationFlow{
		ConversationFlowID: "conversation_flow_x",
		AgentID:            "agent_x",
		WorkspaceID:        "ws_x",
		Version:            0,
		Nodes: []models.Node{
			{ID: "n1", Type: "conversation", Name: "Start", Instruction: models.Instruction{Type: "static_text", Text: string(big)}},
		},
		StartNodeID: "n1",
	}
	require.Error(t, ValidateConversationFlow(cf))
}

