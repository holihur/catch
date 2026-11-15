package store

import (
	"context"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// IssueStatusCounts returns issue counts per status for a tenant/project.
func (s *Store) IssueStatusCounts(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID) (map[string]int64, error) {
	match := bson.M{"tenantId": tenantID}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{"_id": "$status", "count": bson.M{"$sum": 1}}}},
	}
	cur, err := s.Issues.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID    string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := map[string]int64{model.IssueOpen: 0, model.IssueResolved: 0, model.IssueIgnored: 0}
	for _, r := range rows {
		out[r.ID] = r.Count
	}
	return out, nil
}

// NewIssueCount counts issues first seen in a window.
func (s *Store) NewIssueCount(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to time.Time) (int64, error) {
	m := bson.M{"tenantId": tenantID, "firstSeen": bson.M{"$gte": from, "$lte": to}}
	if projectID != nil {
		m["projectId"] = *projectID
	}
	return s.Issues.CountDocuments(ctx, m)
}

// MTTRAverage returns the mean time-to-resolution for issues resolved in a window.
func (s *Store) MTTRAverage(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to time.Time) (time.Duration, int64, error) {
	match := bson.M{
		"tenantId":   tenantID,
		"resolvedAt": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$project", Value: bson.M{"ms": bson.M{"$subtract": bson.A{"$resolvedAt", "$firstSeen"}}}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "avg": bson.M{"$avg": "$ms"}, "n": bson.M{"$sum": 1}}}},
	}
	cur, err := s.Issues.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, err
	}
	var rows []struct {
		Avg float64 `bson:"avg"`
		N   int64   `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return 0, 0, err
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	return time.Duration(rows[0].Avg) * time.Millisecond, rows[0].N, nil
}

// StaleIssueCount counts open issues that have not recurred for `before`.
func (s *Store) StaleIssueCount(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, before time.Time) (int64, error) {
	m := bson.M{"tenantId": tenantID, "status": model.IssueOpen, "lastSeen": bson.M{"$lt": before}}
	if projectID != nil {
		m["projectId"] = *projectID
	}
	return s.Issues.CountDocuments(ctx, m)
}

// RegressionCount counts issues that regressed in a window.
func (s *Store) RegressionCount(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from time.Time) (int64, error) {
	m := bson.M{
		"tenantId":        tenantID,
		"regressionCount": bson.M{"$gt": 0},
		"lastSeen":        bson.M{"$gte": from},
	}
	if projectID != nil {
		m["projectId"] = *projectID
	}
	return s.Issues.CountDocuments(ctx, m)
}

// IssueTrendPoint is a new/resolved pair for one day.
type IssueTrendPoint struct {
	T        string `bson:"_id" json:"t"`
	New      int64  `bson:"new" json:"new"`
	Resolved int64  `bson:"resolved" json:"resolved"`
}

// IssueTrend aggregates issue creation and resolution by day.
func (s *Store) IssueTrend(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to time.Time) ([]IssueTrendPoint, error) {
	match := bson.M{"tenantId": tenantID}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$project", Value: bson.M{
			"firstSeen":  1,
			"resolvedAt": 1,
			"firstDay":   bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$firstSeen", "timezone": "UTC"}},
			"resolvedDay": bson.M{"$cond": bson.A{
				bson.M{"$and": bson.A{
					bson.M{"$ne": bson.A{"$resolvedAt", nil}},
					bson.M{"$gte": bson.A{"$resolvedAt", from}},
					bson.M{"$lte": bson.A{"$resolvedAt", to}},
				}},
				bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$resolvedAt", "timezone": "UTC"}},
				nil,
			}},
		}}},
		{{Key: "$facet", Value: bson.M{
			"new": bson.A{
				bson.M{"$match": bson.M{"firstSeen": bson.M{"$gte": from, "$lte": to}}},
				bson.M{"$group": bson.M{"_id": "$firstDay", "n": bson.M{"$sum": 1}}},
			},
			"resolved": bson.A{
				bson.M{"$match": bson.M{"resolvedDay": bson.M{"$ne": nil}}},
				bson.M{"$group": bson.M{"_id": "$resolvedDay", "n": bson.M{"$sum": 1}}},
			},
		}}},
	}
	cur, err := s.Issues.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var facets []struct {
		New []struct {
			ID string `bson:"_id"`
			N  int64  `bson:"n"`
		} `bson:"new"`
		Resolved []struct {
			ID string `bson:"_id"`
			N  int64  `bson:"n"`
		} `bson:"resolved"`
	}
	if err := cur.All(ctx, &facets); err != nil {
		return nil, err
	}
	if len(facets) == 0 {
		return nil, nil
	}
	points := map[string]*IssueTrendPoint{}
	for _, r := range facets[0].New {
		if r.ID == "" {
			continue
		}
		points[r.ID] = &IssueTrendPoint{T: r.ID, New: r.N}
	}
	for _, r := range facets[0].Resolved {
		if r.ID == "" {
			continue
		}
		p := points[r.ID]
		if p == nil {
			p = &IssueTrendPoint{T: r.ID}
			points[r.ID] = p
		}
		p.Resolved = r.N
	}
	// Fill missing days so charts render continuously.
	out := make([]IssueTrendPoint, 0, len(points))
	for d := from.UTC().Truncate(24 * time.Hour); !d.After(to); d = d.Add(24 * time.Hour) {
		key := d.Format("2006-01-02")
		if p, ok := points[key]; ok {
			out = append(out, *p)
		} else {
			out = append(out, IssueTrendPoint{T: key})
		}
	}
	return out, nil
}

// IssueSpike compares errors per issue in two adjacent windows.
type IssueSpike struct {
	IssueID   primitive.ObjectID `bson:"_id" json:"issueId"`
	Current   int64              `bson:"current" json:"current"`
	Previous  int64              `bson:"previous" json:"previous"`
	Title     string             `bson:"title" json:"title"`
	Level     string             `bson:"level" json:"level"`
	ProjectID primitive.ObjectID `bson:"projectId" json:"projectId"`
}

// IssueSpikes returns issues with the largest absolute increase in the last 24h.
func (s *Store) IssueSpikes(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, now time.Time) ([]IssueSpike, error) {
	dayAgo := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)
	match := bson.M{
		"tenantId":  tenantID,
		"kind":      model.KindError,
		"timestamp": bson.M{"$gte": twoDaysAgo},
	}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id": "$issueId",
			"current": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$gte": bson.A{"$timestamp", dayAgo}}, 1, 0,
			}}},
			"previous": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$lt": bson.A{"$timestamp", dayAgo}}, 1, 0,
			}}},
		}}},
		{{Key: "$match", Value: bson.M{"current": bson.M{"$gt": 0}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "issues",
			"localField":   "_id",
			"foreignField": "_id",
			"as":           "issue",
		}}},
		{{Key: "$unwind", Value: "$issue"}},
		{{Key: "$project", Value: bson.M{
			"current":   1,
			"previous":  1,
			"title":     "$issue.title",
			"level":     "$issue.level",
			"projectId": "$issue.projectId",
			"delta":     bson.M{"$subtract": bson.A{"$current", "$previous"}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "delta", Value: -1}, {Key: "current", Value: -1}}}},
		{{Key: "$limit", Value: 5}},
	}
	cur, err := s.Issues.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var out []IssueSpike
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ProjectLookup returns a map of project id -> name for a tenant.
func (s *Store) ProjectNames(ctx context.Context, tenantID primitive.ObjectID) (map[primitive.ObjectID]string, error) {
	cur, err := s.Projects.Find(ctx, bson.M{"tenantId": tenantID}, nil)
	if err != nil {
		return nil, err
	}
	var projects []model.Project
	if err := cur.All(ctx, &projects); err != nil {
		return nil, err
	}
	out := make(map[primitive.ObjectID]string, len(projects))
	for _, p := range projects {
		out[p.ID] = p.Name
	}
	return out, nil
}

// DistinctActorsForIssue counts unique end users affected by one issue.
func (s *Store) DistinctActorsForIssue(ctx context.Context, tenantID, issueID primitive.ObjectID) (int, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"tenantId": tenantID, "issueId": issueID,
			"user.id": bson.M{"$nin": bson.A{"", nil}},
		}}},
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

// IssueLevelCounts groups issues by level for a given status.
func (s *Store) IssueLevelCounts(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, status string) (map[string]int64, error) {
	match := bson.M{"tenantId": tenantID}
	if status != "" {
		match["status"] = status
	}
	if projectID != nil {
		match["projectId"] = *projectID
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{"_id": "$level", "count": bson.M{"$sum": 1}}}},
	}
	cur, err := s.Issues.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID    string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, r := range rows {
		out[r.ID] = r.Count
	}
	return out, nil
}
