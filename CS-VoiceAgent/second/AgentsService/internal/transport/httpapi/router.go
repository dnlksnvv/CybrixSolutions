package httpapi

import (
	"net/http"

	"github.com/cybrix-solutions/agents-service/internal/app/usecase"
	mongorepo "github.com/cybrix-solutions/agents-service/internal/repository/mongo"
	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi/handlers"
	"github.com/gin-gonic/gin"
	driver "go.mongodb.org/mongo-driver/mongo"
)

// RouterDeps — зависимости transport-слоя.
// Здесь пока только инфраструктура; usecase и репозитории подключим позже.
type RouterDeps struct {
	BodyLimitBytes int64
	MongoClient    *driver.Client
	MongoDB        string
}

// NewRouter создаёт HTTP router сервиса (MongoDB + usecase composition root).
// Обязательный заголовок `X-Workspace-Id` применяется только к защищённой группе маршрутов; см. MountAPI.
func NewRouter(deps RouterDeps) http.Handler {
	r := gin.New()

	db := deps.MongoClient.Database(deps.MongoDB)
	foldersRepo := mongorepo.NewFolderRepository(db)
	agentsRepo := mongorepo.NewAgentRepository(db)
	flowsRepo := mongorepo.NewConversationFlowRepository(db)
	publishedRepo := mongorepo.NewPublishedWorkflowRepository(db)
	tx := mongorepo.NewTxRunner(deps.MongoClient)

	ucDeps := usecase.Deps{
		Folders:           foldersRepo,
		Agents:            agentsRepo,
		ConversationFlows: flowsRepo,
		Published:         publishedRepo,
		Tx:                tx,
	}
	foldersUC := usecase.NewFolderUsecase(ucDeps)
	agentsUC := usecase.NewAgentUsecase(ucDeps)
	flowsUC := usecase.NewConversationFlowUsecase(ucDeps)
	runtimeUC := usecase.NewRuntimeUsecase(ucDeps)

	h := handlers.NewAPI(handlers.Deps{
		Folders:   foldersUC,
		Agents:    agentsUC,
		Flows:     flowsUC,
		Runtime:   runtimeUC,
		Published: publishedRepo,
	})

	MountAPI(r, h, deps.BodyLimitBytes)
	return r
}
