package httpapi

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
)

type memTx struct{}

func (memTx) WithTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

func flowStorageKey(ws, agentID, cfID string, version int) string {
	return ws + "\x00" + agentID + "\x00" + cfID + "\x00" + strconv.Itoa(version)
}

func publishedKey(ws, agentID string) string {
	return ws + "\x00" + agentID
}

// testMemoryStore — упрощённое in-memory хранилище для API-тестов (не потокобезопасно как прод, но с Mutex).
type testMemoryStore struct {
	mu sync.Mutex

	foldersByWS map[string]map[string]models.Folder
	agentsByWS  map[string]map[string]models.Agent
	flows       map[string]models.ConversationFlow // flowStorageKey -> ConversationFlow
	published   map[string]models.PublishedWorkflow
}

func newTestMemoryStore() *testMemoryStore {
	return &testMemoryStore{
		foldersByWS: make(map[string]map[string]models.Folder),
		agentsByWS:  make(map[string]map[string]models.Agent),
		flows:       make(map[string]models.ConversationFlow),
		published:   make(map[string]models.PublishedWorkflow),
	}
}

func (s *testMemoryStore) folderRepo() repo.FolderRepository { return (*memFolderRepo)(s) }
func (s *testMemoryStore) agentRepo() repo.AgentRepository   { return (*memAgentRepo)(s) }
func (s *testMemoryStore) flowRepo() repo.ConversationFlowRepository {
	return (*memFlowRepo)(s)
}
func (s *testMemoryStore) publishedRepo() repo.PublishedWorkflowRepository {
	return (*memPublishedRepo)(s)
}

type memFolderRepo testMemoryStore

func (r *memFolderRepo) List(ctx context.Context, workspaceID string) ([]models.Folder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.foldersByWS[workspaceID]
	if len(m) == 0 {
		return nil, nil
	}
	out := make([]models.Folder, 0, len(m))
	for _, f := range m {
		out = append(out, f)
	}
	return out, nil
}

func (r *memFolderRepo) GetByID(ctx context.Context, workspaceID, folderID string) (models.Folder, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.foldersByWS[workspaceID]
	if m == nil {
		return models.Folder{}, false, nil
	}
	f, ok := m[folderID]
	return f, ok, nil
}

func (r *memFolderRepo) Create(ctx context.Context, folder models.Folder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.foldersByWS[folder.WorkspaceID] == nil {
		r.foldersByWS[folder.WorkspaceID] = make(map[string]models.Folder)
	}
	r.foldersByWS[folder.WorkspaceID][folder.FolderID] = folder
	return nil
}

func (r *memFolderRepo) UpdateName(ctx context.Context, workspaceID, folderID, name string, updatedAt int64) (models.Folder, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.foldersByWS[workspaceID]
	if m == nil {
		return models.Folder{}, false, nil
	}
	f, ok := m[folderID]
	if !ok {
		return models.Folder{}, false, nil
	}
	f.Name = name
	f.UpdatedAt = updatedAt
	m[folderID] = f
	return f, true, nil
}

func (r *memFolderRepo) Delete(ctx context.Context, workspaceID, folderID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.foldersByWS[workspaceID]
	if m == nil {
		return false, nil
	}
	if _, ok := m[folderID]; !ok {
		return false, nil
	}
	delete(m, folderID)
	return true, nil
}

func (r *memFolderRepo) Count(ctx context.Context, workspaceID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.foldersByWS[workspaceID])), nil
}

type memAgentRepo testMemoryStore

func (r *memAgentRepo) GetByID(ctx context.Context, workspaceID, agentID string) (models.Agent, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.agentsByWS[workspaceID]
	if m == nil {
		return models.Agent{}, false, nil
	}
	a, ok := m[agentID]
	return a, ok, nil
}

func (r *memAgentRepo) Create(ctx context.Context, agent models.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agentsByWS[agent.WorkspaceID] == nil {
		r.agentsByWS[agent.WorkspaceID] = make(map[string]models.Agent)
	}
	r.agentsByWS[agent.WorkspaceID][agent.AgentID] = agent
	return nil
}

func (r *memAgentRepo) Update(ctx context.Context, workspaceID, agentID string, patch repo.AgentPatch) (models.Agent, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.agentsByWS[workspaceID]
	if m == nil {
		return models.Agent{}, false, nil
	}
	a, ok := m[agentID]
	if !ok {
		return models.Agent{}, false, nil
	}
	if patch.Name != nil {
		a.Name = *patch.Name
	}
	if patch.FolderID != nil {
		a.FolderID = *patch.FolderID
	}
	if patch.Channel != nil {
		a.Channel = *patch.Channel
	}
	if patch.VoiceID != nil {
		a.VoiceID = *patch.VoiceID
	}
	if patch.Language != nil {
		a.Language = *patch.Language
	}
	if patch.TTS != nil {
		a.TTS = *patch.TTS
	}
	if patch.STT != nil {
		a.STT = *patch.STT
	}
	if patch.InterruptionSensitivity != nil {
		a.InterruptionSensitivity = *patch.InterruptionSensitivity
	}
	if patch.MaxCallDurationMS != nil {
		a.MaxCallDurationMS = *patch.MaxCallDurationMS
	}
	if patch.NormalizeForSpeech != nil {
		a.NormalizeForSpeech = *patch.NormalizeForSpeech
	}
	if patch.AllowUserDTMF != nil {
		a.AllowUserDTMF = *patch.AllowUserDTMF
	}
	if patch.UserDTMFOptions != nil {
		a.UserDTMFOptions = *patch.UserDTMFOptions
	}
	if patch.ResponseEngine != nil {
		a.ResponseEngine = *patch.ResponseEngine
	}
	if patch.HandbookConfig != nil {
		a.HandbookConfig = *patch.HandbookConfig
	}
	if patch.PIIConfig != nil {
		a.PIIConfig = *patch.PIIConfig
	}
	if patch.DataStorageSetting != nil {
		a.DataStorageSetting = *patch.DataStorageSetting
	}
	if patch.LastModified != nil {
		a.LastModified = *patch.LastModified
	}
	if patch.UpdatedAt != nil {
		a.UpdatedAt = *patch.UpdatedAt
	}
	m[agentID] = a
	return a, true, nil
}

func (r *memAgentRepo) Delete(ctx context.Context, workspaceID, agentID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.agentsByWS[workspaceID]
	if m == nil {
		return false, nil
	}
	if _, ok := m[agentID]; !ok {
		return false, nil
	}
	delete(m, agentID)
	return true, nil
}

func (r *memAgentRepo) Count(ctx context.Context, workspaceID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.agentsByWS[workspaceID])), nil
}

func (r *memAgentRepo) ListLight(ctx context.Context, workspaceID string, q repo.ListAgentsQuery) ([]repo.LightAgent, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.agentsByWS[workspaceID]
	if len(m) == 0 {
		return nil, 0, nil
	}
	var rows []repo.LightAgent
	for _, a := range m {
		if q.FolderID != nil {
			if a.FolderID == nil || *a.FolderID != *q.FolderID {
				continue
			}
		}
		rows = append(rows, repo.LightAgent{
			AgentID:      a.AgentID,
			Name:         a.Name,
			VoiceID:      a.VoiceID,
			Language:     a.Language,
			FolderID:     a.FolderID,
			LastModified: a.LastModified,
			CreatedAt:    a.CreatedAt,
			UpdatedAt:    a.UpdatedAt,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UpdatedAt > rows[j].UpdatedAt })
	total := int64(len(rows))
	page := q.Page
	if page <= 0 {
		page = 1
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	start := (page - 1) * limit
	if start >= len(rows) {
		return nil, total, nil
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], total, nil
}

func (r *memAgentRepo) DetachFromFolder(ctx context.Context, workspaceID, folderID string, updatedAt int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.agentsByWS[workspaceID]
	for id, a := range m {
		if a.FolderID != nil && *a.FolderID == folderID {
			a.FolderID = nil
			a.UpdatedAt = updatedAt
			a.LastModified = updatedAt
			m[id] = a
		}
	}
	return nil
}

type memFlowRepo testMemoryStore

func (r *memFlowRepo) GetVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (models.ConversationFlow, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := flowStorageKey(workspaceID, agentID, conversationFlowID, version)
	cf, ok := r.flows[key]
	return cf, ok, nil
}

func (r *memFlowRepo) ListVersionsLight(ctx context.Context, workspaceID, agentID string) ([]repo.LightConversationFlowVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []repo.LightConversationFlowVersion
	prefix := workspaceID + "\x00" + agentID + "\x00"
	for key, cf := range r.flows {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			out = append(out, repo.LightConversationFlowVersion{
				ConversationFlowID: cf.ConversationFlowID,
				Version:            cf.Version,
				CreatedAt:          cf.CreatedAt,
				UpdatedAt:          cf.UpdatedAt,
				Published:          false,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConversationFlowID != out[j].ConversationFlowID {
			return out[i].ConversationFlowID < out[j].ConversationFlowID
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

func (r *memFlowRepo) CreateVersion(ctx context.Context, cf models.ConversationFlow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := flowStorageKey(cf.WorkspaceID, cf.AgentID, cf.ConversationFlowID, cf.Version)
	if _, exists := r.flows[key]; exists {
		return fmt.Errorf("duplicate flow version")
	}
	r.flows[key] = cf
	return nil
}

func (r *memFlowRepo) UpdateVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int, patch repo.ConversationFlowPatch) (models.ConversationFlow, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := flowStorageKey(workspaceID, agentID, conversationFlowID, version)
	cf, ok := r.flows[key]
	if !ok {
		return models.ConversationFlow{}, false, nil
	}
	changed := false
	if patch.Nodes != nil {
		cf.Nodes = *patch.Nodes
		changed = true
	}
	if patch.StartNodeID != nil {
		cf.StartNodeID = *patch.StartNodeID
		changed = true
	}
	if patch.StartSpeaker != nil {
		cf.StartSpeaker = *patch.StartSpeaker
		changed = true
	}
	if patch.GlobalPrompt != nil {
		cf.GlobalPrompt = *patch.GlobalPrompt
		changed = true
	}
	if patch.ModelChoice != nil {
		cf.ModelChoice = *patch.ModelChoice
		changed = true
	}
	if patch.ModelTemperature != nil {
		cf.ModelTemperature = *patch.ModelTemperature
		changed = true
	}
	if patch.FlexMode != nil {
		cf.FlexMode = *patch.FlexMode
		changed = true
	}
	if patch.ToolCallStrictMode != nil {
		cf.ToolCallStrictMode = *patch.ToolCallStrictMode
		changed = true
	}
	if patch.KBConfig != nil {
		cf.KBConfig = *patch.KBConfig
		changed = true
	}
	if patch.BeginTagDisplayPosition != nil {
		cf.BeginTagDisplayPos = *patch.BeginTagDisplayPosition
		changed = true
	}
	if patch.IsTransferCF != nil {
		cf.IsTransferCF = *patch.IsTransferCF
		changed = true
	}
	if patch.UpdatedAt != nil {
		cf.UpdatedAt = *patch.UpdatedAt
		changed = true
	}
	if !changed {
		return cf, true, nil
	}
	r.flows[key] = cf
	return cf, true, nil
}

func (r *memFlowRepo) DeleteVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := flowStorageKey(workspaceID, agentID, conversationFlowID, version)
	if _, ok := r.flows[key]; !ok {
		return false, nil
	}
	delete(r.flows, key)
	return true, nil
}

func (r *memFlowRepo) MaxVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string) (int, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxV := -1
	found := false
	prefix := workspaceID + "\x00" + agentID + "\x00" + conversationFlowID + "\x00"
	for key, cf := range r.flows {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			found = true
			if cf.Version > maxV {
				maxV = cf.Version
			}
		}
	}
	if !found {
		return 0, false, nil
	}
	return maxV, true, nil
}

func (r *memFlowRepo) DeleteAllForAgent(ctx context.Context, workspaceID, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefix := workspaceID + "\x00" + agentID + "\x00"
	var keys []string
	for k := range r.flows {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		delete(r.flows, k)
	}
	return nil
}

type memPublishedRepo testMemoryStore

func (r *memPublishedRepo) Get(ctx context.Context, workspaceID, agentID string) (models.PublishedWorkflow, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pw, ok := r.published[publishedKey(workspaceID, agentID)]
	return pw, ok, nil
}

func (r *memPublishedRepo) Upsert(ctx context.Context, pw models.PublishedWorkflow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published[publishedKey(pw.WorkspaceID, pw.AgentID)] = pw
	return nil
}

func (r *memPublishedRepo) Delete(ctx context.Context, workspaceID, agentID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := publishedKey(workspaceID, agentID)
	if _, ok := r.published[k]; !ok {
		return false, nil
	}
	delete(r.published, k)
	return true, nil
}

func (r *memPublishedRepo) ListByWorkspace(ctx context.Context, workspaceID string) ([]models.PublishedWorkflow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []models.PublishedWorkflow
	for _, pw := range r.published {
		if pw.WorkspaceID == workspaceID {
			out = append(out, pw)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt > out[j].PublishedAt })
	return out, nil
}

func (r *memPublishedRepo) ListGroupedByWorkspace(ctx context.Context) ([]repo.PublishedAgentsByWorkspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	byWS := make(map[string][]repo.PublishedAgentSummary)
	for _, pw := range r.published {
		byWS[pw.WorkspaceID] = append(byWS[pw.WorkspaceID], repo.PublishedAgentSummary{
			AgentID:            pw.AgentID,
			ConversationFlowID: pw.ConversationFlowID,
			Version:            pw.Version,
			PublishedAt:        pw.PublishedAt,
		})
	}
	wss := make([]string, 0, len(byWS))
	for ws := range byWS {
		wss = append(wss, ws)
	}
	sort.Strings(wss)
	out := make([]repo.PublishedAgentsByWorkspace, 0, len(wss))
	for _, ws := range wss {
		agents := byWS[ws]
		sort.Slice(agents, func(i, j int) bool { return agents[i].AgentID < agents[j].AgentID })
		out = append(out, repo.PublishedAgentsByWorkspace{
			WorkspaceID: ws,
			Agents:      agents,
		})
	}
	return out, nil
}
