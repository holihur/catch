package store

import (
	"context"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/secure"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Store) CreateProject(ctx context.Context, tenantID primitive.ObjectID, name, slug, platform string) (model.Project, error) {
	now := time.Now().UTC()
	p := model.Project{
		TenantID: tenantID, Name: name, Slug: slug, Platform: platform,
		IngestKey: "ck_" + secure.RandomToken(24), CreatedAt: now, UpdatedAt: now,
	}
	res, err := s.Projects.InsertOne(ctx, p)
	if err != nil {
		return p, err
	}
	p.ID = res.InsertedID.(primitive.ObjectID)
	return p, nil
}

func (s *Store) ListProjects(ctx context.Context, tenantID primitive.ObjectID) ([]model.Project, error) {
	cur, err := s.Projects.Find(ctx, bson.M{"tenantId": tenantID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	out := make([]model.Project, 0)
	err = cur.All(ctx, &out)
	return out, err
}

func (s *Store) ProjectByID(ctx context.Context, tenantID, id primitive.ObjectID) (model.Project, error) {
	var p model.Project
	err := s.Projects.FindOne(ctx, bson.M{"_id": id, "tenantId": tenantID}).Decode(&p)
	return p, err
}

func (s *Store) ProjectByKey(ctx context.Context, key string) (model.Project, error) {
	var p model.Project
	err := s.Projects.FindOne(ctx, bson.M{"ingestKey": key}).Decode(&p)
	return p, err
}

func (s *Store) UpdateProject(ctx context.Context, tenantID, id primitive.ObjectID, set bson.M) (model.Project, error) {
	var p model.Project
	set["updatedAt"] = time.Now().UTC()
	err := s.Projects.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": tenantID},
		bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&p)
	return p, err
}

func (s *Store) RotateProjectKey(ctx context.Context, tenantID, id primitive.ObjectID) (model.Project, error) {
	return s.UpdateProject(ctx, tenantID, id, bson.M{"ingestKey": "ck_" + secure.RandomToken(24)})
}

func (s *Store) DeleteProject(ctx context.Context, tenantID, id primitive.ObjectID) error {
	res, err := s.Projects.DeleteOne(ctx, bson.M{"_id": id, "tenantId": tenantID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	// Remove the project's telemetry and issues. Comments are keyed by issue id,
	// so collect issue ids first.
	cur, err := s.Issues.Find(ctx, bson.M{"tenantId": tenantID, "projectId": id}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err == nil {
		var rows []struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := cur.All(ctx, &rows); err == nil && len(rows) > 0 {
			ids := make([]primitive.ObjectID, len(rows))
			for i := range rows {
				ids[i] = rows[i].ID
			}
			_, _ = s.Comments.DeleteMany(ctx, bson.M{"tenantId": tenantID, "issueId": bson.M{"$in": ids}})
		}
	}
	_, _ = s.Issues.DeleteMany(ctx, bson.M{"tenantId": tenantID, "projectId": id})
	_, _ = s.Events.DeleteMany(ctx, bson.M{"tenantId": tenantID, "projectId": id})
	return nil
}

func (s *Store) CountProjects(ctx context.Context, tenantID primitive.ObjectID) (int64, error) {
	return s.Projects.CountDocuments(ctx, bson.M{"tenantId": tenantID})
}
