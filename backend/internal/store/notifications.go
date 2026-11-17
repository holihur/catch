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

func (s *Store) ListNotificationChannels(ctx context.Context, tenantID primitive.ObjectID) ([]model.NotificationChannel, error) {
	cursor, err := s.Channels.Find(ctx, bson.M{"tenantId": tenantID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(200))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]model.NotificationChannel, 0)
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].WebhookSecretSet = out[i].WebhookSecretEnc != ""
	}
	return out, nil
}

func (s *Store) NotificationChannelByID(ctx context.Context, tenantID, id primitive.ObjectID) (*model.NotificationChannel, error) {
	var channel model.NotificationChannel
	err := s.Channels.FindOne(ctx, bson.M{"_id": id, "tenantId": tenantID}).Decode(&channel)
	if err != nil {
		return nil, err
	}
	channel.WebhookSecretSet = channel.WebhookSecretEnc != ""
	return &channel, nil
}

func (s *Store) CreateNotificationChannel(ctx context.Context, channel *model.NotificationChannel) error {
	res, err := s.Channels.InsertOne(ctx, channel)
	if err != nil {
		return err
	}
	channel.ID = res.InsertedID.(primitive.ObjectID)
	channel.WebhookSecretSet = channel.WebhookSecretEnc != ""
	return nil
}

func (s *Store) UpdateNotificationChannel(ctx context.Context, tenantID, id primitive.ObjectID, set bson.M) (*model.NotificationChannel, error) {
	set["updatedAt"] = time.Now().UTC()
	var channel model.NotificationChannel
	err := s.Channels.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": tenantID},
		bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&channel)
	if err != nil {
		return nil, err
	}
	channel.WebhookSecretSet = channel.WebhookSecretEnc != ""
	return &channel, nil
}

func (s *Store) DeleteNotificationChannel(ctx context.Context, tenantID, id primitive.ObjectID) error {
	if _, err := s.Channels.DeleteOne(ctx, bson.M{"_id": id, "tenantId": tenantID}); err != nil {
		return err
	}
	_, err := s.AlertRules.UpdateMany(ctx, bson.M{"tenantId": tenantID}, bson.M{"$pull": bson.M{"channelIds": id}})
	return err
}

func (s *Store) EnqueueNotificationDelivery(ctx context.Context, delivery *model.NotificationDelivery) error {
	res, err := s.Deliveries.InsertOne(ctx, delivery)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil
		}
		return err
	}
	delivery.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// ClaimNotificationDeliveries atomically leases due deliveries for one worker.
// A crashed worker's processing lease is recovered after five minutes.
func (s *Store) ClaimNotificationDeliveries(ctx context.Context, limit int64) ([]model.NotificationDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	now := time.Now().UTC()
	_, _ = s.Deliveries.UpdateMany(ctx,
		bson.M{"status": "processing", "nextAttemptAt": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"status": "pending", "updatedAt": now}})

	out := make([]model.NotificationDelivery, 0, limit)
	for int64(len(out)) < limit {
		var delivery model.NotificationDelivery
		err := s.Deliveries.FindOneAndUpdate(ctx,
			bson.M{"status": "pending", "nextAttemptAt": bson.M{"$lte": now}},
			bson.M{
				"$set": bson.M{"status": "processing", "nextAttemptAt": now.Add(5 * time.Minute), "updatedAt": now},
				"$inc": bson.M{"attempts": 1},
			},
			options.FindOneAndUpdate().
				SetSort(bson.D{{Key: "nextAttemptAt", Value: 1}}).
				SetReturnDocument(options.After),
		).Decode(&delivery)
		if err != nil {
			if mongo.ErrNoDocuments == err {
				break
			}
			return out, err
		}
		out = append(out, delivery)
	}
	return out, nil
}

func (s *Store) UpdateNotificationDelivery(ctx context.Context, id primitive.ObjectID, set bson.M) error {
	set["updatedAt"] = time.Now().UTC()
	_, err := s.Deliveries.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

func (s *Store) ListNotificationDeliveries(ctx context.Context, tenantID, projectID primitive.ObjectID, alertID primitive.ObjectID, status string, limit int64) ([]model.NotificationDelivery, error) {
	filter := projectScope(tenantID, projectID)
	if alertID != primitive.NilObjectID {
		filter["alertId"] = alertID
	}
	if status != "" && status != "all" {
		filter["status"] = status
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cursor, err := s.Deliveries.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]model.NotificationDelivery, 0)
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
