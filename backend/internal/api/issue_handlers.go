package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
)

func (a *API) listIssues(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = "open"
	}
	switch status {
	case "open", "resolved", "ignored", "all":
	default:
		httpx.Error(w, http.StatusBadRequest, "invalid_status", "status 必须是 open、resolved、ignored 或 all")
		return
	}
	level := normalizeLevelParam(r.URL.Query().Get("level"))
	source := normalizeSourceParam(r.URL.Query().Get("source"))
	page, size := pageParams(r, 25)
	issues, total, err := a.store.ListIssues(r.Context(), store.IssueFilter{
		TenantID: t.ID, ProjectID: projectID, Status: status, Level: level, Source: source,
		Query: r.URL.Query().Get("q"), Sort: r.URL.Query().Get("sort"), Page: page, PageSize: size,
	})
	if err != nil {
		a.logError(r, "list issues failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载问题列表")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"issues": issues, "total": total, "page": page, "pageSize": size,
	})
}

func (a *API) getIssue(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	issue, err := a.store.IssueByID(r.Context(), t.ID, id)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "issue_not_found", "问题不存在")
			return
		}
		a.logError(r, "load issue failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载问题")
		return
	}
	events, _, err := a.store.ListEvents(r.Context(), store.EventFilter{
		TenantID: t.ID, IssueID: &id, Page: 1, PageSize: 20,
	})
	if err != nil {
		a.logError(r, "load issue events failed", err)
	}
	comments, err := a.store.ListComments(r.Context(), t.ID, id)
	if err != nil {
		a.logError(r, "load issue comments failed", err)
	}
	now := time.Now().UTC()
	trend, err := a.store.EventCountsByIssue(r.Context(), t.ID, id, now.Add(-7*24*time.Hour), now)
	if err != nil {
		a.logError(r, "load issue trend failed", err)
	}
	actors, _ := a.store.DistinctActorsForIssue(r.Context(), t.ID, id)
	var projectName string
	if !issue.ProjectID.IsZero() {
		if p, e := a.store.ProjectByID(r.Context(), t.ID, issue.ProjectID); e == nil {
			projectName = p.Name
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"issue": issue, "events": events, "comments": comments,
		"trend": trend, "userCount": actors, "projectName": projectName,
	})
}

func (a *API) updateIssue(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	user := currentUser(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var body struct {
		Status     *string `json:"status"`
		AssigneeID *string `json:"assigneeId"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if body.Status == nil && body.AssigneeID == nil {
		httpx.Error(w, http.StatusBadRequest, "empty_update", "没有需要更新的字段")
		return
	}
	issue, err := a.store.IssueByID(r.Context(), t.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "issue_not_found", "问题不存在")
		return
	}
	actor := user.Name
	if actor == "" {
		actor = user.Email
	}
	if body.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*body.Status))
		switch status {
		case model.IssueOpen, model.IssueResolved, model.IssueIgnored:
		default:
			httpx.Error(w, http.StatusBadRequest, "invalid_status", "status 必须是 open、resolved 或 ignored")
			return
		}
		issue, err = a.store.SetIssueStatus(r.Context(), t.ID, id, status, actor)
		if err != nil {
			a.logError(r, "update issue status failed", err)
			httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新问题状态")
			return
		}
		a.logger.Slog.InfoContext(r.Context(), "issue status changed",
			"issueId", id.Hex(), "status", status, "actor", user.Email)
	}
	if body.AssigneeID != nil {
		assigneeID := strings.TrimSpace(*body.AssigneeID)
		name := ""
		if assigneeID != "" {
			uid, err := parseID(assigneeID)
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid_assignee", "指派人 ID 无效")
				return
			}
			target, err := a.store.UserByID(r.Context(), t.ID, uid)
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid_assignee", "指派人不是当前租户成员")
				return
			}
			name = target.Name
			if name == "" {
				name = target.Email
			}
		}
		issue, err = a.store.AssignIssue(r.Context(), t.ID, id, assigneeID, name)
		if err != nil {
			a.logError(r, "assign issue failed", err)
			httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法指派问题")
			return
		}
	}
	httpx.JSON(w, http.StatusOK, issue)
}

func (a *API) addComment(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	user := currentUser(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if _, err := a.store.IssueByID(r.Context(), t.ID, id); err != nil {
		httpx.Error(w, http.StatusNotFound, "issue_not_found", "问题不存在")
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" || len(text) > 5000 {
		httpx.Error(w, http.StatusBadRequest, "invalid_comment", "评论长度需在 1-5000 之间")
		return
	}
	name := user.Name
	if name == "" {
		name = user.Email
	}
	comment, err := a.store.AddComment(r.Context(), model.Comment{
		TenantID: t.ID, IssueID: id, UserID: user.ID, AuthorName: name, Body: text,
	})
	if err != nil {
		a.logError(r, "add comment failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存评论")
		return
	}
	httpx.JSON(w, http.StatusCreated, comment)
}

func normalizeLevelParam(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug", "info", "warn", "error", "fatal":
		return strings.ToLower(strings.TrimSpace(v))
	}
	return ""
}

func normalizeSourceParam(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case model.SourceFrontend, model.SourceBackend:
		return strings.ToLower(strings.TrimSpace(v))
	}
	return ""
}
