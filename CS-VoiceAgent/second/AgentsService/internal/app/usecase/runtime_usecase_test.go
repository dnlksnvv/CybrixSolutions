package usecase

import (
	"context"
	"testing"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	"github.com/stretchr/testify/require"
)

type fakePublishedRepo2 struct {
	pw  models.PublishedWorkflow
	has bool
}

func (f *fakePublishedRepo2) Get(_ context.Context, _, _ string) (models.PublishedWorkflow, bool, error) {
	return f.pw, f.has, nil
}
func (f *fakePublishedRepo2) Upsert(_ context.Context, _ models.PublishedWorkflow) error { return nil }
func (f *fakePublishedRepo2) Delete(_ context.Context, _, _ string) (bool, error)        { return false, nil }
func (f *fakePublishedRepo2) ListByWorkspace(_ context.Context, _ string) ([]models.PublishedWorkflow, error) {
	return nil, nil
}
func (f *fakePublishedRepo2) ListGroupedByWorkspace(_ context.Context) ([]repo.PublishedAgentsByWorkspace, error) {
	return nil, nil
}

type fakeAgentsRepo2 struct {
	a  models.Agent
	ok bool
}

func (f *fakeAgentsRepo2) GetByID(_ context.Context, _, _ string) (models.Agent, bool, error) { return f.a, f.ok, nil }
func (f *fakeAgentsRepo2) Create(context.Context, models.Agent) error                          { return nil }
func (f *fakeAgentsRepo2) Update(context.Context, string, string, repo.AgentPatch) (models.Agent, bool, error) {
	return models.Agent{}, false, nil
}
func (f *fakeAgentsRepo2) Delete(context.Context, string, string) (bool, error)                               { return false, nil }
func (f *fakeAgentsRepo2) Count(context.Context, string) (int64, error)                                       { return 0, nil }
func (f *fakeAgentsRepo2) ListLight(context.Context, string, repo.ListAgentsQuery) ([]repo.LightAgent, int64, error) {
	return nil, 0, nil
}
func (f *fakeAgentsRepo2) DetachFromFolder(context.Context, string, string, int64) error { return nil }

type fakeFlowsRepo2 struct {
	cf  models.ConversationFlow
	ok  bool
}

func (f *fakeFlowsRepo2) GetVersion(_ context.Context, _, _, _ string, _ int) (models.ConversationFlow, bool, error) {
	return f.cf, f.ok, nil
}
func (f *fakeFlowsRepo2) ListVersionsLight(context.Context, string, string) ([]repo.LightConversationFlowVersion, error) {
	return nil, nil
}
func (f *fakeFlowsRepo2) CreateVersion(context.Context, models.ConversationFlow) error { return nil }
func (f *fakeFlowsRepo2) UpdateVersion(context.Context, string, string, string, int, repo.ConversationFlowPatch) (models.ConversationFlow, bool, error) {
	return models.ConversationFlow{}, false, nil
}
func (f *fakeFlowsRepo2) DeleteVersion(context.Context, string, string, string, int) (bool, error) { return false, nil }
func (f *fakeFlowsRepo2) MaxVersion(context.Context, string, string, string) (int, bool, error)    { return 0, false, nil }
func (f *fakeFlowsRepo2) DeleteAllForAgent(context.Context, string, string) error                   { return nil }

func TestRuntimeGetPublishedConfig_HappyPath_OverwritesResponseEngine(t *testing.T) {
	pwRepo := &fakePublishedRepo2{
		pw:  models.PublishedWorkflow{WorkspaceID: "ws", AgentID: "agent_1", ConversationFlowID: "cf_1", Version: 2},
		has: true,
	}
	agentsRepo := &fakeAgentsRepo2{
		a:  models.Agent{AgentID: "agent_1", WorkspaceID: "ws", ResponseEngine: models.ResponseEngine{Type: "conversation-flow", ConversationFlowID: "old", Version: 0}},
		ok: true,
	}
	flowsRepo := &fakeFlowsRepo2{
		cf: models.ConversationFlow{ConversationFlowID: "cf_1", AgentID: "agent_1", WorkspaceID: "ws", Version: 2},
		ok: true,
	}

	u := NewRuntimeUsecase(Deps{
		Agents:            agentsRepo,
		ConversationFlows: flowsRepo,
		Published:         pwRepo,
	})

	cfg, err := u.GetPublishedConfig(context.Background(), "ws", "agent_1")
	require.NoError(t, err)
	require.Equal(t, "cf_1", cfg.Agent.ResponseEngine.ConversationFlowID)
	require.Equal(t, 2, cfg.Agent.ResponseEngine.Version)
	require.Equal(t, "cf_1", cfg.ConversationFlow.ConversationFlowID)
	require.Equal(t, 2, cfg.ConversationFlow.Version)
}

func TestRuntimeGetPublishedConfig_NoPublishedWorkflow(t *testing.T) {
	u := NewRuntimeUsecase(Deps{
		Agents:            &fakeAgentsRepo2{a: models.Agent{AgentID: "agent_1", WorkspaceID: "ws"}, ok: true},
		ConversationFlows: &fakeFlowsRepo2{ok: true},
		Published:         &fakePublishedRepo2{has: false},
	})
	_, err := u.GetPublishedConfig(context.Background(), "ws", "agent_1")
	var apiErr derr.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, derr.ErrorTypeNotFound, apiErr.Type)
	require.Equal(t, "published_workflow_not_found", apiErr.Code)
}

