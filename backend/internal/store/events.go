package store

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EventFilter describes a log/error query.
type EventFilter struct {
	TenantID  primitive.ObjectID
	ProjectID *primitive.ObjectID
	Kind      string
	Source    string
	Level     string
	Query     string
	IssueID   *primitive.ObjectID
	From      *time.Time
	To        *time.Time
	Page      int
	PageSize  int
}

func (f EventFilter) match() bson.M {
	m := bson.M{"tenantId": f.TenantID}
	if f.ProjectID != nil {
		m["projectId"] = *f.ProjectID
	}
	if f.Kind != "" {
		m["kind"] = f.Kind
	}
	if f.Source != "" {
		m["source"] = f.Source
	}
	if f.Level != "" {
		m["level"] = f.Level
	}
	if f.IssueID != nil {
		m["issueId"] = *f.IssueID
	}
	if f.From != nil || f.To != nil {
		r := bson.M{}
		if f.From != nil {
			r["$gte"] = *f.From
		}
		if f.To != nil {
			r["$lte"] = *f.To
		}
		m["timestamp"] = r
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		m["message"] = primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
	}
	return m
}

// InsertEvents writes a batch of events.
func (s *Store) InsertEvents(ctx context.Context, events []model.Event) error {
	if len(events) == 0 {
		return nil
	}
	docs := make([]any, len(events))
	for i := range events {
		docs[i] = events[i]
	}
	_, err := s.Events.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	return err
}

func (s *Store) InsertEvent(ctx context.Context, e model.Event) error {
	_, err := s.Events.InsertOne(ctx, e)
	return err
}

func (s *Store) ListEvents(ctx context.Context, f EventFilter) ([]model.Event, int64, error) {
	m := f.match()
	page := f.Page
	if page < 1 {
		page = 1
	}
	size := f.PageSize
	if size < 1 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	total, err := s.Events.CountDocuments(ctx, m)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64((page - 1) * size)).
		SetLimit(int64(size))
	cur, err := s.Events.Find(ctx, m, opts)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Event, 0, size)
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) EventByID(ctx context.Context, tenantID, id primitive.ObjectID) (model.Event, error) {
	var e model.Event
	err := s.Events.FindOne(ctx, bson.M{"_id": id, "tenantId": tenantID}).Decode(&e)
	return e, err
}

// CountEvents returns events matching an arbitrary Mongo filter.
func (s *Store) CountEvents(ctx context.Context, filter bson.M) (int64, error) {
	return s.Events.CountDocuments(ctx, filter)
}

// Bucket is one point in a time series.
type Bucket struct {
	T     string `bson:"_id" json:"t"`
	Count int64  `bson:"count" json:"count"`
}

// Timeseries groups events by hour when the window is <= 48h, otherwise by day.
func (s *Store) Timeseries(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, kind string, from, to time.Time) ([]Bucket, error) {
	format := "%Y-%m-%d"
	if to.Sub(from) <= 48*time.Hour {
		format = "%Y-%m-%dT%H:00:00Z"
	}
	match := bson.M{
		"tenantId":  tenantID,
		"kind":      kind,
		"timestamp": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$dateToString": bson.M{"format": format, "date": "$timestamp", "timezone": "UTC"}},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cur, err := s.Events.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var out []Bucket
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LevelCounts aggregates counts by level.
func (s *Store) LevelCounts(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, kind string, from, to time.Time) (map[string]int64, error) {
	match := bson.M{
		"tenantId":  tenantID,
		"kind":      kind,
		"timestamp": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{"_id": "$level", "count": bson.M{"$sum": 1}}}},
	}
	cur, err := s.Events.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	type row struct {
		ID    string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	var rows []row
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Count
	}
	return out, nil
}

// DistinctActors counts unique end-user ids across errors in a window.
func (s *Store) DistinctActors(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to time.Time) (int, error) {
	match := bson.M{
		"tenantId":  tenantID,
		"kind":      model.KindError,
		"timestamp": bson.M{"$gte": from, "$lte": to},
		"user.id":   bson.M{"$nin": bson.A{"", nil}},
	}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{"_id": "$user.id"}}},
		{{Key: "$count", Value: "n"}},
	}
	cur, err := s.Events.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	var rows []struct {
		N int `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].N, nil
}

// EventCountsByIssue returns occurrence counts per day for one issue.
func (s *Store) EventCountsByIssue(ctx context.Context, tenantID, issueID primitive.ObjectID, from, to time.Time) ([]Bucket, error) {
	format := "%Y-%m-%d"
	if to.Sub(from) <= 48*time.Hour {
		format = "%Y-%m-%dT%H:00:00Z"
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"tenantId": tenantID, "issueId": issueID,
			"timestamp": bson.M{"$gte": from, "$lte": to},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$dateToString": bson.M{"format": format, "date": "$timestamp", "timezone": "UTC"}},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cur, err := s.Events.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var out []Bucket
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PurgeTenantEvents removes all events for a tenant (used by retention tests).
func (s *Store) PurgeTenantEvents(ctx context.Context, tenantID primitive.ObjectID) error {
	_, err := s.Events.DeleteMany(ctx, bson.M{"tenantId": tenantID})
	return err
}

// Explain is a small helper used by integration smoke tests.
func (s *Store) Explain() string { return fmt.Sprintf("db=%s", s.DB.Name()) }
