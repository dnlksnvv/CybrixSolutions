package interfaces

import (
	"context"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
)

// AgentRepository — контракт доступа к агентам.
type AgentRepository interface {
	GetByID(ctx context.Context, workspaceID, agentID string) (models.Agent, bool, error)
	Create(ctx context.Context, agent models.Agent) error
	Update(ctx context.Context, workspaceID, agentID string, patch AgentPatch) (models.Agent, bool, error)
	Delete(ctx context.Context, workspaceID, agentID string) (bool, error)
	Count(ctx context.Context, workspaceID string) (int64, error)
	ListLight(ctx context.Context, workspaceID string, q ListAgentsQuery) (items []LightAgent, total int64, err error)

	// DetachFromFolder сбрасывает folder_id у всех агентов папки (используется при удалении папки).
	DetachFromFolder(ctx context.Context, workspaceID, folderID string, updatedAt int64) error
}

// ListAgentsQuery — параметры пагинации и фильтрации для GET /agents.
type ListAgentsQuery struct {
	Page     int
	Limit    int
	FolderID *string
}

// LightAgent — сокращённый DTO для списка агентов.
type LightAgent struct {
	AgentID      string  `json:"agent_id"`
	Name         string  `json:"name"`
	VoiceID      string  `json:"voice_id"`
	Language     string  `json:"language"`
	FolderID     *string `json:"folder_id"`
	LastModified int64   `json:"last_modified"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

// AgentPatch — набор top-level полей, которые нужно обновить.
// Это отражает правило из ТЗ: PATCH обновляет только явно переданные top-level поля.
type AgentPatch struct {
	Name                   *string
	FolderID               **string // nil=не менять, &nil=сбросить, &("id")=установить
	Channel                *string
	VoiceID                *string
	Language               *string
	TTS                    *models.TTSConfig
	STT                    *models.STTConfig
	InterruptionSensitivity *float64
	MaxCallDurationMS      *int64
	NormalizeForSpeech     *bool
	AllowUserDTMF          *bool
	UserDTMFOptions        *any
	ResponseEngine         *models.ResponseEngine
	HandbookConfig         *models.HandbookConfig
	PIIConfig              *models.PIIConfig
	DataStorageSetting     *string

	LastModified *int64
	UpdatedAt    *int64
}

