package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// IssueFilter describes an issue list query.
type IssueFilter struct {
	TenantID  primitive.ObjectID
	ProjectID *primitive.ObjectID
	Status    string // open|resolved|ignored|all
	Level     string
	Source    string
	Query     string
	Sort      string // lastSeen|count|firstSeen
	Page      int
	PageSize  int
}

func (f IssueFilter) match() bson.M {
	m := bson.M{"tenantId": f.TenantID}
	if f.ProjectID != nil {
		m["projectId"] = *f.ProjectID
	}
	if f.Status != "" && f.Status != "all" {
		m["status"] = f.Status
	}
	if f.Level != "" {
		m["level"] = f.Level
	}
	if f.Source != "" {
		m["source"] = f.Source
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		re := primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
		m["$or"] = bson.A{
			bson.M{"title": re},
			bson.M{"culprit": re},
			bson.M{"fingerprint": q},
		}
	}
	return m
}

// UpsertIssue groups an error into an issue, reopening resolved issues and
// recording regressions. It is safe under concurrent ingestion.
func (s *Store) UpsertIssue(ctx context.Context, seed model.Issue) (model.Issue, bool, bool, error) {
	now := time.Now().UTC()
	if seed.LastSeen.IsZero() {
		seed.LastSeen = now
	}
	if seed.FirstSeen.IsZero() {
		seed.FirstSeen = seed.LastSeen
	}
	filter := bson.M{
		"tenantId":    seed.TenantID,
		"projectId":   seed.ProjectID,
		"fingerprint": seed.Fingerprint,
	}
	var existing model.Issue
	err := s.Issues.FindOne(ctx, filter).Decode(&existing)
	if err == nil {
		issue, reopened, uerr := s.bumpIssue(ctx, existing, seed, now)
		return issue, false, reopened, uerr
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return model.Issue{}, false, false, err
	}
	seed.Status = model.IssueOpen
	seed.Count = 1
	seed.CreatedAt = now
	seed.UpdatedAt = now
	seed.RegressionCount = 0
	res, err := s.Issues.InsertOne(ctx, seed)
	if err != nil {
		if IsDuplicate(err) {
			if e := s.Issues.FindOne(ctx, filter).Decode(&existing); e == nil {
				issue, reopened, uerr := s.bumpIssue(ctx, existing, seed, now)
				return issue, false, reopened, uerr
			}
		}
		return model.Issue{}, false, false, err
	}
	seed.ID = res.InsertedID.(primitive.ObjectID)
	return seed, true, false, nil
}

func (s *Store) bumpIssue(ctx context.Context, existing, seed model.Issue, now time.Time) (model.Issue, bool, error) {
	lastSeen := seed.LastSeen
	if lastSeen.Before(existing.LastSeen) {
		lastSeen = existing.LastSeen
	}
	set := bson.M{"updatedAt": now}
	if levelRank(seed.Level) > levelRank(existing.Level) {
		set["level"] = seed.Level
	}
	inc := bson.M{"count": int64(1)}
	unset := bson.M{}
	reopened := false
	if existing.Status == model.IssueResolved {
		set["status"] = model.IssueOpen
		inc["regressionCount"] = 1
		unset["resolvedAt"] = ""
		unset["resolvedBy"] = ""
		reopened = true
	}
	update := bson.M{"$set": set, "$inc": inc, "$max": bson.M{"lastSeen": lastSeen}}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	var out model.Issue
	err := s.Issues.FindOneAndUpdate(ctx,
		bson.M{"_id": existing.ID},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&out)
	if err != nil {
		return out, false, err
	}
	return out, reopened, nil
}

func levelRank(level string) int {
	switch level {
	case "fatal":
		return 5
	case "error":
		return 4
	case "warn":
		return 3
	case "info":
		return 2
	case "debug":
		return 1
	}
	return 0
}

func (s *Store) ListIssues(ctx context.Context, f IssueFilter) ([]model.Issue, int64, error) {
	m := f.match()
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
	total, err := s.Issues.CountDocuments(ctx, m)
	if err != nil {
		return nil, 0, err
	}
	sortKey := "lastSeen"
	switch f.Sort {
	case "count":
		sortKey = "count"
	case "firstSeen":
		sortKey = "firstSeen"
	}
	opts := options.Find().
		SetSort(bson.D{{Key: sortKey, Value: -1}}).
		SetSkip(int64((page - 1) * size)).
		SetLimit(int64(size))
	cur, err := s.Issues.Find(ctx, m, opts)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Issue, 0, size)
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) IssueByID(ctx context.Context, tenantID, id primitive.ObjectID) (model.Issue, error) {
	var issue model.Issue
	err := s.Issues.FindOne(ctx, bson.M{"_id": id, "tenantId": tenantID}).Decode(&issue)
	return issue, err
}

// TopIssues returns the noisiest open issues in a window.
func (s *Store) TopIssues(ctx context.Context, tenantID primitive.ObjectID, projectID *primitive.ObjectID, from, to time.Time, limit int64) ([]model.Issue, error) {
	m := bson.M{
		"tenantId": tenantID,
		"status":   bson.M{"$ne": model.IssueIgnored},
		"lastSeen": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		m["projectId"] = *projectID
	}
	cur, err := s.Issues.Find(ctx, m, options.Find().
		SetSort(bson.D{{Key: "count", Value: -1}}).
		SetLimit(limit))
	if err != nil {
		return nil, err
	}
	out := make([]model.Issue, 0)
	err = cur.All(ctx, &out)
	return out, err
}

func (s *Store) SetIssueStatus(ctx context.Context, tenantID, id primitive.ObjectID, status, actor string) (model.Issue, error) {
	set := bson.M{"status": status, "updatedAt": time.Now().UTC()}
	unset := bson.M{}
	now := time.Now().UTC()
	switch status {
	case model.IssueResolved:
		set["resolvedAt"] = now
		set["resolvedBy"] = actor
	case model.IssueOpen:
		unset["resolvedAt"] = ""
		unset["resolvedBy"] = ""
	case model.IssueIgnored:
		unset["resolvedAt"] = ""
		unset["resolvedBy"] = ""
	}
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	var issue model.Issue
	err := s.Issues.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": tenantID}, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&issue)
	return issue, err
}

func (s *Store) AssignIssue(ctx context.Context, tenantID, id primitive.ObjectID, assigneeID, assigneeName string) (model.Issue, error) {
	set := bson.M{"updatedAt": time.Now().UTC()}
	if assigneeID == "" {
		set["assigneeId"] = ""
		set["assigneeName"] = ""
	} else {
		set["assigneeId"] = assigneeID
		set["assigneeName"] = assigneeName
	}
	var issue model.Issue
	err := s.Issues.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "tenantId": tenantID}, bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&issue)
	return issue, err
}

// ---- comments ----

func (s *Store) AddComment(ctx context.Context, c model.Comment) (model.Comment, error) {
	c.CreatedAt = time.Now().UTC()
	res, err := s.Comments.InsertOne(ctx, c)
	if err != nil {
		return c, err
	}
	c.ID = res.InsertedID.(primitive.ObjectID)
	return c, nil
}

func (s *Store) ListComments(ctx context.Context, tenantID, issueID primitive.ObjectID) ([]model.Comment, error) {
	cur, err := s.Comments.Find(ctx, bson.M{"tenantId": tenantID, "issueId": issueID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(200))
	if err != nil {
		return nil, err
	}
	out := make([]model.Comment, 0)
	err = cur.All(ctx, &out)
	return out, err
}
