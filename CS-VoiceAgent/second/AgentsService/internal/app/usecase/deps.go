package usecase

import (
	"time"

	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
)

// Deps — зависимости usecase-слоя.
// Мы группируем их в одну структуру, чтобы конструирование приложения оставалось простым.
type Deps struct {
	Folders          repo.FolderRepository
	Agents           repo.AgentRepository
	ConversationFlows repo.ConversationFlowRepository
	Published        repo.PublishedWorkflowRepository
	Tx              repo.TxRunner

	Now func() time.Time
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

