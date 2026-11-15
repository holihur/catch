package store

import (
	"context"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UpsertSourceMap inserts or replaces a compressed source map for a bundle.
func (s *Store) UpsertSourceMap(ctx context.Context, sm model.SourceMap) (model.SourceMap, error) {
	now := time.Now().UTC()
	sm.UpdatedAt = now
	filter := bson.M{
		"tenantId": sm.TenantID, "projectId": sm.ProjectID,
		"release": sm.Release, "file": sm.File,
	}
	update := bson.M{
		"$set": bson.M{
			"size": sm.Size, "sourceCount": sm.SourceCount,
			"content": sm.Content, "updatedAt": now,
		},
		"$setOnInsert": bson.M{"createdAt": now},
	}
	var out model.SourceMap
	err := s.SourceMaps.FindOneAndUpdate(ctx, filter, update,
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&out)
	return out, err
}

// SourceMapsForRelease returns metadata for all maps of a release (no content).
func (s *Store) SourceMapsForRelease(ctx context.Context, tenantID, projectID primitive.ObjectID, release string) ([]model.SourceMap, error) {
	cur, err := s.SourceMaps.Find(ctx,
		bson.M{"tenantId": tenantID, "projectId": projectID, "release": release},
		options.Find().
			SetProjection(bson.M{"content": 0}).
			SetSort(bson.D{{Key: "file", Value: 1}}))
	if err != nil {
		return nil, err
	}
	out := make([]model.SourceMap, 0)
	err = cur.All(ctx, &out)
	return out, err
}

// ListSourceMaps returns source map metadata for a project, newest first.
func (s *Store) ListSourceMaps(ctx context.Context, tenantID, projectID primitive.ObjectID, release string) ([]model.SourceMap, error) {
	filter := bson.M{"tenantId": tenantID, "projectId": projectID}
	if release != "" {
		filter["release"] = release
	}
	cur, err := s.SourceMaps.Find(ctx, filter,
		options.Find().
			SetProjection(bson.M{"content": 0}).
			SetSort(bson.D{{Key: "updatedAt", Value: -1}}).
			SetLimit(200))
	if err != nil {
		return nil, err
	}
	out := make([]model.SourceMap, 0)
	err = cur.All(ctx, &out)
	return out, err
}

// SourceMapContent returns a source map document including compressed content.
func (s *Store) SourceMapContent(ctx context.Context, tenantID, projectID, id primitive.ObjectID) (model.SourceMap, error) {
	var sm model.SourceMap
	err := s.SourceMaps.FindOne(ctx, bson.M{
		"_id": id, "tenantId": tenantID, "projectId": projectID,
	}).Decode(&sm)
	return sm, err
}

// DeleteSourceMap removes a map and returns its metadata.
func (s *Store) DeleteSourceMap(ctx context.Context, tenantID, projectID, id primitive.ObjectID) (model.SourceMap, error) {
	var sm model.SourceMap
	err := s.SourceMaps.FindOneAndDelete(ctx, bson.M{
		"_id": id, "tenantId": tenantID, "projectId": projectID,
	}, options.FindOneAndDelete().SetProjection(bson.M{"content": 0})).Decode(&sm)
	return sm, err
}

var _ = mongo.ErrNoDocuments
