package store

import (
	"context"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Store) ListAlertSilences(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID) ([]model.AlertSilence, error) {
	filter := bson.M{"tenantId": tenantID}
	if projectID != nil {
		filter["$or"] = []bson.M{
			{"projectId": bson.M{"$exists": false}},
			{"projectId": *projectID},
		}
	}
	cursor, err := s.Silences.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "startsAt", Value: -1}}).SetLimit(500))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]model.AlertSilence, 0)
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) AlertSilenceByID(ctx context.Context, tenantID, id primitive.ObjectID) (*model.AlertSilence, error) {
	var silence model.AlertSilence
	if err := s.Silences.FindOne(ctx, bson.M{"_id": id, "tenantId": tenantID}).Decode(&silence); err != nil {
		return nil, err
	}
	return &silence, nil
}

func (s *Store) CreateAlertSilence(ctx context.Context, silence *model.AlertSilence) error {
	res, err := s.Silences.InsertOne(ctx, silence)
	if err != nil {
		return err
	}
	silence.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (s *Store) UpdateAlertSilence(ctx context.Context, tenantID, id primitive.ObjectID, set bson.M) (*model.AlertSilence, error) {
	set["updatedAt"] = time.Now().UTC()
	var silence model.AlertSilence
	err := s.Silences.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": tenantID},
		bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&silence)
	if err != nil {
		return nil, err
	}
	return &silence, nil
}

func (s *Store) DeleteAlertSilence(ctx context.Context, tenantID, id primitive.ObjectID) error {
	_, err := s.Silences.DeleteOne(ctx, bson.M{"_id": id, "tenantId": tenantID})
	return err
}

// IsRuleSilenced reports whether an active maintenance window covers the rule.
func (s *Store) IsRuleSilenced(ctx context.Context, tenantID, projectID, ruleID primitive.ObjectID, now time.Time) (bool, error) {
	filter := bson.M{
		"tenantId": tenantID,
		"startsAt": bson.M{"$lte": now},
		"endsAt":   bson.M{"$gt": now},
		"$and": []bson.M{
			{"$or": []bson.M{
				{"projectId": bson.M{"$exists": false}},
				{"projectId": projectID},
			}},
			{"$or": []bson.M{
				{"ruleIds": bson.M{"$exists": false}},
				{"ruleIds": bson.M{"$size": 0}},
				{"ruleIds": ruleID},
			}},
		},
	}
	n, err := s.Silences.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
