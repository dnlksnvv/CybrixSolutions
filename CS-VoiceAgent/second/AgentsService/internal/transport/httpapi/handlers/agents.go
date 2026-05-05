package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cybrix-solutions/agents-service/internal/app/usecase"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	"github.com/gin-gonic/gin"
)

type agentUpsertReq struct {
	Name     *string  `json:"name"`
	FolderID **string `json:"folder_id"`

	Channel  *string `json:"channel"`
	VoiceID  *string `json:"voice_id"`
	Language *string `json:"language"`

	TTS *models.TTSConfig `json:"tts"`
	STT *models.STTConfig `json:"stt"`

	InterruptionSensitivity *float64 `json:"interruption_sensitivity"`
	MaxCallDurationMS       *int64   `json:"max_call_duration_ms"`
	NormalizeForSpeech      *bool    `json:"normalize_for_speech"`
	AllowUserDTMF           *bool    `json:"allow_user_dtmf"`
	UserDTMFOptions         *any     `json:"user_dtmf_options"`

	HandbookConfig     *models.HandbookConfig `json:"handbook_config"`
	PIIConfig          *models.PIIConfig      `json:"pii_config"`
	DataStorageSetting *string                `json:"data_storage_setting"`
}

func (a *API) ListAgents(c *gin.Context) {
	ws := workspaceID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	var folderID *string
	if v := c.Query("folderId"); v != "" {
		folderID = &v
	}

	items, total, err := a.deps.Agents.ListLight(c.Request.Context(), ws, repo.ListAgentsQuery{
		Page:     page,
		Limit:    limit,
		FolderID: folderID,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeList(c, http.StatusOK, items, meta{Page: page, Limit: limit, Total: total})
}

func (a *API) CreateAgent(c *gin.Context) {
	ws := workspaceID(c)
	var req agentUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, badJSON(err))
		return
	}

	out, _, err := a.deps.Agents.Create(c.Request.Context(), ws, usecase.CreateAgentInput{
		Name:                   req.Name,
		FolderID:               req.FolderID,
		Channel:                req.Channel,
		VoiceID:                req.VoiceID,
		Language:               req.Language,
		TTS:                    req.TTS,
		STT:                    req.STT,
		InterruptionSensitivity: req.InterruptionSensitivity,
		MaxCallDurationMS:      req.MaxCallDurationMS,
		NormalizeForSpeech:     req.NormalizeForSpeech,
		AllowUserDTMF:          req.AllowUserDTMF,
		UserDTMFOptions:        req.UserDTMFOptions,
		HandbookConfig:         req.HandbookConfig,
		PIIConfig:              req.PIIConfig,
		DataStorageSetting:     req.DataStorageSetting,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusCreated, out)
}

func (a *API) GetAgent(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	out, err := a.deps.Agents.Get(c.Request.Context(), ws, agentID)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusOK, out)
}

func (a *API) PatchAgent(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	var req agentUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, badJSON(err))
		return
	}

	nowMs := time.Now().UnixMilli()
	patch := repo.AgentPatch{
		Name:                   req.Name,
		FolderID:               req.FolderID,
		Channel:                req.Channel,
		VoiceID:                req.VoiceID,
		Language:               req.Language,
		TTS:                    req.TTS,
		STT:                    req.STT,
		InterruptionSensitivity: req.InterruptionSensitivity,
		MaxCallDurationMS:      req.MaxCallDurationMS,
		NormalizeForSpeech:     req.NormalizeForSpeech,
		AllowUserDTMF:          req.AllowUserDTMF,
		UserDTMFOptions:        req.UserDTMFOptions,
		HandbookConfig:         req.HandbookConfig,
		PIIConfig:              req.PIIConfig,
		DataStorageSetting:     req.DataStorageSetting,
		UpdatedAt:              &nowMs,
		LastModified:           &nowMs,
	}

	out, err := a.deps.Agents.Update(c.Request.Context(), ws, agentID, usecase.UpdateAgentInput{
		Patch:      patch,
		NewFolderID: req.FolderID,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, http.StatusOK, out)
}

func (a *API) DeleteAgent(c *gin.Context) {
	ws := workspaceID(c)
	agentID := c.Param("agentId")
	if err := a.deps.Agents.Delete(c.Request.Context(), ws, agentID); err != nil {
		writeError(c, err)
		return
	}
	writeNoContent(c)
}

