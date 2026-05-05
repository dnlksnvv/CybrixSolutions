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

// FolderRepository — MongoDB реализация repo.FolderRepository.
type FolderRepository struct {
	coll *mongo.Collection
}

var _ repo.FolderRepository = (*FolderRepository)(nil)

func NewFolderRepository(db *mongo.Database) *FolderRepository {
	return &FolderRepository{coll: db.Collection(CollectionFolders)}
}

func (r *FolderRepository) List(ctx context.Context, workspaceID string) ([]models.Folder, error) {
	cur, err := r.coll.Find(ctx, bson.M{"workspace_id": workspaceID}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("mongo folders list: %w", err)
	}
	defer cur.Close(ctx)

	var out []models.Folder
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("mongo folders decode: %w", err)
	}
	return out, nil
}

func (r *FolderRepository) GetByID(ctx context.Context, workspaceID, folderID string) (models.Folder, bool, error) {
	var f models.Folder
	err := r.coll.FindOne(ctx, bson.M{"workspace_id": workspaceID, "folder_id": folderID}).Decode(&f)
	if err == nil {
		return f, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return models.Folder{}, false, nil
	}
	return models.Folder{}, false, fmt.Errorf("mongo folders get: %w", err)
}

func (r *FolderRepository) Create(ctx context.Context, folder models.Folder) error {
	_, err := r.coll.InsertOne(ctx, folder)
	if err != nil {
		return fmt.Errorf("mongo folders create: %w", err)
	}
	return nil
}

func (r *FolderRepository) UpdateName(ctx context.Context, workspaceID, folderID, name string, updatedAt int64) (models.Folder, bool, error) {
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var out models.Folder
	err := r.coll.FindOneAndUpdate(
		ctx,
		bson.M{"workspace_id": workspaceID, "folder_id": folderID},
		bson.M{"$set": bson.M{"name": name, "updated_at": updatedAt}},
		opts,
	).Decode(&out)
	if err == nil {
		return out, true, nil
	}
	if err == mongo.ErrNoDocuments {
		return models.Folder{}, false, nil
	}
	return models.Folder{}, false, fmt.Errorf("mongo folders update: %w", err)
}

func (r *FolderRepository) Delete(ctx context.Context, workspaceID, folderID string) (bool, error) {
	res, err := r.coll.DeleteOne(ctx, bson.M{"workspace_id": workspaceID, "folder_id": folderID})
	if err != nil {
		return false, fmt.Errorf("mongo folders delete: %w", err)
	}
	return res.DeletedCount == 1, nil
}

func (r *FolderRepository) Count(ctx context.Context, workspaceID string) (int64, error) {
	// Отдельный helper для лимитов (например, максимум 50 папок на workspace).
	cnt, err := r.coll.CountDocuments(ctx, bson.M{"workspace_id": workspaceID})
	if err != nil {
		return 0, fmt.Errorf("mongo folders count: %w", err)
	}
	return cnt, nil
}

