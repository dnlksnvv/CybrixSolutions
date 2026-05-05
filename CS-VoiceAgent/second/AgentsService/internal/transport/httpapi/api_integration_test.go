package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cybrix-solutions/agents-service/internal/app/usecase"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	"github.com/cybrix-solutions/agents-service/internal/domain/validation"
	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi/handlers"
	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var fixedTestTime = time.Unix(1700000000, 0)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newTestRouter(tb testing.TB, bodyLimit int64) (http.Handler, *testMemoryStore) {
	tb.Helper()
	store := newTestMemoryStore()
	ucDeps := usecase.Deps{
		Folders:           store.folderRepo(),
		Agents:            store.agentRepo(),
		ConversationFlows: store.flowRepo(),
		Published:         store.publishedRepo(),
		Tx:                memTx{},
		Now:               func() time.Time { return fixedTestTime },
	}
	pub := store.publishedRepo()
	h := handlers.NewAPI(handlers.Deps{
		Folders:   usecase.NewFolderUsecase(ucDeps),
		Agents:    usecase.NewAgentUsecase(ucDeps),
		Flows:     usecase.NewConversationFlowUsecase(ucDeps),
		Runtime:   usecase.NewRuntimeUsecase(ucDeps),
		Published: pub,
	})
	r := gin.New()
	MountAPI(r, h, bodyLimit)
	return r, store
}

func doREQ(tb testing.TB, r http.Handler, method, path string, body []byte, hdr map[string]string) *httptest.ResponseRecorder {
	tb.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}

type errEnvelope struct {
	Error struct {
		Type string `json:"type"`
		Code string `json:"code"`
	} `json:"error"`
}

func decodeErr(tb testing.TB, raw []byte) errEnvelope {
	tb.Helper()
	var e errEnvelope
	require.NoError(tb, json.Unmarshal(raw, &e))
	return e
}

// A: runtime без X-Workspace-Id; protected без заголовка → missing_workspace_id
func TestAPI_RuntimeVsProtectedWorkspaceHeader(t *testing.T) {
	r, _ := newTestRouter(t, 8*1024*1024)

	w := doREQ(t, r, http.MethodGet, "/api/v1/runtime/workspaces/published-agents", nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Data []any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.NotNil(t, env.Data)

	w = doREQ(t, r, http.MethodGet, "/api/v1/agents", nil, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	e := decodeErr(t, w.Body.Bytes())
	require.Equal(t, "validation_error", e.Error.Type)
	require.Equal(t, "missing_workspace_id", e.Error.Code)
}

// B: happy path — создание агента, список версий, GET v0 (published + response_engine), runtime published-config
func TestAPI_FullHappyPath_AgentFlowAndRuntimeConfig(t *testing.T) {
	r, _ := newTestRouter(t, 8*1024*1024)
	ws := "ws-happy"
	hdr := map[string]string{middleware.HeaderWorkspaceID: ws}

	w := doREQ(t, r, http.MethodPost, "/api/v1/agents", []byte(`{}`), hdr)
	require.Equal(t, http.StatusCreated, w.Code)
	var createWrap struct {
		Data models.Agent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createWrap))
	agent := createWrap.Data
	require.NotEmpty(t, agent.AgentID)
	require.Equal(t, ws, agent.WorkspaceID)
	require.Equal(t, "conversation-flow", agent.ResponseEngine.Type)
	require.NotEmpty(t, agent.ResponseEngine.ConversationFlowID)
	require.Equal(t, 0, agent.ResponseEngine.Version)

	flowID := agent.ResponseEngine.ConversationFlowID
	agentID := agent.AgentID

	w = doREQ(t, r, http.MethodGet, "/api/v1/agents/"+agentID+"/conversation-flows", nil, hdr)
	require.Equal(t, http.StatusOK, w.Code)
	var listWrap struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listWrap))
	require.Len(t, listWrap.Data, 1)
	require.EqualValues(t, true, listWrap.Data[0]["published"])

	w = doREQ(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", agentID, flowID), nil, hdr)
	require.Equal(t, http.StatusOK, w.Code)
	var flowWrap struct {
		Data struct {
			Published      bool                  `json:"published"`
			ResponseEngine models.ResponseEngine `json:"response_engine"`
			Version        int                   `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &flowWrap))
	require.True(t, flowWrap.Data.Published)
	require.Equal(t, flowID, flowWrap.Data.ResponseEngine.ConversationFlowID)
	require.Equal(t, 0, flowWrap.Data.ResponseEngine.Version)

	w = doREQ(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/runtime/workspaces/%s/agents/%s/published-config", ws, agentID), nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var rtWrap struct {
		Data struct {
			Agent             map[string]any `json:"agent"`
			ConversationFlow  map[string]any `json:"conversation_flow"`
			PublishedWorkflow map[string]any `json:"published_workflow"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rtWrap))
	require.Contains(t, rtWrap.Data.Agent, "agent_id")
	require.Equal(t, agentID, rtWrap.Data.Agent["agent_id"])
	require.Equal(t, true, rtWrap.Data.ConversationFlow["published"])
	require.Contains(t, rtWrap.Data.PublishedWorkflow, "version")
}

// C: unpublish → frontend GET всё ещё ок с published=false; runtime published-config → 404
func TestAPI_UnpublishPath(t *testing.T) {
	r, _ := newTestRouter(t, 8*1024*1024)
	ws := "ws-unpub"
	hdr := map[string]string{middleware.HeaderWorkspaceID: ws}

	w := doREQ(t, r, http.MethodPost, "/api/v1/agents", []byte(`{}`), hdr)
	require.Equal(t, http.StatusCreated, w.Code)
	var aw struct {
		Data models.Agent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aw))
	agentID := aw.Data.AgentID
	flowID := aw.Data.ResponseEngine.ConversationFlowID

	w = doREQ(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/unpublish", agentID), []byte(`{}`), hdr)
	require.Equal(t, http.StatusNoContent, w.Code)

	w = doREQ(t, r, http.MethodGet, "/api/v1/agents/"+agentID, nil, hdr)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aw))
	require.Equal(t, flowID, aw.Data.ResponseEngine.ConversationFlowID)
	require.Equal(t, 0, aw.Data.ResponseEngine.Version)

	w = doREQ(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", agentID, flowID), nil, hdr)
	require.Equal(t, http.StatusOK, w.Code)
	var fw struct {
		Data struct {
			Published bool `json:"published"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fw))
	require.False(t, fw.Data.Published)

	w = doREQ(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/runtime/workspaces/%s/agents/%s/published-config", ws, agentID), nil, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	e := decodeErr(t, w.Body.Bytes())
	require.Equal(t, "not_found", e.Error.Type)
	require.Equal(t, "published_workflow_not_found", e.Error.Code)
}

// D: защита опубликованной v0 и удаления агента при наличии PublishedWorkflow
func TestAPI_PublishedVersionProtectionAndAgentDeleteBlocked(t *testing.T) {
	r, _ := newTestRouter(t, 8*1024*1024)
	ws := "ws-prot"
	hdr := map[string]string{middleware.HeaderWorkspaceID: ws}

	w := doREQ(t, r, http.MethodPost, "/api/v1/agents", []byte(`{}`), hdr)
	require.Equal(t, http.StatusCreated, w.Code)
	var aw struct {
		Data models.Agent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aw))
	agentID := aw.Data.AgentID
	flowID := aw.Data.ResponseEngine.ConversationFlowID

	w = doREQ(t, r, http.MethodPatch,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", agentID, flowID),
		[]byte(`{"global_prompt":"x"}`), hdr)
	require.Equal(t, http.StatusBadRequest, w.Code)
	e := decodeErr(t, w.Body.Bytes())
	require.Equal(t, "business_error", e.Error.Type)
	require.Equal(t, "published_version_is_readonly", e.Error.Code)

	w = doREQ(t, r, http.MethodDelete,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", agentID, flowID), nil, hdr)
	require.Equal(t, http.StatusBadRequest, w.Code)
	e = decodeErr(t, w.Body.Bytes())
	require.Equal(t, "business_error", e.Error.Type)
	require.Equal(t, "cannot_delete_published_version", e.Error.Code)

	w = doREQ(t, r, http.MethodDelete, "/api/v1/agents/"+agentID, nil, hdr)
	require.Equal(t, http.StatusBadRequest, w.Code)
	e = decodeErr(t, w.Body.Bytes())
	require.Equal(t, "business_error", e.Error.Type)
	require.Equal(t, "agent_has_published_workflow", e.Error.Code)
}

// E: >1000 nodes → validation_error (max_items_exceeded); oversize workflow JSON → workflow_size_exceeded
func TestAPI_Validation_NodeLimitAndWorkflowSize(t *testing.T) {
	r, _ := newTestRouter(t, 20*1024*1024)
	ws := "ws-val"
	hdr := map[string]string{middleware.HeaderWorkspaceID: ws}

	w := doREQ(t, r, http.MethodPost, "/api/v1/agents", []byte(`{}`), hdr)
	require.Equal(t, http.StatusCreated, w.Code)
	var aw struct {
		Data models.Agent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aw))
	agentID := aw.Data.AgentID
	flowID := aw.Data.ResponseEngine.ConversationFlowID

	w = doREQ(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/unpublish", agentID), []byte(`{}`), hdr)
	require.Equal(t, http.StatusNoContent, w.Code)

	nodes := make([]models.Node, 1001)
	for i := range nodes {
		id := fmt.Sprintf("n%d", i)
		nodes[i] = models.Node{
			ID:   id,
			Type: "conversation",
			Name: "N",
			Instruction: models.Instruction{
				Type: "static_text",
				Text: "hi",
			},
			Edges: []models.Edge{},
		}
	}
	start := "n0"
	patchNodes := map[string]any{
		"nodes":         nodes,
		"start_node_id": start,
		"start_speaker": "agent",
		"global_prompt": "p",
	}
	body1001, err := json.Marshal(patchNodes)
	require.NoError(t, err)
	w = doREQ(t, r, http.MethodPatch,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", agentID, flowID),
		body1001, hdr)
	require.Equal(t, http.StatusBadRequest, w.Code)
	e := decodeErr(t, w.Body.Bytes())
	require.Equal(t, "validation_error", e.Error.Type)
	require.Equal(t, "max_items_exceeded", e.Error.Code)

	hugeText := strings.Repeat("x", validation.MaxWorkflowJSONBytes+2048)
	patchHuge := map[string]any{
		"nodes": []models.Node{
			{
				ID:   "n1",
				Type: "conversation",
				Name: "Start",
				Instruction: models.Instruction{
					Type: "static_text",
					Text: hugeText,
				},
				Edges: []models.Edge{},
			},
		},
		"start_node_id": "n1",
	}
	bodyHuge, err := json.Marshal(patchHuge)
	require.NoError(t, err)
	w = doREQ(t, r, http.MethodPatch,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", agentID, flowID),
		bodyHuge, hdr)
	require.Equal(t, http.StatusBadRequest, w.Code)
	e = decodeErr(t, w.Body.Bytes())
	require.Equal(t, "validation_error", e.Error.Type)
	require.Equal(t, "workflow_size_exceeded", e.Error.Code)
}

// POST publish обновляет PublishedWorkflow и agent.response_engine (переход на v1)
func TestAPI_PublishNewVersionUpdatesAgentResponseEngine(t *testing.T) {
	r, _ := newTestRouter(t, 8*1024*1024)
	ws := "ws-pub"
	hdr := map[string]string{middleware.HeaderWorkspaceID: ws}

	w := doREQ(t, r, http.MethodPost, "/api/v1/agents", []byte(`{}`), hdr)
	require.Equal(t, http.StatusCreated, w.Code)
	var aw struct {
		Data models.Agent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aw))
	agentID := aw.Data.AgentID
	flowID := aw.Data.ResponseEngine.ConversationFlowID

	w = doREQ(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s/versions?fromVersion=0", agentID, flowID),
		nil, hdr)
	require.Equal(t, http.StatusCreated, w.Code)

	w = doREQ(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s/publish?version=1", agentID, flowID),
		nil, hdr)
	require.Equal(t, http.StatusOK, w.Code)

	w = doREQ(t, r, http.MethodGet, "/api/v1/agents/"+agentID, nil, hdr)
	require.Equal(t, http.StatusOK, w.Code)
	var gw struct {
		Data models.Agent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gw))
	require.Equal(t, 1, gw.Data.ResponseEngine.Version)
	require.Equal(t, flowID, gw.Data.ResponseEngine.ConversationFlowID)
}
