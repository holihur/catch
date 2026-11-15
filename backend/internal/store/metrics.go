package store

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MetricSummary is the latest value for one metric name.
type MetricSummary struct {
	Name        string            `bson:"_id" json:"name"`
	Description string            `bson:"description" json:"description"`
	Unit        string            `bson:"unit" json:"unit"`
	Type        string            `bson:"type" json:"type"`
	Latest      float64           `bson:"latest" json:"latest"`
	Service     string            `bson:"service" json:"service"`
	LastSeen    time.Time         `bson:"lastSeen" json:"lastSeen"`
	PointCount  int64             `bson:"pointCount" json:"pointCount"`
	Attributes  map[string]string `bson:"attributes" json:"attributes"`
}

func (s *Store) InsertMetrics(ctx context.Context, points []model.MetricPoint) error {
	if len(points) == 0 {
		return nil
	}
	docs := make([]any, len(points))
	for i := range points {
		docs[i] = points[i]
	}
	_, err := s.Metrics.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	return err
}

func (s *Store) ListMetrics(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to time.Time, service, query string) ([]MetricSummary, error) {
	match := bson.M{
		"tenantId":  tenantID,
		"timestamp": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	if service != "" {
		match["service"] = service
	}
	if q := strings.TrimSpace(query); q != "" {
		match["name"] = primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "timestamp", Value: -1}}}},
		{{Key: "$group", Value: bson.M{
			"_id":         "$name",
			"description": bson.M{"$first": "$description"},
			"unit":        bson.M{"$first": "$unit"},
			"type":        bson.M{"$first": "$type"},
			"latest":      bson.M{"$first": "$value"},
			"service":     bson.M{"$first": "$service"},
			"lastSeen":    bson.M{"$max": "$timestamp"},
			"pointCount":  bson.M{"$sum": 1},
			"attributes":  bson.M{"$first": "$attributes"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "lastSeen", Value: -1}}}},
		{{Key: "$limit", Value: 500}},
	}
	cur, err := s.Metrics.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	out := make([]MetricSummary, 0)
	err = cur.All(ctx, &out)
	return out, err
}

func (s *Store) MetricSeries(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, name, service string, from, to time.Time, limit int64) ([]model.MetricPoint, error) {
	filter := bson.M{
		"tenantId": tenantID, "name": name,
		"timestamp": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		filter["projectId"] = *projectID
	}
	if service != "" {
		filter["service"] = service
	}
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	cur, err := s.Metrics.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: 1}}).
		SetLimit(limit))
	if err != nil {
		return nil, err
	}
	out := make([]model.MetricPoint, 0)
	err = cur.All(ctx, &out)
	return out, err
}

func (s *Store) MetricServices(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to time.Time) ([]string, error) {
	filter := bson.M{
		"tenantId":  tenantID,
		"timestamp": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		filter["projectId"] = *projectID
	}
	values, err := s.Metrics.Distinct(ctx, "service", filter)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if str, ok := v.(string); ok && str != "" {
			out = append(out, str)
		}
	}
	return out, nil
}
