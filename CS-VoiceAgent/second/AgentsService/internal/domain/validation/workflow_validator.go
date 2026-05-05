package validation

import (
	"encoding/json"
	"fmt"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// ValidateConversationFlow проверяет workflow по правилам из ТЗ.
// Это не \"формальная\" проверка, а контрактный барьер, который защищает сервис
// от сохранения невалидного графа (который потом невозможно исполнить/отрисовать).
func ValidateConversationFlow(cf models.ConversationFlow) error {
	if err := ValidateConversationFlowSize(cf); err != nil {
		return err
	}

	// nodes
	if len(cf.Nodes) > MaxNodesPerWorkflow1000 {
		limit := MaxNodesPerWorkflow1000
		return derr.NewValidation("nodes", "max_items_exceeded", fmt.Sprintf("nodes must contain no more than %d items", limit), &limit)
	}

	ids := make(map[string]struct{}, len(cf.Nodes))
	for i, n := range cf.Nodes {
		if n.ID == "" {
			return derr.NewValidation(fmt.Sprintf("nodes[%d].id", i), "required", "node id is required", nil)
		}
		if _, ok := ids[n.ID]; ok {
			return derr.NewValidation(fmt.Sprintf("nodes[%d].id", i), "duplicate", "node id must be unique", nil)
		}
		ids[n.ID] = struct{}{}

		if n.Type == "" {
			return derr.NewValidation(fmt.Sprintf("nodes[%d].type", i), "required", "node type is required", nil)
		}
		if n.Name == "" {
			return derr.NewValidation(fmt.Sprintf("nodes[%d].name", i), "required", "node name is required", nil)
		}
		if len(n.Name) > MaxLenName255 {
			limit := MaxLenName255
			return derr.NewValidation(fmt.Sprintf("nodes[%d].name", i), "max_length_exceeded", fmt.Sprintf("node name must be no longer than %d characters", limit), &limit)
		}

		// instruction
		if n.Instruction.Type != "" && n.Instruction.Type != "static_text" && n.Instruction.Type != "prompt" {
			return derr.NewValidation(fmt.Sprintf("nodes[%d].instruction.type", i), "invalid_enum", "instruction.type must be static_text or prompt", nil)
		}
		if len(n.Instruction.Text) > MaxLenInstructionText50000 {
			limit := MaxLenInstructionText50000
			return derr.NewValidation(fmt.Sprintf("nodes[%d].instruction.text", i), "max_length_exceeded", fmt.Sprintf("instruction.text must be no longer than %d characters", limit), &limit)
		}

		// edges
		if len(n.Edges) > MaxEdgesPerNode50 {
			limit := MaxEdgesPerNode50
			return derr.NewValidation(fmt.Sprintf("nodes[%d].edges", i), "max_items_exceeded", fmt.Sprintf("edges must contain no more than %d items", limit), &limit)
		}
		for j, e := range n.Edges {
			if e.DestinationNodeID == "" {
				return derr.NewValidation(fmt.Sprintf("nodes[%d].edges[%d].destination_node_id", i, j), "required", "destination_node_id is required", nil)
			}
			// destination_node_id must exist
			if _, ok := ids[e.DestinationNodeID]; !ok {
				return derr.NewValidation(fmt.Sprintf("nodes[%d].edges[%d].destination_node_id", i, j), "invalid_reference", "destination_node_id must reference existing node id", nil)
			}
			if len(e.TransitionCondition.Prompt) > MaxLenEdgePrompt10000 {
				limit := MaxLenEdgePrompt10000
				return derr.NewValidation(fmt.Sprintf("nodes[%d].edges[%d].transition_condition.prompt", i, j), "max_length_exceeded", fmt.Sprintf("prompt must be no longer than %d characters", limit), &limit)
			}
		}

		// variables
		if len(n.Variables) > MaxVarsPerNode100 {
			limit := MaxVarsPerNode100
			return derr.NewValidation(fmt.Sprintf("nodes[%d].variables", i), "max_items_exceeded", fmt.Sprintf("variables must contain no more than %d items", limit), &limit)
		}
		for k, v := range n.Variables {
			if v.Name == "" {
				return derr.NewValidation(fmt.Sprintf("nodes[%d].variables[%d].name", i, k), "required", "variable name is required", nil)
			}
			if len(v.Name) > MaxLenVarName128 {
				limit := MaxLenVarName128
				return derr.NewValidation(fmt.Sprintf("nodes[%d].variables[%d].name", i, k), "max_length_exceeded", fmt.Sprintf("variable name must be no longer than %d characters", limit), &limit)
			}
			if len(v.Description) > MaxLenVarDescription2000 {
				limit := MaxLenVarDescription2000
				return derr.NewValidation(fmt.Sprintf("nodes[%d].variables[%d].description", i, k), "max_length_exceeded", fmt.Sprintf("variable description must be no longer than %d characters", limit), &limit)
			}
			switch v.Type {
			case "string", "number", "boolean", "enum":
			default:
				return derr.NewValidation(fmt.Sprintf("nodes[%d].variables[%d].type", i, k), "invalid_enum", "variable type must be string, number, boolean, or enum", nil)
			}
			if v.Type == "enum" {
				if len(v.Choices) == 0 {
					return derr.NewValidation(fmt.Sprintf("nodes[%d].variables[%d].choices", i, k), "required", "choices is required for enum variables", nil)
				}
				if len(v.Choices) > MaxEnumChoices100 {
					limit := MaxEnumChoices100
					return derr.NewValidation(fmt.Sprintf("nodes[%d].variables[%d].choices", i, k), "max_items_exceeded", fmt.Sprintf("choices must contain no more than %d items", limit), &limit)
				}
				for m, ch := range v.Choices {
					if len(ch) > MaxLenEnumChoice255 {
						limit := MaxLenEnumChoice255
						return derr.NewValidation(fmt.Sprintf("nodes[%d].variables[%d].choices[%d]", i, k, m), "max_length_exceeded", fmt.Sprintf("choice must be no longer than %d characters", limit), &limit)
					}
				}
			}
		}
	}

	// start_node_id must exist (after we collected ids)
	if cf.StartNodeID == "" {
		return derr.NewValidation("start_node_id", "required", "start_node_id is required", nil)
	}
	if _, ok := ids[cf.StartNodeID]; !ok {
		return derr.NewValidation("start_node_id", "invalid_reference", "start_node_id must reference existing node id", nil)
	}

	// start_speaker
	if cf.StartSpeaker != "" && cf.StartSpeaker != "agent" && cf.StartSpeaker != "user" {
		return derr.NewValidation("start_speaker", "invalid_enum", "start_speaker must be agent or user", nil)
	}

	// global_prompt
	if len(cf.GlobalPrompt) > MaxLenGlobalPrompt50000 {
		limit := MaxLenGlobalPrompt50000
		return derr.NewValidation("global_prompt", "max_length_exceeded", fmt.Sprintf("global_prompt must be no longer than %d characters", limit), &limit)
	}

	// kb_config numeric bounds are validated in higher-level validators (to keep this function focused on workflow structure).
	return nil
}

// ValidateConversationFlowSize проверяет размер сериализованного ConversationFlow JSON.
// Это отдельное правило из ТЗ: важно отсекать слишком большие workflow ещё на входе,
// чтобы не перегружать БД и не создавать \"тяжёлые\" агенты для рантайма.
func ValidateConversationFlowSize(cf models.ConversationFlow) error {
	b, err := json.Marshal(cf)
	if err != nil {
		return derr.NewInternal("workflow_json_marshal_failed", "failed to serialize workflow")
	}
	if len(b) <= MaxWorkflowJSONBytes {
		return nil
	}
	limit := MaxWorkflowJSONBytes
	field := "workflow"
	return derr.NewValidation(field, "workflow_size_exceeded", "Workflow JSON size must be no larger than 8 MB", &limit)
}

