//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	"github.com/cybrix-solutions/agents-service/internal/domain/validation"
	"github.com/cybrix-solutions/agents-service/internal/transport/httpapi/middleware"
	"github.com/stretchr/testify/require"
)

func testBaseURL(t *testing.T) string {
	t.Helper()
	u := strings.TrimSuffix(os.Getenv("AGENTS_SERVICE_BASE_URL"), "/")
	if u == "" {
		u = "http://localhost:8080"
	}
	return u
}

func testWorkspaceID(t *testing.T) string {
	t.Helper()
	ws := strings.TrimSpace(os.Getenv("AGENTS_SERVICE_WORKSPACE_ID"))
	if ws == "" {
		return fmt.Sprintf("ws_e2e_%d", time.Now().UnixNano())
	}
	return ws
}

func testHTTPClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Minute}
}

type errObj struct {
	Type    string  `json:"type"`
	Code    string  `json:"code"`
	Field   *string `json:"field"`
	Limit   *int    `json:"limit"`
	Message string  `json:"message"`
}

type errEnvelope struct {
	Error errObj `json:"error"`
}

func req(t *testing.T, c *http.Client, method, baseURL, path string, body []byte, hdr map[string]string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	require.NoError(t, err)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(r)
	require.NoError(t, err)
	return res
}

func readBody(t *testing.T, res *http.Response) []byte {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	_ = res.Body.Close()
	return b
}

func wsHdr(ws string) map[string]string {
	return map[string]string{middleware.HeaderWorkspaceID: ws}
}

// TestE2E_SmokeAPI покрывает сценарии из docs/api-test.md (основной happy-path, runtime, publish, unpublish, запреты, cleanup, папки, лимиты).
func TestE2E_SmokeAPI(t *testing.T) {
	base := testBaseURL(t)
	ws := testWorkspaceID(t)
	c := testHTTPClient()

	t.Run("healthz", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base, "/healthz", nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("runtime_without_workspace_header", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base, "/api/v1/runtime/workspaces/published-agents", nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data []any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		// Пустой список может прийти как null (nil slice) или как []
		if env.Data == nil {
			env.Data = []any{}
		}
	})

	t.Run("protected_missing_workspace", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base, "/api/v1/agents", nil, nil)
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "missing_workspace_id", e.Error.Code)
	})

	var agentID, flowID string

	t.Run("create_agent", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base, "/api/v1/agents", []byte(`{}`), wsHdr(ws))
		require.Equal(t, http.StatusCreated, res.StatusCode)
		var env struct {
			Data struct {
				AgentID        string `json:"agent_id"`
				WorkspaceID    string `json:"workspace_id"`
				ResponseEngine struct {
					Type               string `json:"type"`
					ConversationFlowID string `json:"conversation_flow_id"`
					Version            int    `json:"version"`
				} `json:"response_engine"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.NotEmpty(t, env.Data.AgentID)
		require.Equal(t, ws, env.Data.WorkspaceID)
		require.Equal(t, "conversation-flow", env.Data.ResponseEngine.Type)
		require.NotEmpty(t, env.Data.ResponseEngine.ConversationFlowID)
		require.Equal(t, 0, env.Data.ResponseEngine.Version)
		agentID = env.Data.AgentID
		flowID = env.Data.ResponseEngine.ConversationFlowID
	})

	t.Run("list_versions_v0_published", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base, "/api/v1/agents/"+agentID+"/conversation-flows", nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data []struct {
				ConversationFlowID string `json:"conversation_flow_id"`
				Version            int    `json:"version"`
				Published          bool   `json:"published"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		var v0 *struct {
			ConversationFlowID string `json:"conversation_flow_id"`
			Version            int    `json:"version"`
			Published          bool   `json:"published"`
		}
		for i := range env.Data {
			if env.Data[i].Version == 0 && env.Data[i].ConversationFlowID == flowID {
				v0 = &env.Data[i]
				break
			}
		}
		require.NotNil(t, v0)
		require.True(t, v0.Published)
	})

	t.Run("get_flow_v0_editor", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", agentID, flowID), nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data struct {
				ConversationFlowID string `json:"conversation_flow_id"`
				AgentID            string `json:"agent_id"`
				Version            int    `json:"version"`
				Published          bool   `json:"published"`
				ResponseEngine     struct {
					Type               string `json:"type"`
					ConversationFlowID string `json:"conversation_flow_id"`
					Version            int    `json:"version"`
				} `json:"response_engine"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.Equal(t, flowID, env.Data.ConversationFlowID)
		require.Equal(t, agentID, env.Data.AgentID)
		require.Equal(t, 0, env.Data.Version)
		require.True(t, env.Data.Published)
		require.Equal(t, flowID, env.Data.ResponseEngine.ConversationFlowID)
		require.Equal(t, 0, env.Data.ResponseEngine.Version)
	})

	t.Run("runtime_published_config_v0", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base,
			fmt.Sprintf("/api/v1/runtime/workspaces/%s/agents/%s/published-config", ws, agentID), nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data struct {
				Agent struct {
					AgentID        string `json:"agent_id"`
					ResponseEngine struct {
						ConversationFlowID string `json:"conversation_flow_id"`
						Version            int    `json:"version"`
					} `json:"response_engine"`
				} `json:"agent"`
				ConversationFlow struct {
					ConversationFlowID string `json:"conversation_flow_id"`
					Version            int    `json:"version"`
					Published          bool   `json:"published"`
				} `json:"conversation_flow"`
				PublishedWorkflow struct {
					AgentID            string `json:"agent_id"`
					ConversationFlowID string `json:"conversation_flow_id"`
					Version            int    `json:"version"`
				} `json:"published_workflow"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.Equal(t, agentID, env.Data.Agent.AgentID)
		require.Equal(t, flowID, env.Data.ConversationFlow.ConversationFlowID)
		require.Equal(t, 0, env.Data.ConversationFlow.Version)
		require.True(t, env.Data.ConversationFlow.Published)
		require.Equal(t, agentID, env.Data.PublishedWorkflow.AgentID)
		require.Equal(t, flowID, env.Data.PublishedWorkflow.ConversationFlowID)
		require.Equal(t, 0, env.Data.PublishedWorkflow.Version)
		require.Equal(t, flowID, env.Data.Agent.ResponseEngine.ConversationFlowID)
		require.Equal(t, 0, env.Data.Agent.ResponseEngine.Version)
	})

	t.Run("runtime_workspace_published_agents", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base,
			fmt.Sprintf("/api/v1/runtime/workspaces/%s/published-agents", ws), nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data []struct {
				AgentID            string `json:"agent_id"`
				ConversationFlowID string `json:"conversation_flow_id"`
				Version            int    `json:"version"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		found := false
		for _, row := range env.Data {
			if row.AgentID == agentID {
				require.Equal(t, flowID, row.ConversationFlowID)
				require.Equal(t, 0, row.Version)
				found = true
			}
		}
		require.True(t, found, "agent in workspace published list")
	})

	t.Run("runtime_all_workspaces_includes_agent", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base, "/api/v1/runtime/workspaces/published-agents", nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data []struct {
				WorkspaceID string `json:"workspace_id"`
				Agents      []struct {
					AgentID            string `json:"agent_id"`
					ConversationFlowID string `json:"conversation_flow_id"`
					Version            int    `json:"version"`
				} `json:"agents"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		found := false
		for _, g := range env.Data {
			if g.WorkspaceID != ws {
				continue
			}
			for _, a := range g.Agents {
				if a.AgentID == agentID && a.ConversationFlowID == flowID && a.Version == 0 {
					found = true
				}
			}
		}
		require.True(t, found, "workspace group contains agent v0")
	})

	t.Run("create_version_v1", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s/versions?fromVersion=0", agentID, flowID), nil, wsHdr(ws))
		require.Equal(t, http.StatusCreated, res.StatusCode)
		var env struct {
			Data struct {
				ConversationFlowID string `json:"conversation_flow_id"`
				Version            int    `json:"version"`
				Published          bool   `json:"published"`
				ResponseEngine     struct {
					ConversationFlowID string `json:"conversation_flow_id"`
					Version            int    `json:"version"`
				} `json:"response_engine"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.Equal(t, flowID, env.Data.ConversationFlowID)
		require.Equal(t, 1, env.Data.Version)
		require.False(t, env.Data.Published)
		require.Equal(t, 1, env.Data.ResponseEngine.Version)
	})

	t.Run("patch_v1_unpublished", func(t *testing.T) {
		body := []byte(`{"global_prompt":"E2E updated prompt"}`)
		res := req(t, c, http.MethodPatch, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=1", agentID, flowID), body, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data struct {
				Version        int    `json:"version"`
				Published      bool   `json:"published"`
				GlobalPrompt   string `json:"global_prompt"`
				ResponseEngine struct {
					Version int `json:"version"`
				} `json:"response_engine"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.Equal(t, 1, env.Data.Version)
		require.False(t, env.Data.Published)
		require.Equal(t, "E2E updated prompt", env.Data.GlobalPrompt)
		require.Equal(t, 1, env.Data.ResponseEngine.Version)
	})

	t.Run("runtime_still_v0_before_publish_v1", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base,
			fmt.Sprintf("/api/v1/runtime/workspaces/%s/agents/%s/published-config", ws, agentID), nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data struct {
				ConversationFlow struct {
					Version int `json:"version"`
				} `json:"conversation_flow"`
				PublishedWorkflow struct {
					Version int `json:"version"`
				} `json:"published_workflow"`
				Agent struct {
					ResponseEngine struct {
						Version int `json:"version"`
					} `json:"response_engine"`
				} `json:"agent"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.Equal(t, 0, env.Data.ConversationFlow.Version)
		require.Equal(t, 0, env.Data.PublishedWorkflow.Version)
		require.Equal(t, 0, env.Data.Agent.ResponseEngine.Version)
	})

	t.Run("publish_v1", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s/publish?version=1", agentID, flowID), nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data struct {
				AgentID            string `json:"agent_id"`
				ConversationFlowID string `json:"conversation_flow_id"`
				Version            int    `json:"version"`
				Published          bool   `json:"published"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.Equal(t, agentID, env.Data.AgentID)
		require.Equal(t, flowID, env.Data.ConversationFlowID)
		require.Equal(t, 1, env.Data.Version)
		require.True(t, env.Data.Published)
	})

	t.Run("runtime_published_config_v1", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base,
			fmt.Sprintf("/api/v1/runtime/workspaces/%s/agents/%s/published-config", ws, agentID), nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data struct {
				ConversationFlow struct {
					Version      int    `json:"version"`
					GlobalPrompt string `json:"global_prompt"`
				} `json:"conversation_flow"`
				PublishedWorkflow struct {
					Version int `json:"version"`
				} `json:"published_workflow"`
				Agent struct {
					ResponseEngine struct {
						Version int `json:"version"`
					} `json:"response_engine"`
				} `json:"agent"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		require.Equal(t, 1, env.Data.ConversationFlow.Version)
		require.Equal(t, "E2E updated prompt", env.Data.ConversationFlow.GlobalPrompt)
		require.Equal(t, 1, env.Data.PublishedWorkflow.Version)
		require.Equal(t, 1, env.Data.Agent.ResponseEngine.Version)
	})

	t.Run("list_versions_v0_false_v1_true", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base, "/api/v1/agents/"+agentID+"/conversation-flows", nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data []struct {
				Version   int  `json:"version"`
				Published bool `json:"published"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		var pub0, pub1 *bool
		for _, row := range env.Data {
			if row.Version == 0 {
				p := row.Published
				pub0 = &p
			}
			if row.Version == 1 {
				p := row.Published
				pub1 = &p
			}
		}
		require.NotNil(t, pub0)
		require.NotNil(t, pub1)
		require.False(t, *pub0)
		require.True(t, *pub1)
	})

	t.Run("patch_v1_readonly", func(t *testing.T) {
		res := req(t, c, http.MethodPatch, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=1", agentID, flowID),
			[]byte(`{"global_prompt":"Should not be saved"}`), wsHdr(ws))
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "published_version_is_readonly", e.Error.Code)
	})

	t.Run("delete_v1_published_forbidden", func(t *testing.T) {
		res := req(t, c, http.MethodDelete, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=1", agentID, flowID), nil, wsHdr(ws))
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "cannot_delete_published_version", e.Error.Code)
	})

	t.Run("delete_agent_while_published_forbidden", func(t *testing.T) {
		res := req(t, c, http.MethodDelete, base, "/api/v1/agents/"+agentID, nil, wsHdr(ws))
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "agent_has_published_workflow", e.Error.Code)
	})

	t.Run("unpublish", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/unpublish", agentID), []byte(`{}`), wsHdr(ws))
		require.Equal(t, http.StatusNoContent, res.StatusCode)
	})

	t.Run("frontend_flows_after_unpublish", func(t *testing.T) {
		for _, ver := range []int{0, 1} {
			res := req(t, c, http.MethodGet, base,
				fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=%d", agentID, flowID, ver), nil, wsHdr(ws))
			require.Equal(t, http.StatusOK, res.StatusCode)
			var env struct {
				Data struct {
					Version        int  `json:"version"`
					Published      bool `json:"published"`
					ResponseEngine struct {
						Version int `json:"version"`
					} `json:"response_engine"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(readBody(t, res), &env))
			require.Equal(t, ver, env.Data.Version)
			require.False(t, env.Data.Published)
			require.Equal(t, ver, env.Data.ResponseEngine.Version)
		}
	})

	t.Run("runtime_published_config_after_unpublish", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base,
			fmt.Sprintf("/api/v1/runtime/workspaces/%s/agents/%s/published-config", ws, agentID), nil, nil)
		require.Equal(t, http.StatusNotFound, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "published_workflow_not_found", e.Error.Code)
	})

	t.Run("runtime_lists_no_longer_contain_agent", func(t *testing.T) {
		res := req(t, c, http.MethodGet, base,
			fmt.Sprintf("/api/v1/runtime/workspaces/%s/published-agents", ws), nil, nil)
		require.Equal(t, http.StatusOK, res.StatusCode)
		var env struct {
			Data []struct {
				AgentID string `json:"agent_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		for _, row := range env.Data {
			require.NotEqual(t, agentID, row.AgentID)
		}
	})

	t.Run("delete_v1_after_unpublish", func(t *testing.T) {
		res := req(t, c, http.MethodDelete, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=1", agentID, flowID), nil, wsHdr(ws))
		require.Equal(t, http.StatusNoContent, res.StatusCode)
	})

	t.Run("delete_agent_after_cleanup", func(t *testing.T) {
		res := req(t, c, http.MethodDelete, base, "/api/v1/agents/"+agentID, nil, wsHdr(ws))
		require.Equal(t, http.StatusNoContent, res.StatusCode)
	})

	// --- Отдельный агент для лимитов (после удаления основного)
	var limitAgentID, limitFlowID string
	t.Run("create_agent_for_limits", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base, "/api/v1/agents", []byte(`{}`), wsHdr(ws))
		require.Equal(t, http.StatusCreated, res.StatusCode)
		var env struct {
			Data struct {
				AgentID        string `json:"agent_id"`
				ResponseEngine struct {
					ConversationFlowID string `json:"conversation_flow_id"`
				} `json:"response_engine"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		limitAgentID = env.Data.AgentID
		limitFlowID = env.Data.ResponseEngine.ConversationFlowID
	})

	t.Run("unpublish_for_limit_agent", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/unpublish", limitAgentID), []byte(`{}`), wsHdr(ws))
		require.Equal(t, http.StatusNoContent, res.StatusCode)
	})

	t.Run("nodes_over_1000_validation", func(t *testing.T) {
		nodes := make([]models.Node, 1001)
		for i := range nodes {
			id := fmt.Sprintf("n%d", i)
			nodes[i] = models.Node{
				ID:   id,
				Type: "conversation",
				Name: "N",
				Instruction: models.Instruction{
					Type: "static_text",
					Text: "x",
				},
				Edges: []models.Edge{},
			}
		}
		payload := map[string]any{
			"nodes":         nodes,
			"start_node_id": "n0",
			"start_speaker": "agent",
			"global_prompt": "p",
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		res := req(t, c, http.MethodPatch, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", limitAgentID, limitFlowID),
			body, wsHdr(ws))
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "validation_error", e.Error.Type)
		require.Equal(t, "max_items_exceeded", e.Error.Code)
		require.NotNil(t, e.Error.Field)
		require.Equal(t, "nodes", *e.Error.Field)
	})

	t.Run("workflow_size_exceeded", func(t *testing.T) {
		huge := strings.Repeat("x", validation.MaxWorkflowJSONBytes+2048)
		payload := map[string]any{
			"nodes": []models.Node{
				{
					ID:   "n1",
					Type: "conversation",
					Name: "Start",
					Instruction: models.Instruction{
						Type: "static_text",
						Text: huge,
					},
					Edges: []models.Edge{},
				},
			},
			"start_node_id": "n1",
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		res := req(t, c, http.MethodPatch, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=0", limitAgentID, limitFlowID),
			body, wsHdr(ws))
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "validation_error", e.Error.Type)
		require.Equal(t, "workflow_size_exceeded", e.Error.Code)
		require.NotNil(t, e.Error.Field)
		require.Equal(t, "workflow", *e.Error.Field)
		if e.Error.Limit != nil {
			require.Equal(t, validation.MaxWorkflowJSONBytes, *e.Error.Limit)
		}
	})

	t.Run("folder_ops_and_template_guard", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base, "/api/v1/folders",
			[]byte(`{"name":"E2E Folder"}`), wsHdr(ws))
		require.Equal(t, http.StatusCreated, res.StatusCode)
		var env struct {
			Data struct {
				FolderID string `json:"folder_id"`
				Name     string `json:"name"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &env))
		folderID := env.Data.FolderID

		res = req(t, c, http.MethodGet, base, "/api/v1/folders", nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)

		// Новый агент для привязки к папке
		res = req(t, c, http.MethodPost, base, "/api/v1/agents", []byte(`{}`), wsHdr(ws))
		require.Equal(t, http.StatusCreated, res.StatusCode)
		var ag struct {
			Data struct {
				AgentID string `json:"agent_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &ag))
		folderAgentID := ag.Data.AgentID

		patchBody := fmt.Sprintf(`{"folder_id":%q}`, folderID)
		res = req(t, c, http.MethodPatch, base, "/api/v1/agents/"+folderAgentID, []byte(patchBody), wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)

		res = req(t, c, http.MethodGet, base, "/api/v1/agents?page=1&limit=20&folderId="+folderID, nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)

		res = req(t, c, http.MethodDelete, base, "/api/v1/folders/"+folderID, nil, wsHdr(ws))
		require.Equal(t, http.StatusNoContent, res.StatusCode)

		res = req(t, c, http.MethodGet, base, "/api/v1/agents/"+folderAgentID, nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var g struct {
			Data struct {
				FolderID *string `json:"folder_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &g))
		require.Nil(t, g.Data.FolderID)

		res = req(t, c, http.MethodPost, base, "/api/v1/folders",
			[]byte(`{"name":"Template Agents"}`), wsHdr(ws))
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		var e errEnvelope
		require.NoError(t, json.Unmarshal(readBody(t, res), &e))
		require.Equal(t, "template_folder_is_virtual", e.Error.Code)

		// cleanup: unpublish + delete folder agent flows + delete agent
		res = req(t, c, http.MethodPost, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/unpublish", folderAgentID), []byte(`{}`), wsHdr(ws))
		unpubBody := readBody(t, res)
		require.Contains(t, []int{http.StatusNoContent, http.StatusNotFound}, res.StatusCode, string(unpubBody))

		res = req(t, c, http.MethodGet, base, "/api/v1/agents/"+folderAgentID+"/conversation-flows", nil, wsHdr(ws))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var list struct {
			Data []struct {
				ConversationFlowID string `json:"conversation_flow_id"`
				Version            int    `json:"version"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &list))
		for _, row := range list.Data {
			res = req(t, c, http.MethodDelete, base,
				fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=%d", folderAgentID, row.ConversationFlowID, row.Version),
				nil, wsHdr(ws))
			delBody := readBody(t, res)
			require.Contains(t, []int{http.StatusNoContent, http.StatusBadRequest}, res.StatusCode, string(delBody))
		}
		res = req(t, c, http.MethodDelete, base, "/api/v1/agents/"+folderAgentID, nil, wsHdr(ws))
		require.Equal(t, http.StatusNoContent, res.StatusCode)
	})

	t.Run("cleanup_limit_agent", func(t *testing.T) {
		res := req(t, c, http.MethodPost, base,
			fmt.Sprintf("/api/v1/agents/%s/conversation-flows/unpublish", limitAgentID), []byte(`{}`), wsHdr(ws))
		_ = readBody(t, res)

		res = req(t, c, http.MethodGet, base, "/api/v1/agents/"+limitAgentID+"/conversation-flows", nil, wsHdr(ws))
		if res.StatusCode != http.StatusOK {
			return
		}
		var list struct {
			Data []struct {
				ConversationFlowID string `json:"conversation_flow_id"`
				Version            int    `json:"version"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readBody(t, res), &list))
		for _, row := range list.Data {
			res = req(t, c, http.MethodDelete, base,
				fmt.Sprintf("/api/v1/agents/%s/conversation-flows/%s?version=%d", limitAgentID, row.ConversationFlowID, row.Version),
				nil, wsHdr(ws))
			_ = readBody(t, res)
		}
		res = req(t, c, http.MethodDelete, base, "/api/v1/agents/"+limitAgentID, nil, wsHdr(ws))
		_ = readBody(t, res)
	})
}
