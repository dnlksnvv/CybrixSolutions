package handlers

import (
	"net/http"
	"strconv"
	"time"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	"github.com/gin-gonic/gin"
)

type conversationFlowPatchReq struct {
	Nodes                  *[]models.Node      `json:"nodes"`
	StartNodeID             *string            `json:"start_node_id"`
	StartSpeaker            *string            `json:"start_speaker"`
	GlobalPrompt            *string            `json:"global_prompt"`
	ModelChoice             *models.ModelChoice `json:"model_choice"`
	ModelTemperature        *float64           `json:"model_temperature"`
	FlexMode                *bool              `json:"flex_mode"`
	ToolCallStrictMode      *bool              `json:"tool_call_strict_mode"`
	KBConfig                *models.KBConfig   `json:"kb_config"`
	BeginTagDisplayPosition *models.Position   `json:"begin_tag_display_position"`
	IsTransferCF            *bool              `json:"is_transfer_cf"`
}

type conversationFlowResp struct {
	models.ConversationFlow
	Published bool `json:"published"`
	ResponseEngine models.ResponseEngine `json:"response_engine"`
}

func (a *API) ListConversationFlowVersions(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	items, err := a.deps.Flows.ListVersions(c.Request.Context(), ws, agentID)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusOK, items)
}

func (a *API) GetPublishedConversationFlow(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	cf, err := a.deps.Flows.GetPublished(c.Request.Context(), ws, agentID)
	if err != nil {
		writeError(c, err)
		return
	}
	// published version выбирается из PublishedWorkflow.
	pw, _, _ := a.deps.Published.Get(c.Request.Context(), ws, agentID)
	writeOK(c, http.StatusOK, conversationFlowResp{
		ConversationFlow: cf,
		Published:        true,
		ResponseEngine: models.ResponseEngine{
			Type:               "conversation-flow",
			ConversationFlowID: pw.ConversationFlowID,
			Version:            pw.Version,
		},
	})
}

func (a *API) GetConversationFlowVersion(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	flowID := c.Param("conversationFlowId")
	v, err := strconv.Atoi(c.Query("version"))
	if err != nil {
		writeError(c, derr.NewValidation("version", "required", "version query param is required", nil))
		return
	}

	cf, _, err := a.deps.Flows.GetVersion(c.Request.Context(), ws, agentID, flowID, v)
	if err != nil {
		writeError(c, err)
		return
	}

	published := false
	if pw, ok, _ := a.deps.Published.Get(c.Request.Context(), ws, agentID); ok {
		if pw.ConversationFlowID == flowID && pw.Version == v {
			published = true
		}
	}

	// Для editor сценариев response_engine должен отражать явно запрошенную версию.
	writeOK(c, http.StatusOK, conversationFlowResp{
		ConversationFlow: cf,
		Published:        published,
		ResponseEngine: models.ResponseEngine{
			Type:               "conversation-flow",
			ConversationFlowID: flowID,
			Version:            v,
		},
	})
}

func (a *API) PatchConversationFlowVersion(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	flowID := c.Param("conversationFlowId")
	v, err := strconv.Atoi(c.Query("version"))
	if err != nil {
		writeError(c, derr.NewValidation("version", "required", "version query param is required", nil))
		return
	}
	var req conversationFlowPatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, badJSON(err))
		return
	}

	nowMs := time.Now().UnixMilli()
	out, err := a.deps.Flows.PatchVersion(c.Request.Context(), ws, agentID, flowID, v, repo.ConversationFlowPatch{
		Nodes:                  req.Nodes,
		StartNodeID:             req.StartNodeID,
		StartSpeaker:            req.StartSpeaker,
		GlobalPrompt:            req.GlobalPrompt,
		ModelChoice:             req.ModelChoice,
		ModelTemperature:        req.ModelTemperature,
		FlexMode:                req.FlexMode,
		ToolCallStrictMode:      req.ToolCallStrictMode,
		KBConfig:                req.KBConfig,
		BeginTagDisplayPosition: req.BeginTagDisplayPosition,
		IsTransferCF:            req.IsTransferCF,
		UpdatedAt:               &nowMs,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	// Если PATCH прошёл, версия по определению непубликованная (иначе usecase вернул бы ошибку).
	writeOK(c, http.StatusOK, conversationFlowResp{
		ConversationFlow: out,
		Published:        false,
		ResponseEngine: models.ResponseEngine{
			Type:               "conversation-flow",
			ConversationFlowID: flowID,
			Version:            v,
		},
	})
}

func (a *API) CreateConversationFlowVersion(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	flowID := c.Param("conversationFlowId")
	fromV, err := strconv.Atoi(c.Query("fromVersion"))
	if err != nil {
		writeError(c, derr.NewValidation("fromVersion", "required", "fromVersion query param is required", nil))
		return
	}
	out, err := a.deps.Flows.CreateVersionFrom(c.Request.Context(), ws, agentID, flowID, fromV)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusCreated, conversationFlowResp{
		ConversationFlow: out,
		Published:        false,
		ResponseEngine: models.ResponseEngine{
			Type:               "conversation-flow",
			ConversationFlowID: flowID,
			Version:            out.Version,
		},
	})
}

type publishResp struct {
	AgentID            string `json:"agent_id"`
	ConversationFlowID string `json:"conversation_flow_id"`
	Version            int    `json:"version"`
	Published          bool   `json:"published"`
	PublishedAt        int64  `json:"published_at"`
}

func (a *API) PublishConversationFlowVersion(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	flowID := c.Param("conversationFlowId")
	v, err := strconv.Atoi(c.Query("version"))
	if err != nil {
		writeError(c, derr.NewValidation("version", "required", "version query param is required", nil))
		return
	}
	pw, err := a.deps.Flows.Publish(c.Request.Context(), ws, agentID, flowID, v)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusOK, publishResp{
		AgentID:            pw.AgentID,
		ConversationFlowID: pw.ConversationFlowID,
		Version:            pw.Version,
		Published:          true,
		PublishedAt:        pw.PublishedAt,
	})
}

func (a *API) UnpublishConversationFlow(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	if err := a.deps.Flows.Unpublish(c.Request.Context(), ws, agentID); err != nil {
		writeError(c, err)
		return
	}
	writeNoContent(c)
}

func (a *API) DeleteConversationFlowVersion(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	flowID := c.Param("conversationFlowId")
	v, err := strconv.Atoi(c.Query("version"))
	if err != nil {
		writeError(c, derr.NewValidation("version", "required", "version query param is required", nil))
		return
	}
	if err := a.deps.Flows.DeleteVersion(c.Request.Context(), ws, agentID, flowID, v); err != nil {
		writeError(c, err)
		return
	}
	writeNoContent(c)
}

