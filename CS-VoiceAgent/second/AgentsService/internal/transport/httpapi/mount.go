package httpapi

import (
	"net/http"

	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi/handlers"
	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi/middleware"
	"github.com/gin-gonic/gin"
)

// MountAPI регистрирует middleware и маршруты API на r (healthz + /api/v1).
// Используется из NewRouter и из httptest-интеграционных тестов с in-memory репозиториями.
func MountAPI(r *gin.Engine, h *handlers.API, bodyLimitBytes int64) {
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.BodyLimit(bodyLimitBytes))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	runtime := api.Group("/runtime")
	protected := api.Group("")
	protected.Use(middleware.RequireWorkspaceID())

	runtime.GET("/workspaces/published-agents", h.RuntimeListPublishedAgentsByWorkspace)
	runtime.GET("/workspaces/:workspaceId/published-agents", h.RuntimeListPublishedAgentsForWorkspace)
	runtime.GET("/workspaces/:workspaceId/agents/:agentId/published-config", h.RuntimeGetPublishedConfig)

	protected.GET("/folders", h.ListFolders)
	protected.POST("/folders", h.CreateFolder)
	protected.PATCH("/folders/:folderId", h.RenameFolder)
	protected.DELETE("/folders/:folderId", h.DeleteFolder)

	protected.GET("/agents", h.ListAgents)
	protected.POST("/agents", h.CreateAgent)
	protected.GET("/agents/:agentId", h.GetAgent)
	protected.PATCH("/agents/:agentId", h.PatchAgent)
	protected.DELETE("/agents/:agentId", h.DeleteAgent)

	protected.GET("/agents/:agentId/conversation-flows", h.ListConversationFlowVersions)
	protected.GET("/agents/:agentId/conversation-flows/published", h.GetPublishedConversationFlow)
	protected.GET("/agents/:agentId/conversation-flows/:conversationFlowId", h.GetConversationFlowVersion)
	protected.PATCH("/agents/:agentId/conversation-flows/:conversationFlowId", h.PatchConversationFlowVersion)
	protected.POST("/agents/:agentId/conversation-flows/:conversationFlowId/versions", h.CreateConversationFlowVersion)
	protected.POST("/agents/:agentId/conversation-flows/:conversationFlowId/publish", h.PublishConversationFlowVersion)
	protected.POST("/agents/:agentId/conversation-flows/unpublish", h.UnpublishConversationFlow)
	protected.DELETE("/agents/:agentId/conversation-flows/:conversationFlowId", h.DeleteConversationFlowVersion)
}
