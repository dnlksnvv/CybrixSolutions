package handlers

import (
	"net/http"

	"github.com/cybrix-solutions/agents-service/internal/app/usecase"
	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi/middleware"
	"github.com/gin-gonic/gin"
)

type Deps struct {
	Folders *usecase.FolderUsecase
	Agents  *usecase.AgentUsecase
	Flows   *usecase.ConversationFlowUsecase
	Runtime *usecase.RuntimeUsecase

	Published repo.PublishedWorkflowRepository
}

type API struct {
	deps Deps
}

func NewAPI(deps Deps) *API {
	return &API{deps: deps}
}

type envelope[T any] struct {
	Data T `json:"data"`
}

type meta struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

type envelopeList[T any] struct {
	Data []T  `json:"data"`
	Meta meta `json:"meta"`
}

func workspaceID(c *gin.Context) string {
	// middleware.RequireWorkspaceID уже проверил заголовок; здесь просто читаем его ещё раз.
	return c.GetHeader(middleware.HeaderWorkspaceID)
}

func writeOK[T any](c *gin.Context, status int, data T) {
	c.JSON(status, envelope[T]{Data: data})
}

func writeList[T any](c *gin.Context, status int, data []T, m meta) {
	c.JSON(status, envelopeList[T]{Data: data, Meta: m})
}

func writeNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func writeError(c *gin.Context, err error) {
	apiErr, ok := err.(derr.APIError)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": derr.APIError{
				Type:    derr.ErrorTypeInternal,
				Field:   nil,
				Code:    "internal_error",
				Message: "internal error",
				Limit:   nil,
			},
		})
		return
	}

	status := http.StatusInternalServerError
	switch apiErr.Type {
	case derr.ErrorTypeValidation:
		status = http.StatusBadRequest
	case derr.ErrorTypeBusiness:
		status = http.StatusBadRequest
	case derr.ErrorTypeNotFound:
		status = http.StatusNotFound
	case derr.ErrorTypeInternal:
		status = http.StatusInternalServerError
	}

	c.JSON(status, gin.H{"error": apiErr})
}

