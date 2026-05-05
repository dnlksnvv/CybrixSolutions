package handlers

import (
	"net/http"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/gin-gonic/gin"
)

// RuntimeListPublishedAgentsByWorkspace реализует:
// GET /api/v1/runtime/workspaces/published-agents
func (a *API) RuntimeListPublishedAgentsByWorkspace(c *gin.Context) {
	if a.deps.Runtime == nil {
		writeError(c, derr.NewInternal("runtime_not_configured", "runtime usecase is not configured"))
		return
	}
	out, err := a.deps.Runtime.ListPublishedAgentsByWorkspaces(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusOK, out)
}

// RuntimeListPublishedAgentsForWorkspace реализует:
// GET /api/v1/runtime/workspaces/{workspaceId}/published-agents
func (a *API) RuntimeListPublishedAgentsForWorkspace(c *gin.Context) {
	if a.deps.Runtime == nil {
		writeError(c, derr.NewInternal("runtime_not_configured", "runtime usecase is not configured"))
		return
	}
	workspaceID := c.Param("workspaceId")
	out, err := a.deps.Runtime.ListPublishedAgentsForWorkspace(c.Request.Context(), workspaceID)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusOK, out)
}

type runtimePublishedConfigResp struct {
	Agent             any `json:"agent"`
	ConversationFlow  any `json:"conversation_flow"`
	PublishedWorkflow any `json:"published_workflow"`
}

// RuntimeGetPublishedConfig реализует:
// GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
func (a *API) RuntimeGetPublishedConfig(c *gin.Context) {
	if a.deps.Runtime == nil {
		writeError(c, derr.NewInternal("runtime_not_configured", "runtime usecase is not configured"))
		return
	}
	workspaceID := c.Param("workspaceId")
	agentID := c.Param("agentId")

	cfg, err := a.deps.Runtime.GetPublishedConfig(c.Request.Context(), workspaceID, agentID)
	if err != nil {
		writeError(c, err)
		return
	}

	// В runtime response conversation_flow должен содержать published=true (по контракту update.md).
	writeOK(c, http.StatusOK, runtimePublishedConfigResp{
		Agent: cfg.Agent,
		ConversationFlow: gin.H{
			"published":           true,
			"conversation_flow_id": cfg.ConversationFlow.ConversationFlowID,
			"agent_id":            cfg.ConversationFlow.AgentID,
			"workspace_id":        cfg.ConversationFlow.WorkspaceID,
			"version":             cfg.ConversationFlow.Version,
			"nodes":               cfg.ConversationFlow.Nodes,
			"start_node_id":       cfg.ConversationFlow.StartNodeID,
			"start_speaker":       cfg.ConversationFlow.StartSpeaker,
			"global_prompt":       cfg.ConversationFlow.GlobalPrompt,
			"model_choice":        cfg.ConversationFlow.ModelChoice,
			"model_temperature":   cfg.ConversationFlow.ModelTemperature,
			"flex_mode":           cfg.ConversationFlow.FlexMode,
			"tool_call_strict_mode": cfg.ConversationFlow.ToolCallStrictMode,
			"kb_config":           cfg.ConversationFlow.KBConfig,
			"begin_tag_display_position": cfg.ConversationFlow.BeginTagDisplayPos,
			"is_transfer_cf":      cfg.ConversationFlow.IsTransferCF,
			"created_at":          cfg.ConversationFlow.CreatedAt,
			"updated_at":          cfg.ConversationFlow.UpdatedAt,
		},
		PublishedWorkflow: cfg.PublishedWorkflow,
	})
}

