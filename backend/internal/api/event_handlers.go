package api

import (
	"net/http"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
)

func (a *API) listEvents(w http.ResponseWriter, r *http.Request) {
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
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	switch kind {
	case "", model.KindLog, model.KindError:
	default:
		httpx.Error(w, http.StatusBadRequest, "invalid_kind", "kind 必须是 log 或 error")
		return
	}
	page, size := pageParams(r, 50)
	events, total, err := a.store.ListEvents(r.Context(), store.EventFilter{
		TenantID: t.ID, ProjectID: projectID, Kind: kind,
		Source: normalizeSourceParam(r.URL.Query().Get("source")),
		Level:  normalizeLevelParam(r.URL.Query().Get("level")),
		Query:  r.URL.Query().Get("q"),
		From:   &from, To: &to, Page: page, PageSize: size,
	})
	if err != nil {
		a.logError(r, "list events failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载日志")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"events": events, "total": total, "page": page, "pageSize": size,
		"from": from, "to": to,
	})
}

func (a *API) getEvent(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	e, err := a.store.EventByID(r.Context(), t.ID, id)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "event_not_found", "事件不存在")
			return
		}
		a.logError(r, "load event failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载事件")
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}
