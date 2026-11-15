package api

import (
	"net/http"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
)

func (a *API) listMetrics(w http.ResponseWriter, r *http.Request) {
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
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	metrics, err := a.store.ListMetrics(r.Context(), t.ID, projectID, from, to, service, r.URL.Query().Get("q"))
	if err != nil {
		a.logError(r, "list metrics failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载指标")
		return
	}
	services, _ := a.store.MetricServices(r.Context(), t.ID, projectID, from, to)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"metrics": metrics, "services": services, "from": from, "to": to,
	})
}

func (a *API) metricSeries(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" || len(name) > 300 {
		httpx.Error(w, http.StatusBadRequest, "invalid_metric", "metric name 无效")
		return
	}
	from, to, err := timeRange(r, "24h")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	points, err := a.store.MetricSeries(r.Context(), t.ID, projectID, name, service, from, to, 5000)
	if err != nil {
		a.logError(r, "metric series failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载指标序列")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"name": name, "service": service, "points": points, "from": from, "to": to,
	})
}
