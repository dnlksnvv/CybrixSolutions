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

// ConversationFlowRepository — MongoDB реализация repo.ConversationFlowRepository.
type ConversationFlowRepository struct {
	coll *mongo.Collection
}

var _ repo.ConversationFlowRepository = (*ConversationFlowRepository)(nil)

func NewConversationFlowRepository(db *mongo.Database) *ConversationFlowRepository {
	return &ConversationFlowRepository{coll: db.Collection(CollectionConversationFlows)}
}

func (r *ConversationFlowRepository) GetVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (models.ConversationFlow, bool, error) {
	var cf models.ConversationFlow
	err := r.coll.FindOne(ctx, bson.M{
		"workspace_id":         workspaceID,
		"agent_id":             agentID,
		"conversation_flow_id": conversationFlowID,
		"version":              version,
	}).Decode(&cf)
	if err == nil {
		return cf, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return models.ConversationFlow{}, false, nil
	}
	return models.ConversationFlow{}, false, fmt.Errorf("mongo conversation_flows get: %w", err)
}

func (r *ConversationFlowRepository) ListVersionsLight(ctx context.Context, workspaceID, agentID string) ([]repo.LightConversationFlowVersion, error) {
	proj := bson.M{
		"_id":                 0,
		"conversation_flow_id": 1,
		"version":              1,
		"created_at":           1,
		"updated_at":           1,
	}
	opts := options.Find().
		SetProjection(proj).
		SetSort(bson.D{{Key: "version", Value: 1}})

	cur, err := r.coll.Find(ctx, bson.M{"workspace_id": workspaceID, "agent_id": agentID}, opts)
	if err != nil {
		return nil, fmt.Errorf("mongo conversation_flows list light: %w", err)
	}
	defer cur.Close(ctx)

	type row struct {
		ConversationFlowID string `bson:"conversation_flow_id"`
		Version            int    `bson:"version"`
		CreatedAt          int64  `bson:"created_at"`
		UpdatedAt          int64  `bson:"updated_at"`
	}
	var rows []row
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("mongo conversation_flows list decode: %w", err)
	}
	out := make([]repo.LightConversationFlowVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, repo.LightConversationFlowVersion{
			ConversationFlowID: r.ConversationFlowID,
			Version:            r.Version,
			CreatedAt:          r.CreatedAt,
			UpdatedAt:          r.UpdatedAt,
			Published:          false, // вычисляется usecase-слоем через PublishedWorkflow
		})
	}
	return out, nil
}

func (r *ConversationFlowRepository) CreateVersion(ctx context.Context, cf models.ConversationFlow) error {
	_, err := r.coll.InsertOne(ctx, cf)
	if err != nil {
		return fmt.Errorf("mongo conversation_flows create: %w", err)
	}
	return nil
}

func (r *ConversationFlowRepository) UpdateVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int, patch repo.ConversationFlowPatch) (models.ConversationFlow, bool, error) {
	set := bson.M{}
	if patch.Nodes != nil {
		set["nodes"] = *patch.Nodes
	}
	if patch.StartNodeID != nil {
		set["start_node_id"] = *patch.StartNodeID
	}
	if patch.StartSpeaker != nil {
		set["start_speaker"] = *patch.StartSpeaker
	}
	if patch.GlobalPrompt != nil {
		set["global_prompt"] = *patch.GlobalPrompt
	}
	if patch.ModelChoice != nil {
		set["model_choice"] = *patch.ModelChoice
	}
	if patch.ModelTemperature != nil {
		set["model_temperature"] = *patch.ModelTemperature
	}
	if patch.FlexMode != nil {
		set["flex_mode"] = *patch.FlexMode
	}
	if patch.ToolCallStrictMode != nil {
		set["tool_call_strict_mode"] = *patch.ToolCallStrictMode
	}
	if patch.KBConfig != nil {
		set["kb_config"] = *patch.KBConfig
	}
	if patch.BeginTagDisplayPosition != nil {
		set["begin_tag_display_position"] = *patch.BeginTagDisplayPosition
	}
	if patch.IsTransferCF != nil {
		set["is_transfer_cf"] = *patch.IsTransferCF
	}
	if patch.UpdatedAt != nil {
		set["updated_at"] = *patch.UpdatedAt
	}

	if len(set) == 0 {
		return r.GetVersion(ctx, workspaceID, agentID, conversationFlowID, version)
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var out models.ConversationFlow
	err := r.coll.FindOneAndUpdate(
		ctx,
		bson.M{
			"workspace_id":         workspaceID,
			"agent_id":             agentID,
			"conversation_flow_id": conversationFlowID,
			"version":              version,
		},
		bson.M{"$set": set},
		opts,
	).Decode(&out)
	if err == nil {
		return out, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return models.ConversationFlow{}, false, nil
	}
	return models.ConversationFlow{}, false, fmt.Errorf("mongo conversation_flows update: %w", err)
}

func (r *ConversationFlowRepository) DeleteVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string, version int) (bool, error) {
	res, err := r.coll.DeleteOne(ctx, bson.M{
		"workspace_id":         workspaceID,
		"agent_id":             agentID,
		"conversation_flow_id": conversationFlowID,
		"version":              version,
	})
	if err != nil {
		return false, fmt.Errorf("mongo conversation_flows delete: %w", err)
	}
	return res.DeletedCount == 1, nil
}

func (r *ConversationFlowRepository) MaxVersion(ctx context.Context, workspaceID, agentID, conversationFlowID string) (int, bool, error) {
	proj := bson.M{"_id": 0, "version": 1}
	opts := options.FindOne().SetProjection(proj).SetSort(bson.D{{Key: "version", Value: -1}})
	var row struct {
		Version int `bson:"version"`
	}
	err := r.coll.FindOne(ctx, bson.M{
		"workspace_id":         workspaceID,
		"agent_id":             agentID,
		"conversation_flow_id": conversationFlowID,
	}, opts).Decode(&row)
	if err == nil {
		return row.Version, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("mongo conversation_flows max version: %w", err)
}

func (r *ConversationFlowRepository) DeleteAllForAgent(ctx context.Context, workspaceID, agentID string) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{"workspace_id": workspaceID, "agent_id": agentID})
	if err != nil {
		return fmt.Errorf("mongo conversation_flows delete all: %w", err)
	}
	return nil
}

