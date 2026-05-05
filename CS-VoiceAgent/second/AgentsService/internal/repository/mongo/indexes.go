package mongo

import (
	"context"
	"fmt"

	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	CollectionFolders           = "folders"
	CollectionAgents            = "agents"
	CollectionConversationFlows = "conversation_flows"
	CollectionPublishedWorkflows = "published_workflows"
)

// EnsureIndexes создаёт индексы, необходимые для корректной работы сервиса.
// Важно: индексы — часть контракта консистентности данных. Например,
// PublishedWorkflow требует уникальности (workspace_id, agent_id).
func EnsureIndexes(ctx context.Context, db *driver.Database) error {
	if db == nil {
		return fmt.Errorf("mongo: db is nil")
	}

	// PublishedWorkflow: unique (workspace_id, agent_id)
	{
		coll := db.Collection(CollectionPublishedWorkflows)
		_, err := coll.Indexes().CreateOne(ctx, driver.IndexModel{
			Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "agent_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("ux_workspace_agent"),
		})
		if err != nil {
			return fmt.Errorf("mongo: create index published_workflows ux_workspace_agent: %w", err)
		}
	}

	// Agents lookups
	{
		coll := db.Collection(CollectionAgents)
		_, err := coll.Indexes().CreateMany(ctx, []driver.IndexModel{
			{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "agent_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("ux_workspace_agent_id")},
			{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "folder_id", Value: 1}}, Options: options.Index().SetName("ix_workspace_folder")},
			{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "updated_at", Value: -1}}, Options: options.Index().SetName("ix_workspace_updated_at")},
		})
		if err != nil {
			return fmt.Errorf("mongo: create index agents: %w", err)
		}
	}

	// Folders lookups
	{
		coll := db.Collection(CollectionFolders)
		_, err := coll.Indexes().CreateMany(ctx, []driver.IndexModel{
			{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "folder_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("ux_workspace_folder_id")},
			{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "updated_at", Value: -1}}, Options: options.Index().SetName("ix_workspace_updated_at")},
		})
		if err != nil {
			return fmt.Errorf("mongo: create index folders: %w", err)
		}
	}

	// ConversationFlow lookups
	{
		coll := db.Collection(CollectionConversationFlows)
		_, err := coll.Indexes().CreateMany(ctx, []driver.IndexModel{
			{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "agent_id", Value: 1}, {Key: "conversation_flow_id", Value: 1}, {Key: "version", Value: 1}}, Options: options.Index().SetUnique(true).SetName("ux_workspace_agent_flow_version")},
			{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "agent_id", Value: 1}}, Options: options.Index().SetName("ix_workspace_agent")},
		})
		if err != nil {
			return fmt.Errorf("mongo: create index conversation_flows: %w", err)
		}
	}

	return nil
}

