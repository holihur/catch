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

// TraceFilter describes a trace list query.
type TraceFilter struct {
	TenantID  primitive.ObjectID
	ProjectID *primitive.ObjectID
	From      *time.Time
	To        *time.Time
	Service   string
	Status    string // all|error|ok
	Query     string
	Page      int
	PageSize  int
}

// TraceSummary is an aggregated trace row for the trace list.
type TraceSummary struct {
	TraceID    string    `bson:"_id" json:"traceId"`
	RootName   string    `bson:"rootName" json:"rootName"`
	Service    string    `bson:"service" json:"service"`
	StartTime  time.Time `bson:"startTime" json:"startTime"`
	EndTime    time.Time `bson:"endTime" json:"endTime"`
	DurationMS int64     `bson:"durationMs" json:"durationMs"`
	SpanCount  int64     `bson:"spanCount" json:"spanCount"`
	ErrorCount int64     `bson:"errorCount" json:"errorCount"`
	Services   []string  `bson:"services" json:"services"`
}

func (s *Store) InsertSpans(ctx context.Context, spans []model.Span) error {
	if len(spans) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, 0, len(spans))
	for i := range spans {
		span := spans[i]
		raw, err := bson.Marshal(span)
		if err != nil {
			return err
		}
		var doc bson.M
		if err := bson.Unmarshal(raw, &doc); err != nil {
			return err
		}
		delete(doc, "_id")
		filter := bson.M{
			"tenantId": span.TenantID, "projectId": span.ProjectID,
			"traceId": span.TraceID, "spanId": span.SpanID,
		}
		models = append(models, mongo.NewReplaceOneModel().
			SetFilter(filter).
			SetReplacement(doc).
			SetUpsert(true))
	}
	_, err := s.Spans.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	return err
}

func (s *Store) ListTraces(ctx context.Context, f TraceFilter) ([]TraceSummary, int64, error) {
	match := bson.M{"tenantId": f.TenantID}
	if f.ProjectID != nil {
		match["projectId"] = *f.ProjectID
	}
	if f.From != nil || f.To != nil {
		r := bson.M{}
		if f.From != nil {
			r["$gte"] = *f.From
		}
		if f.To != nil {
			r["$lte"] = *f.To
		}
		match["startTime"] = r
	}
	if f.Service != "" {
		match["service"] = f.Service
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		re := primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
		match["$or"] = bson.A{
			bson.M{"traceId": re},
			bson.M{"name": re},
			bson.M{"service": re},
		}
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	size := f.PageSize
	if size < 1 {
		size = 25
	}
	if size > 100 {
		size = 100
	}
	groupStage := bson.M{
		"_id":        "$traceId",
		"startTime":  bson.M{"$min": "$startTime"},
		"endTime":    bson.M{"$max": "$endTime"},
		"spanCount":  bson.M{"$sum": 1},
		"errorCount": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$status", "error"}}, 1, 0}}},
		"rootName":   bson.M{"$first": "$name"},
		"service":    bson.M{"$first": "$service"},
		"services":   bson.M{"$addToSet": "$service"},
	}
	afterGroup := bson.M{}
	switch f.Status {
	case "error":
		afterGroup["errorCount"] = bson.M{"$gt": 0}
	case "ok":
		afterGroup["errorCount"] = 0
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "startTime", Value: 1}}}},
		{{Key: "$group", Value: groupStage}},
	}
	if len(afterGroup) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: afterGroup}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$sort", Value: bson.D{{Key: "startTime", Value: -1}}}},
		bson.D{{Key: "$facet", Value: bson.M{
			"items": bson.A{
				bson.M{"$skip": (page - 1) * size},
				bson.M{"$limit": size},
			},
			"total": bson.A{bson.M{"$count": "n"}},
		}}},
	)
	cur, err := s.Spans.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	var rows []struct {
		Items []TraceSummary `bson:"items"`
		Total []struct {
			N int64 `bson:"n"`
		} `bson:"total"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return []TraceSummary{}, 0, nil
	}
	total := int64(0)
	if len(rows[0].Total) > 0 {
		total = rows[0].Total[0].N
	}
	if rows[0].Items == nil {
		rows[0].Items = []TraceSummary{}
	}
	return rows[0].Items, total, nil
}

func (s *Store) TraceSpans(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, traceID string) ([]model.Span, error) {
	filter := bson.M{"tenantId": tenantID, "traceId": traceID}
	if projectID != nil {
		filter["projectId"] = *projectID
	}
	cur, err := s.Spans.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "startTime", Value: 1}}).
		SetLimit(10000))
	if err != nil {
		return nil, err
	}
	out := make([]model.Span, 0)
	err = cur.All(ctx, &out)
	return out, err
}

func (s *Store) TraceServices(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to *time.Time) ([]string, error) {
	filter := bson.M{"tenantId": tenantID}
	if projectID != nil {
		filter["projectId"] = *projectID
	}
	if from != nil || to != nil {
		r := bson.M{}
		if from != nil {
			r["$gte"] = *from
		}
		if to != nil {
			r["$lte"] = *to
		}
		filter["startTime"] = r
	}
	values, err := s.Spans.Distinct(ctx, "service", filter)
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
