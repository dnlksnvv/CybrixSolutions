package mongo

import (
	"context"
	"fmt"

	"github.com/cybrix-solutions/agents-service/internal/domain/models"
	repo "github.com/cybrix-solutions/agents-service/internal/repository/interfaces"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AgentRepository — MongoDB реализация repo.AgentRepository.
type AgentRepository struct {
	coll *mongo.Collection
}

var _ repo.AgentRepository = (*AgentRepository)(nil)

func NewAgentRepository(db *mongo.Database) *AgentRepository {
	return &AgentRepository{coll: db.Collection(CollectionAgents)}
}

func (r *AgentRepository) GetByID(ctx context.Context, workspaceID, agentID string) (models.Agent, bool, error) {
	var a models.Agent
	err := r.coll.FindOne(ctx, bson.M{"workspace_id": workspaceID, "agent_id": agentID}).Decode(&a)
	if err == nil {
		return a, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return models.Agent{}, false, nil
	}
	return models.Agent{}, false, fmt.Errorf("mongo agents get: %w", err)
}

func (r *AgentRepository) Create(ctx context.Context, agent models.Agent) error {
	_, err := r.coll.InsertOne(ctx, agent)
	if err != nil {
		return fmt.Errorf("mongo agents create: %w", err)
	}
	return nil
}

func (r *AgentRepository) Update(ctx context.Context, workspaceID, agentID string, patch repo.AgentPatch) (models.Agent, bool, error) {
	update := bson.M{}
	set := bson.M{}

	if patch.Name != nil {
		set["name"] = *patch.Name
	}
	if patch.FolderID != nil {
		// **string semantics: nil -> do not set, &nil -> set null, &("id") -> set id
		set["folder_id"] = *patch.FolderID
	}
	if patch.Channel != nil {
		set["channel"] = *patch.Channel
	}
	if patch.VoiceID != nil {
		set["voice_id"] = *patch.VoiceID
	}
	if patch.Language != nil {
		set["language"] = *patch.Language
	}
	if patch.TTS != nil {
		set["tts"] = *patch.TTS
	}
	if patch.STT != nil {
		set["stt"] = *patch.STT
	}
	if patch.InterruptionSensitivity != nil {
		set["interruption_sensitivity"] = *patch.InterruptionSensitivity
	}
	if patch.MaxCallDurationMS != nil {
		set["max_call_duration_ms"] = *patch.MaxCallDurationMS
	}
	if patch.NormalizeForSpeech != nil {
		set["normalize_for_speech"] = *patch.NormalizeForSpeech
	}
	if patch.AllowUserDTMF != nil {
		set["allow_user_dtmf"] = *patch.AllowUserDTMF
	}
	if patch.UserDTMFOptions != nil {
		set["user_dtmf_options"] = *patch.UserDTMFOptions
	}
	if patch.ResponseEngine != nil {
		set["response_engine"] = *patch.ResponseEngine
	}
	if patch.HandbookConfig != nil {
		set["handbook_config"] = *patch.HandbookConfig
	}
	if patch.PIIConfig != nil {
		set["pii_config"] = *patch.PIIConfig
	}
	if patch.DataStorageSetting != nil {
		set["data_storage_setting"] = *patch.DataStorageSetting
	}
	if patch.LastModified != nil {
		set["last_modified"] = *patch.LastModified
	}
	if patch.UpdatedAt != nil {
		set["updated_at"] = *patch.UpdatedAt
	}

	if len(set) > 0 {
		update["$set"] = set
	}
	if len(update) == 0 {
		// Нечего обновлять, но по контракту нужно вернуть полный актуальный объект.
		return r.GetByID(ctx, workspaceID, agentID)
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var out models.Agent
	err := r.coll.FindOneAndUpdate(
		ctx,
		bson.M{"workspace_id": workspaceID, "agent_id": agentID},
		update,
		opts,
	).Decode(&out)
	if err == nil {
		return out, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return models.Agent{}, false, nil
	}
	return models.Agent{}, false, fmt.Errorf("mongo agents update: %w", err)
}

func (r *AgentRepository) Delete(ctx context.Context, workspaceID, agentID string) (bool, error) {
	res, err := r.coll.DeleteOne(ctx, bson.M{"workspace_id": workspaceID, "agent_id": agentID})
	if err != nil {
		return false, fmt.Errorf("mongo agents delete: %w", err)
	}
	return res.DeletedCount == 1, nil
}

func (r *AgentRepository) Count(ctx context.Context, workspaceID string) (int64, error) {
	cnt, err := r.coll.CountDocuments(ctx, bson.M{"workspace_id": workspaceID})
	if err != nil {
		return 0, fmt.Errorf("mongo agents count: %w", err)
	}
	return cnt, nil
}

func (r *AgentRepository) ListLight(ctx context.Context, workspaceID string, q repo.ListAgentsQuery) (items []repo.LightAgent, total int64, err error) {
	filter := bson.M{"workspace_id": workspaceID}
	if q.FolderID != nil {
		filter["folder_id"] = *q.FolderID
	}

	total, err = r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("mongo agents list count: %w", err)
	}

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
	skip := int64((page - 1) * limit)

	proj := bson.M{
		"_id":          0,
		"agent_id":     1,
		"name":         1,
		"voice_id":     1,
		"language":     1,
		"folder_id":    1,
		"last_modified": 1,
		"created_at":   1,
		"updated_at":   1,
	}

	opts := options.Find().
		SetProjection(proj).
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(limit))

	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("mongo agents list: %w", err)
	}
	defer cur.Close(ctx)

	var out []repo.LightAgent
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, fmt.Errorf("mongo agents list decode: %w", err)
	}
	return out, total, nil
}

func (r *AgentRepository) DetachFromFolder(ctx context.Context, workspaceID, folderID string, updatedAt int64) error {
	_, err := r.coll.UpdateMany(
		ctx,
		bson.M{"workspace_id": workspaceID, "folder_id": folderID},
		bson.M{"$set": bson.M{"folder_id": nil, "updated_at": updatedAt, "last_modified": updatedAt}},
	)
	if err != nil {
		return fmt.Errorf("mongo agents detach folder: %w", err)
	}
	return nil
}

