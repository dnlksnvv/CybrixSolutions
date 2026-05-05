package usecase

import (
	"context"
	"testing"
	"time"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	"github.com/stretchr/testify/require"
)

type fakeTx struct{}

func (fakeTx) WithTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type fakePublishedRepo struct {
	pw   models.PublishedWorkflow
	has  bool
}

func (f *fakePublishedRepo) Get(_ context.Context, _, _ string) (models.PublishedWorkflow, bool, error) {
	return f.pw, f.has, nil
}
func (f *fakePublishedRepo) Upsert(_ context.Context, pw models.PublishedWorkflow) error {
	f.pw = pw
	f.has = true
	return nil
}
func (f *fakePublishedRepo) Delete(_ context.Context, _, _ string) (bool, error) {
	if !f.has {
		return false, nil
	}
	f.has = false
	return true, nil
}
func (f *fakePublishedRepo) ListByWorkspace(_ context.Context, _ string) ([]models.PublishedWorkflow, error) {
	return nil, nil
}
func (f *fakePublishedRepo) ListGroupedByWorkspace(_ context.Context) ([]repo.PublishedAgentsByWorkspace, error) {
	return nil, nil
}

type fakeAgentsRepo struct {
	a  models.Agent
	ok bool
}

func (f *fakeAgentsRepo) GetByID(_ context.Context, _, _ string) (models.Agent, bool, error) { return f.a, f.ok, nil }
func (f *fakeAgentsRepo) Create(_ context.Context, _ models.Agent) error                      { return nil }
func (f *fakeAgentsRepo) Update(_ context.Context, _, _ string, patch repo.AgentPatch) (models.Agent, bool, error) {
	if !f.ok {
		return models.Agent{}, false, nil
	}
	if patch.ResponseEngine != nil {
		f.a.ResponseEngine = *patch.ResponseEngine
	}
	if patch.UpdatedAt != nil {
		f.a.UpdatedAt = *patch.UpdatedAt
	}
	if patch.LastModified != nil {
		f.a.LastModified = *patch.LastModified
	}
	return f.a, true, nil
}
func (f *fakeAgentsRepo) Delete(_ context.Context, _, _ string) (bool, error)                 { return true, nil }
func (f *fakeAgentsRepo) Count(_ context.Context, _ string) (int64, error)                    { return 0, nil }
func (f *fakeAgentsRepo) ListLight(_ context.Context, _ string, _ repo.ListAgentsQuery) ([]repo.LightAgent, int64, error) {
	return nil, 0, nil
}
func (f *fakeAgentsRepo) DetachFromFolder(_ context.Context, _, _ string, _ int64) error { return nil }

type fakeFoldersRepo struct{}

func (fakeFoldersRepo) List(context.Context, string) ([]models.Folder, error)                        { return nil, nil }
func (fakeFoldersRepo) GetByID(context.Context, string, string) (models.Folder, bool, error)         { return models.Folder{}, false, nil }
func (fakeFoldersRepo) Create(context.Context, models.Folder) error                                  { return nil }
func (fakeFoldersRepo) UpdateName(context.Context, string, string, string, int64) (models.Folder, bool, error) {
	return models.Folder{}, false, nil
}
func (fakeFoldersRepo) Delete(context.Context, string, string) (bool, error) { return false, nil }
func (fakeFoldersRepo) Count(context.Context, string) (int64, error)         { return 0, nil }

type fakeFlowsRepo struct {
	cf models.ConversationFlow
	ok bool
}

func (f *fakeFlowsRepo) GetVersion(_ context.Context, _, _, _ string, _ int) (models.ConversationFlow, bool, error) {
	return f.cf, f.ok, nil
}
func (f *fakeFlowsRepo) ListVersionsLight(context.Context, string, string) ([]repo.LightConversationFlowVersion, error) {
	return nil, nil
}
func (f *fakeFlowsRepo) CreateVersion(context.Context, models.ConversationFlow) error { return nil }
func (f *fakeFlowsRepo) UpdateVersion(context.Context, string, string, string, int, repo.ConversationFlowPatch) (models.ConversationFlow, bool, error) {
	return f.cf, true, nil
}
func (f *fakeFlowsRepo) DeleteVersion(context.Context, string, string, string, int) (bool, error) { return true, nil }
func (f *fakeFlowsRepo) MaxVersion(context.Context, string, string, string) (int, bool, error)    { return 0, true, nil }
func (f *fakeFlowsRepo) DeleteAllForAgent(context.Context, string, string) error                   { return nil }

func TestPublish_UpdatesPublishedAndAgentResponseEngine(t *testing.T) {
	now := time.Unix(10, 0)
	pwRepo := &fakePublishedRepo{}
	agentsRepo := &fakeAgentsRepo{a: models.Agent{AgentID: "agent_1", WorkspaceID: "ws", ResponseEngine: models.ResponseEngine{Type: "conversation-flow"}}, ok: true}
	flowsRepo := &fakeFlowsRepo{cf: models.ConversationFlow{ConversationFlowID: "cf_1", AgentID: "agent_1", WorkspaceID: "ws", Version: 1}, ok: true}

	u := NewConversationFlowUsecase(Deps{
		Folders:           fakeFoldersRepo{},
		Agents:            agentsRepo,
		ConversationFlows: flowsRepo,
		Published:         pwRepo,
		Tx:               fakeTx{},
		Now:              func() time.Time { return now },
	})

	pw, err := u.Publish(context.Background(), "ws", "agent_1", "cf_1", 1)
	require.NoError(t, err)
	require.True(t, pwRepo.has)
	require.Equal(t, "cf_1", pw.ConversationFlowID)
	require.Equal(t, 1, pw.Version)
	require.Equal(t, "conversation-flow", agentsRepo.a.ResponseEngine.Type)
	require.Equal(t, "cf_1", agentsRepo.a.ResponseEngine.ConversationFlowID)
	require.Equal(t, 1, agentsRepo.a.ResponseEngine.Version)
	require.Equal(t, now.UnixMilli(), agentsRepo.a.UpdatedAt)
	require.Equal(t, now.UnixMilli(), agentsRepo.a.LastModified)
}

func TestDeleteVersion_RefusesPublished(t *testing.T) {
	pwRepo := &fakePublishedRepo{pw: models.PublishedWorkflow{WorkspaceID: "ws", AgentID: "agent_1", ConversationFlowID: "cf_1", Version: 2}, has: true}
	u := NewConversationFlowUsecase(Deps{
		Agents:            &fakeAgentsRepo{ok: true},
		ConversationFlows: &fakeFlowsRepo{ok: true},
		Published:         pwRepo,
		Now:              time.Now,
	})
	err := u.DeleteVersion(context.Background(), "ws", "agent_1", "cf_1", 2)
	var apiErr derr.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, derr.ErrorTypeBusiness, apiErr.Type)
	require.Equal(t, "cannot_delete_published_version", apiErr.Code)
}

