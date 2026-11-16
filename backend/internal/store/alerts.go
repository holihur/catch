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

func projectScope(tenantID, projectID primitive.ObjectID) bson.M {
	filter := bson.M{"tenantId": tenantID}
	if projectID != primitive.NilObjectID {
		filter["projectId"] = projectID
	}
	return filter
}

// ListAlertRules returns rules in a tenant, optionally scoped to a project.
func (s *Store) ListAlertRules(ctx context.Context, tenantID, projectID primitive.ObjectID) ([]model.AlertRule, error) {
	cursor, err := s.AlertRules.Find(ctx, projectScope(tenantID, projectID),
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(500))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]model.AlertRule, 0)
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) CreateAlertRule(ctx context.Context, rule *model.AlertRule) error {
	res, err := s.AlertRules.InsertOne(ctx, rule)
	if err != nil {
		return err
	}
	rule.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (s *Store) UpdateAlertRule(ctx context.Context, tenantID primitive.ObjectID, id primitive.ObjectID, set bson.M) (*model.AlertRule, error) {
	set["updatedAt"] = time.Now().UTC()
	var rule model.AlertRule
	err := s.AlertRules.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": tenantID},
		bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&rule)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *Store) DeleteAlertRule(ctx context.Context, tenantID, id primitive.ObjectID) error {
	_, err := s.AlertRules.DeleteOne(ctx, bson.M{"_id": id, "tenantId": tenantID})
	return err
}

func (s *Store) EnabledAlertRules(ctx context.Context) ([]model.AlertRule, error) {
	cursor, err := s.AlertRules.Find(ctx, bson.M{"enabled": true})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]model.AlertRule, 0)
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SetAlertRuleResult(ctx context.Context, id primitive.ObjectID, state string, value float64, hasData bool, firedAt *time.Time, now time.Time) error {
	set := bson.M{
		"state":         state,
		"value":         value,
		"hasData":       hasData,
		"lastEvaluated": now,
		"updatedAt":     now,
	}
	if firedAt != nil {
		set["firedAt"] = *firedAt
	}
	_, err := s.AlertRules.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// MetricAggregate evaluates a metric over a time window. It returns
// hasData=false when no points exist in the window.
func (s *Store) MetricAggregate(ctx context.Context, tenantID, projectID primitive.ObjectID, name, service string, from time.Time, aggregation string) (float64, bool, error) {
	match := projectScope(tenantID, projectID)
	match["name"] = name
	match["timestamp"] = bson.M{"$gte": from}
	if service != "" {
		match["service"] = service
	}

	group := bson.M{"_id": nil, "value": bson.M{"$avg": "$value"}}
	switch aggregation {
	case "max":
		group["value"] = bson.M{"$max": "$value"}
	case "min":
		group["value"] = bson.M{"$min": "$value"}
	case "sum":
		group["value"] = bson.M{"$sum": "$value"}
	case "last":
		// Sort newest first, then take the first value.
		group["value"] = bson.M{"$first": "$value"}
	}

	pipeline := mongo.Pipeline{{{Key: "$match", Value: match}}}
	if aggregation == "last" {
		pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "timestamp", Value: -1}}}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$group", Value: group}}, bson.D{{Key: "$limit", Value: 1}})

	cursor, err := s.Metrics.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, false, err
	}
	defer cursor.Close(ctx)
	if !cursor.Next(ctx) {
		return 0, false, cursor.Err()
	}
	var result struct {
		Value float64 `bson:"value"`
	}
	if err := cursor.Decode(&result); err != nil {
		return 0, false, err
	}
	return result.Value, true, nil
}

func (s *Store) FiringAlert(ctx context.Context, ruleID primitive.ObjectID) (*model.Alert, error) {
	var alert model.Alert
	err := s.Alerts.FindOne(ctx, bson.M{"ruleId": ruleID, "state": "firing"},
		options.FindOne().SetSort(bson.D{{Key: "startedAt", Value: -1}})).Decode(&alert)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

func (s *Store) CreateAlert(ctx context.Context, alert *model.Alert) error {
	res, err := s.Alerts.InsertOne(ctx, alert)
	if err != nil {
		return err
	}
	alert.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (s *Store) TouchFiringAlert(ctx context.Context, ruleID primitive.ObjectID, value float64, message string, now time.Time) error {
	_, err := s.Alerts.UpdateMany(ctx, bson.M{"ruleId": ruleID, "state": "firing"}, bson.M{"$set": bson.M{
		"value": value, "message": message, "updatedAt": now,
	}})
	return err
}

func (s *Store) ResolveAlerts(ctx context.Context, ruleID primitive.ObjectID, now time.Time) error {
	_, err := s.Alerts.UpdateMany(ctx, bson.M{"ruleId": ruleID, "state": "firing"}, bson.M{"$set": bson.M{
		"state": "resolved", "resolvedAt": now, "updatedAt": now,
	}})
	return err
}

func (s *Store) CountFiringAlerts(ctx context.Context, tenantID, projectID primitive.ObjectID) (int64, error) {
	filter := projectScope(tenantID, projectID)
	filter["state"] = "firing"
	return s.Alerts.CountDocuments(ctx, filter)
}

func (s *Store) ListAlerts(ctx context.Context, tenantID, projectID primitive.ObjectID, state string, limit int64) ([]model.Alert, error) {
	filter := projectScope(tenantID, projectID)
	if state != "" && state != "all" {
		filter["state"] = state
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cursor, err := s.Alerts.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "startedAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]model.Alert, 0)
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
