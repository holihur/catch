package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func parseSilenceTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(raw))
}

func (a *API) silenceProject(r *http.Request, raw string) (*primitive.ObjectID, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "all" {
		return nil, ""
	}
	id, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		return nil, "项目 ID 无效"
	}
	t := currentTenant(r)
	if _, err := a.store.ProjectByID(r.Context(), t.ID, id); err != nil {
		if store.IsNoDocuments(err) {
			return nil, "项目不存在或不属于当前租户"
		}
		return nil, "无法校验项目"
	}
	return &id, ""
}

func (a *API) silenceRules(r *http.Request, raw []string, projectID *primitive.ObjectID) ([]primitive.ObjectID, string) {
	if len(raw) > 50 {
		return nil, "最多选择 50 条规则"
	}
	t := currentTenant(r)
	out := make([]primitive.ObjectID, 0, len(raw))
	seen := map[primitive.ObjectID]bool{}
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := primitive.ObjectIDFromHex(value)
		if err != nil {
			return nil, "告警规则 ID 无效"
		}
		if seen[id] {
			continue
		}
		rule, err := a.store.AlertRuleByID(r.Context(), t.ID, id)
		if err != nil {
			return nil, "告警规则不存在或不属于当前租户"
		}
		if projectID != nil && rule.ProjectID != *projectID {
			return nil, "规则不属于所选项目"
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, ""
}

func (a *API) listAlertSilences(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	silences, err := a.store.ListAlertSilences(r.Context(), t.ID, projectID)
	if err != nil {
		a.logError(r, "list alert silences failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载静默窗口")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"silences": silences})
}

func (a *API) createAlertSilence(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	var body struct {
		Name      string   `json:"name"`
		Reason    string   `json:"reason"`
		ProjectID string   `json:"projectId"`
		RuleIDs   []string `json:"ruleIds"`
		StartsAt  string   `json:"startsAt"`
		EndsAt    string   `json:"endsAt"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 32<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Name == "" || len(body.Name) > 100 {
		httpx.Error(w, http.StatusBadRequest, "invalid_name", "静默名称长度需在 1-100 之间")
		return
	}
	if len(body.Reason) > 500 {
		httpx.Error(w, http.StatusBadRequest, "invalid_reason", "原因不能超过 500 字")
		return
	}
	projectID, msg := a.silenceProject(r, body.ProjectID)
	if msg != "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", msg)
		return
	}
	ruleIDs, msg := a.silenceRules(r, body.RuleIDs, projectID)
	if msg != "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_rules", msg)
		return
	}
	startsAt, err := parseSilenceTime(body.StartsAt)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_start", "开始时间格式无效")
		return
	}
	endsAt, err := parseSilenceTime(body.EndsAt)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_end", "结束时间格式无效")
		return
	}
	now := time.Now().UTC()
	if !endsAt.After(startsAt) {
		httpx.Error(w, http.StatusBadRequest, "invalid_window", "结束时间必须晚于开始时间")
		return
	}
	if !endsAt.After(now) {
		httpx.Error(w, http.StatusBadRequest, "invalid_window", "结束时间必须晚于当前时间")
		return
	}
	silence := &model.AlertSilence{
		TenantID: t.ID, ProjectID: projectID, Name: body.Name, Reason: body.Reason,
		StartsAt: startsAt.UTC(), EndsAt: endsAt.UTC(), RuleIDs: ruleIDs,
		CreatedBy: currentUser(r).Email, CreatedAt: now, UpdatedAt: now,
	}
	if err := a.store.CreateAlertSilence(r.Context(), silence); err != nil {
		a.logError(r, "create alert silence failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法创建静默窗口")
		return
	}
	a.logger.Slog.InfoContext(r.Context(), "alert silence created", "silenceId", silence.ID.Hex(), "name", silence.Name, "actor", currentUser(r).Email)
	httpx.JSON(w, http.StatusCreated, silence)
}

func (a *API) updateAlertSilence(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	silence, err := a.store.AlertSilenceByID(r.Context(), t.ID, id)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "silence_not_found", "静默窗口不存在")
			return
		}
		a.logError(r, "load alert silence failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载静默窗口")
		return
	}
	var body struct {
		Name     *string   `json:"name"`
		Reason   *string   `json:"reason"`
		RuleIDs  *[]string `json:"ruleIds"`
		StartsAt *string   `json:"startsAt"`
		EndsAt   *string   `json:"endsAt"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 32<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 100 {
			httpx.Error(w, http.StatusBadRequest, "invalid_name", "静默名称长度需在 1-100 之间")
			return
		}
		silence.Name = name
	}
	if body.Reason != nil {
		reason := strings.TrimSpace(*body.Reason)
		if len(reason) > 500 {
			httpx.Error(w, http.StatusBadRequest, "invalid_reason", "原因不能超过 500 字")
			return
		}
		silence.Reason = reason
	}
	if body.RuleIDs != nil {
		ruleIDs, msg := a.silenceRules(r, *body.RuleIDs, silence.ProjectID)
		if msg != "" {
			httpx.Error(w, http.StatusBadRequest, "invalid_rules", msg)
			return
		}
		silence.RuleIDs = ruleIDs
	}
	if body.StartsAt != nil {
		startsAt, err := parseSilenceTime(*body.StartsAt)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_start", "开始时间格式无效")
			return
		}
		silence.StartsAt = startsAt.UTC()
	}
	if body.EndsAt != nil {
		endsAt, err := parseSilenceTime(*body.EndsAt)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_end", "结束时间格式无效")
			return
		}
		silence.EndsAt = endsAt.UTC()
	}
	if !silence.EndsAt.After(silence.StartsAt) {
		httpx.Error(w, http.StatusBadRequest, "invalid_window", "结束时间必须晚于开始时间")
		return
	}
	set := bson.M{
		"name": silence.Name, "reason": silence.Reason, "ruleIds": silence.RuleIDs,
		"startsAt": silence.StartsAt, "endsAt": silence.EndsAt,
	}
	updated, err := a.store.UpdateAlertSilence(r.Context(), t.ID, id, set)
	if err != nil {
		a.logError(r, "update alert silence failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新静默窗口")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (a *API) deleteAlertSilence(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := a.store.DeleteAlertSilence(r.Context(), t.ID, id); err != nil {
		a.logError(r, "delete alert silence failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法删除静默窗口")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
