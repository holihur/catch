package api

import (
	"net/http"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	from, to, err := timeRange(r, "24h")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	prevFrom := from.Add(-to.Sub(from))

	errors, err := a.store.CountEvents(r.Context(), countFilter(t, projectID, model.KindError, from, to))
	if err != nil {
		a.logError(r, "count errors failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载概览")
		return
	}
	logs, err := a.store.CountEvents(r.Context(), countFilter(t, projectID, model.KindLog, from, to))
	if err != nil {
		a.logError(r, "count logs failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载概览")
		return
	}
	prevErrors, err := a.store.CountEvents(r.Context(), countFilter(t, projectID, model.KindError, prevFrom, from))
	if err != nil {
		a.logError(r, "count previous errors failed", err)
	}
	statusCounts, err := a.store.IssueStatusCounts(r.Context(), t.ID, projectID)
	if err != nil {
		a.logError(r, "issue status counts failed", err)
	}
	actors, err := a.store.DistinctActors(r.Context(), t.ID, projectID, from, to)
	if err != nil {
		a.logError(r, "distinct actors failed", err)
	}
	errorSeries, err := a.store.Timeseries(r.Context(), t.ID, projectID, model.KindError, from, to)
	if err != nil {
		a.logError(r, "error timeseries failed", err)
	}
	logSeries, err := a.store.Timeseries(r.Context(), t.ID, projectID, model.KindLog, from, to)
	if err != nil {
		a.logError(r, "log timeseries failed", err)
	}
	levels, err := a.store.LevelCounts(r.Context(), t.ID, projectID, model.KindLog, from, to)
	if err != nil {
		a.logError(r, "level counts failed", err)
	}
	topIssues, err := a.store.TopIssues(r.Context(), t.ID, projectID, from, to, 5)
	if err != nil {
		a.logError(r, "top issues failed", err)
	}
	recentErrors, _, err := a.store.ListEvents(r.Context(), store.EventFilter{
		TenantID: t.ID, ProjectID: projectID, Kind: model.KindError,
		From: &from, To: &to, Page: 1, PageSize: 6,
	})
	if err != nil {
		a.logError(r, "recent errors failed", err)
	}
	newIssues, _ := a.store.NewIssueCount(r.Context(), t.ID, projectID, from, to)
	errorRate := 0.0
	if errors+logs > 0 {
		errorRate = float64(errors) / float64(errors+logs)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to,
		"stats": map[string]any{
			"errors": errors, "logs": logs, "prevErrors": prevErrors,
			"errorChange":    errorChange(errors, prevErrors),
			"openIssues":     statusCounts[model.IssueOpen],
			"resolvedIssues": statusCounts[model.IssueResolved],
			"ignoredIssues":  statusCounts[model.IssueIgnored],
			"affectedUsers":  actors,
			"newIssues":      newIssues,
			"errorRate":      errorRate,
		},
		"timeseries": map[string]any{
			"errors": fillBuckets(from, to, errorSeries),
			"logs":   fillBuckets(from, to, logSeries),
		},
		"levels":       levels,
		"topIssues":    topIssues,
		"recentErrors": recentErrors,
	})
}

func countFilter(t model.Tenant, projectID *primitive.ObjectID, kind string, from, to time.Time) bson.M {
	m := bson.M{
		"tenantId":  t.ID,
		"kind":      kind,
		"timestamp": bson.M{"$gte": from, "$lte": to},
	}
	if projectID != nil {
		m["projectId"] = *projectID
	}
	return m
}

func errorChange(current, prev int64) float64 {
	if prev == 0 {
		if current == 0 {
			return 0
		}
		return 100
	}
	return (float64(current) - float64(prev)) / float64(prev) * 100
}

func fillBuckets(from, to time.Time, in []store.Bucket) []store.Bucket {
	byKey := make(map[string]int64, len(in))
	for _, b := range in {
		byKey[b.T] = b.Count
	}
	hourly := to.Sub(from) <= 48*time.Hour
	out := make([]store.Bucket, 0)
	step := 24 * time.Hour
	format := "2006-01-02"
	cur := from.UTC().Truncate(24 * time.Hour)
	if hourly {
		step = time.Hour
		format = "2006-01-02T15:00:00Z"
		cur = from.UTC().Truncate(time.Hour)
	}
	for !cur.After(to) {
		key := cur.Format(format)
		out = append(out, store.Bucket{T: key, Count: byKey[key]})
		cur = cur.Add(step)
	}
	return out
}
