package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	"github.com/go-chi/chi/v5"
)

func (a *API) listTraces(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	from, to, err := timeRange(r, "1h")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", "all", "error", "ok":
	default:
		httpx.Error(w, http.StatusBadRequest, "invalid_status", "status 必须是 all、error 或 ok")
		return
	}
	if status == "" {
		status = "all"
	}
	page, size := pageParams(r, 25)
	traces, total, err := a.store.ListTraces(r.Context(), store.TraceFilter{
		TenantID: t.ID, ProjectID: projectID, From: &from, To: &to,
		Service: strings.TrimSpace(r.URL.Query().Get("service")),
		Status:  status, Query: r.URL.Query().Get("q"),
		Page: page, PageSize: size,
	})
	if err != nil {
		a.logError(r, "list traces failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载 Trace")
		return
	}
	for i := range traces {
		traces[i].DurationMS = traces[i].EndTime.Sub(traces[i].StartTime).Milliseconds()
	}
	services, _ := a.store.TraceServices(r.Context(), t.ID, projectID, &from, &to)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"traces": traces, "total": total, "page": page, "pageSize": size,
		"from": from, "to": to, "services": services,
	})
}

func (a *API) traceServices(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	from, to, err := timeRange(r, "1h")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	services, err := a.store.TraceServices(r.Context(), t.ID, projectID, &from, &to)
	if err != nil {
		a.logError(r, "trace services failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载服务列表")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"services": services})
}

func (a *API) getTrace(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	traceID := strings.TrimSpace(chi.URLParam(r, "traceId"))
	if traceID == "" || len(traceID) > 64 {
		httpx.Error(w, http.StatusBadRequest, "invalid_trace_id", "Trace ID 无效")
		return
	}
	spans, err := a.store.TraceSpans(r.Context(), t.ID, projectID, strings.ToLower(traceID))
	if err != nil {
		a.logError(r, "load trace failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载 Trace")
		return
	}
	if len(spans) == 0 {
		httpx.Error(w, http.StatusNotFound, "trace_not_found", "Trace 不存在或已过期")
		return
	}
	summary := summarizeTrace(spans)
	httpx.JSON(w, http.StatusOK, map[string]any{"trace": summary, "spans": spans})
}

func summarizeTrace(spans []model.Span) store.TraceSummary {
	summary := store.TraceSummary{TraceID: spans[0].TraceID, StartTime: spans[0].StartTime, EndTime: spans[0].EndTime}
	services := map[string]bool{}
	rootFound := false
	for _, span := range spans {
		if summary.RootName == "" && span.ParentSpanID == "" {
			summary.RootName = span.Name
			summary.Service = span.Service
			rootFound = true
		}
		if span.StartTime.Before(summary.StartTime) {
			summary.StartTime = span.StartTime
		}
		if span.EndTime.After(summary.EndTime) {
			summary.EndTime = span.EndTime
		}
		if span.Status == "error" {
			summary.ErrorCount++
		}
		if span.Service != "" {
			services[span.Service] = true
		}
	}
	if !rootFound {
		summary.RootName = spans[0].Name
		summary.Service = spans[0].Service
	}
	summary.SpanCount = int64(len(spans))
	summary.DurationMS = summary.EndTime.Sub(summary.StartTime).Milliseconds()
	for service := range services {
		summary.Services = append(summary.Services, service)
	}
	sort.Strings(summary.Services)
	return summary
}
