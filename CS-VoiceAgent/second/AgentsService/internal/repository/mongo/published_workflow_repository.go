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

// PublishedWorkflowRepository — MongoDB реализация repo.PublishedWorkflowRepository.
type PublishedWorkflowRepository struct {
	coll *mongo.Collection
}

var _ repo.PublishedWorkflowRepository = (*PublishedWorkflowRepository)(nil)

func NewPublishedWorkflowRepository(db *mongo.Database) *PublishedWorkflowRepository {
	return &PublishedWorkflowRepository{coll: db.Collection(CollectionPublishedWorkflows)}
}

func (r *PublishedWorkflowRepository) Get(ctx context.Context, workspaceID, agentID string) (models.PublishedWorkflow, bool, error) {
	var pw models.PublishedWorkflow
	err := r.coll.FindOne(ctx, bson.M{"workspace_id": workspaceID, "agent_id": agentID}).Decode(&pw)
	if err == nil {
		return pw, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return models.PublishedWorkflow{}, false, nil
	}
	return models.PublishedWorkflow{}, false, fmt.Errorf("mongo published_workflows get: %w", err)
}

func (r *PublishedWorkflowRepository) Upsert(ctx context.Context, pw models.PublishedWorkflow) error {
	// Upsert по уникальному ключу (workspace_id, agent_id).
	_, err := r.coll.UpdateOne(
		ctx,
		bson.M{"workspace_id": pw.WorkspaceID, "agent_id": pw.AgentID},
		bson.M{"$set": pw},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("mongo published_workflows upsert: %w", err)
	}
	return nil
}

func (r *PublishedWorkflowRepository) Delete(ctx context.Context, workspaceID, agentID string) (bool, error) {
	res, err := r.coll.DeleteOne(ctx, bson.M{"workspace_id": workspaceID, "agent_id": agentID})
	if err != nil {
		return false, fmt.Errorf("mongo published_workflows delete: %w", err)
	}
	return res.DeletedCount == 1, nil
}

func (r *PublishedWorkflowRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]models.PublishedWorkflow, error) {
	cur, err := r.coll.Find(
		ctx,
		bson.M{"workspace_id": workspaceID},
		options.Find().SetSort(bson.D{{Key: "published_at", Value: -1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("mongo published_workflows list by workspace: %w", err)
	}
	defer cur.Close(ctx)

	var out []models.PublishedWorkflow
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("mongo published_workflows decode: %w", err)
	}
	return out, nil
}

func (r *PublishedWorkflowRepository) ListGroupedByWorkspace(ctx context.Context) ([]repo.PublishedAgentsByWorkspace, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$workspace_id"},
			{Key: "agents", Value: bson.D{{Key: "$push", Value: bson.D{
				{Key: "agent_id", Value: "$agent_id"},
				{Key: "conversation_flow_id", Value: "$conversation_flow_id"},
				{Key: "version", Value: "$version"},
				{Key: "published_at", Value: "$published_at"},
			}}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "workspace_id", Value: "$_id"},
			{Key: "agents", Value: 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "workspace_id", Value: 1}}}},
	}

	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongo published_workflows group by workspace: %w", err)
	}
	defer cur.Close(ctx)

	var out []repo.PublishedAgentsByWorkspace
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("mongo published_workflows group decode: %w", err)
	}
	return out, nil
}

