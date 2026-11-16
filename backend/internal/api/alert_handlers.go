package api

import (
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func validAggregation(value string) bool {
	switch value {
	case "avg", "max", "min", "sum", "last":
		return true
	}
	return false
}

func validOperator(value string) bool {
	switch value {
	case "gt", "gte", "lt", "lte":
		return true
	}
	return false
}

func validSeverity(value string) bool {
	return value == "warning" || value == "critical"
}

func (a *API) listAlertRules(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	var pid primitive.ObjectID
	if projectID != nil {
		pid = *projectID
	}
	rules, err := a.store.ListAlertRules(r.Context(), t.ID, pid)
	if err != nil {
		a.logError(r, "list alert rules failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载告警规则")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (a *API) parseChannelIDs(r *http.Request, tenantID primitive.ObjectID, raw []string) ([]primitive.ObjectID, string) {
	if len(raw) > 10 {
		return nil, "每条规则最多绑定 10 个通知渠道"
	}
	out := make([]primitive.ObjectID, 0, len(raw))
	seen := map[primitive.ObjectID]bool{}
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := primitive.ObjectIDFromHex(value)
		if err != nil {
			return nil, "通知渠道 ID 无效"
		}
		if seen[id] {
			continue
		}
		if _, err := a.store.NotificationChannelByID(r.Context(), tenantID, id); err != nil {
			return nil, "通知渠道不存在或不属于当前租户"
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, ""
}

func (a *API) createAlertRule(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	var body struct {
		Name          string   `json:"name"`
		Metric        string   `json:"metric"`
		Service       string   `json:"service"`
		Aggregation   string   `json:"aggregation"`
		Operator      string   `json:"operator"`
		Threshold     *float64 `json:"threshold"`
		WindowSeconds int      `json:"windowSeconds"`
		Severity      string   `json:"severity"`
		Enabled       *bool    `json:"enabled"`
		ProjectID     string   `json:"projectId"`
		ChannelIDs    []string `json:"channelIds"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	projectID := primitive.NilObjectID
	if strings.TrimSpace(body.ProjectID) != "" {
		id, err := primitive.ObjectIDFromHex(strings.TrimSpace(body.ProjectID))
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_project", "项目 ID 无效")
			return
		}
		if _, err := a.store.ProjectByID(r.Context(), t.ID, id); err != nil {
			if store.IsNoDocuments(err) {
				httpx.Error(w, http.StatusBadRequest, "invalid_project", "项目不存在或不属于当前租户")
				return
			}
			a.logError(r, "lookup alert project failed", err)
			httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法校验项目")
			return
		}
		projectID = id
	} else {
		id, err := a.projectFilter(r)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
			return
		}
		if id != nil {
			projectID = *id
		}
	}
	if projectID == primitive.NilObjectID {
		httpx.Error(w, http.StatusBadRequest, "project_required", "请选择一个项目")
		return
	}
	channelIDs, channelMsg := a.parseChannelIDs(r, t.ID, body.ChannelIDs)
	if channelMsg != "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_channels", channelMsg)
		return
	}
	rule, errMsg := buildAlertRule(t.ID, projectID, body.Name, body.Metric, body.Service, body.Aggregation, body.Operator, body.Threshold, body.WindowSeconds, body.Severity, body.Enabled, channelIDs)
	if errMsg != "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_rule", errMsg)
		return
	}
	if err := a.store.CreateAlertRule(r.Context(), rule); err != nil {
		a.logError(r, "create alert rule failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法创建告警规则")
		return
	}
	a.logger.Slog.InfoContext(r.Context(), "alert rule created", "ruleId", rule.ID.Hex(), "metric", rule.Metric, "actor", currentUser(r).Email)
	httpx.JSON(w, http.StatusCreated, rule)
}

func buildAlertRule(tenantID, projectID primitive.ObjectID, name, metric, service, aggregation, operator string, threshold *float64, window int, severity string, enabled *bool, channelIDs []primitive.ObjectID) (*model.AlertRule, string) {
	name = strings.TrimSpace(name)
	metric = strings.TrimSpace(metric)
	service = strings.TrimSpace(service)
	aggregation = strings.ToLower(strings.TrimSpace(aggregation))
	operator = strings.ToLower(strings.TrimSpace(operator))
	severity = strings.ToLower(strings.TrimSpace(severity))
	if name == "" || len(name) > 100 {
		return nil, "规则名称长度需在 1-100 之间"
	}
	if metric == "" || len(metric) > 200 {
		return nil, "指标名称长度需在 1-200 之间"
	}
	if len(service) > 100 {
		return nil, "服务名过长"
	}
	if aggregation == "" {
		aggregation = "avg"
	}
	if !validAggregation(aggregation) {
		return nil, "聚合方式必须是 avg、max、min、sum 或 last"
	}
	if !validOperator(operator) {
		return nil, "比较符必须是 gt、gte、lt 或 lte"
	}
	if threshold == nil || math.IsNaN(*threshold) || math.IsInf(*threshold, 0) {
		return nil, "阈值必须是有效数字"
	}
	if window == 0 {
		window = 300
	}
	if window < 60 || window > 86400 {
		return nil, "统计窗口需在 60-86400 秒之间"
	}
	if severity == "" {
		severity = "warning"
	}
	if !validSeverity(severity) {
		return nil, "级别必须是 warning 或 critical"
	}
	on := true
	if enabled != nil {
		on = *enabled
	}
	now := time.Now().UTC()
	return &model.AlertRule{
		TenantID: tenantID, ProjectID: projectID, Name: name, Metric: metric, Service: service,
		Aggregation: aggregation, Operator: operator, Threshold: *threshold, WindowSeconds: window,
		Severity: severity, ChannelIDs: channelIDs, Enabled: on, State: "pending", CreatedAt: now, UpdatedAt: now,
	}, ""
}

func (a *API) updateAlertRule(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var body struct {
		Name          *string   `json:"name"`
		Metric        *string   `json:"metric"`
		Service       *string   `json:"service"`
		Aggregation   *string   `json:"aggregation"`
		Operator      *string   `json:"operator"`
		Threshold     *float64  `json:"threshold"`
		WindowSeconds *int      `json:"windowSeconds"`
		Severity      *string   `json:"severity"`
		Enabled       *bool     `json:"enabled"`
		ChannelIDs    *[]string `json:"channelIds"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	set := bson.M{}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 100 {
			httpx.Error(w, http.StatusBadRequest, "invalid_name", "规则名称长度需在 1-100 之间")
			return
		}
		set["name"] = name
	}
	if body.Metric != nil {
		metric := strings.TrimSpace(*body.Metric)
		if metric == "" || len(metric) > 200 {
			httpx.Error(w, http.StatusBadRequest, "invalid_metric", "指标名称长度需在 1-200 之间")
			return
		}
		set["metric"] = metric
	}
	if body.Service != nil {
		set["service"] = strings.TrimSpace(*body.Service)
	}
	if body.Aggregation != nil {
		aggregation := strings.ToLower(strings.TrimSpace(*body.Aggregation))
		if !validAggregation(aggregation) {
			httpx.Error(w, http.StatusBadRequest, "invalid_aggregation", "聚合方式无效")
			return
		}
		set["aggregation"] = aggregation
	}
	if body.Operator != nil {
		operator := strings.ToLower(strings.TrimSpace(*body.Operator))
		if !validOperator(operator) {
			httpx.Error(w, http.StatusBadRequest, "invalid_operator", "比较符无效")
			return
		}
		set["operator"] = operator
	}
	if body.Threshold != nil {
		if math.IsNaN(*body.Threshold) || math.IsInf(*body.Threshold, 0) {
			httpx.Error(w, http.StatusBadRequest, "invalid_threshold", "阈值必须是有效数字")
			return
		}
		set["threshold"] = *body.Threshold
	}
	if body.WindowSeconds != nil {
		if *body.WindowSeconds < 60 || *body.WindowSeconds > 86400 {
			httpx.Error(w, http.StatusBadRequest, "invalid_window", "统计窗口需在 60-86400 秒之间")
			return
		}
		set["windowSeconds"] = *body.WindowSeconds
	}
	if body.Severity != nil {
		severity := strings.ToLower(strings.TrimSpace(*body.Severity))
		if !validSeverity(severity) {
			httpx.Error(w, http.StatusBadRequest, "invalid_severity", "级别必须是 warning 或 critical")
			return
		}
		set["severity"] = severity
	}
	if body.ChannelIDs != nil {
		channelIDs, channelMsg := a.parseChannelIDs(r, t.ID, *body.ChannelIDs)
		if channelMsg != "" {
			httpx.Error(w, http.StatusBadRequest, "invalid_channels", channelMsg)
			return
		}
		set["channelIds"] = channelIDs
	}
	if body.Enabled != nil {
		set["enabled"] = *body.Enabled
		if !*body.Enabled {
			set["state"] = "ok"
		}
	}
	if len(set) == 0 {
		httpx.Error(w, http.StatusBadRequest, "empty_update", "没有需要更新的字段")
		return
	}
	rule, err := a.store.UpdateAlertRule(r.Context(), t.ID, id, set)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusNotFound, "rule_not_found", "告警规则不存在")
			return
		}
		a.logError(r, "update alert rule failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新告警规则")
		return
	}
	if body.Enabled != nil && !*body.Enabled {
		a.resolveRuleAlerts(r.Context(), *rule)
	}
	httpx.JSON(w, http.StatusOK, rule)
}

func (a *API) deleteAlertRule(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if rule, err := a.store.AlertRuleByID(r.Context(), t.ID, id); err == nil {
		a.resolveRuleAlerts(r.Context(), *rule)
	}
	if err := a.store.DeleteAlertRule(r.Context(), t.ID, id); err != nil {
		a.logError(r, "delete alert rule failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法删除告警规则")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) listAlerts(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	state := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	switch state {
	case "", "all", "firing", "resolved":
	default:
		httpx.Error(w, http.StatusBadRequest, "invalid_status", "status 必须是 all、firing 或 resolved")
		return
	}
	limit := int64(100)
	var pid primitive.ObjectID
	if projectID != nil {
		pid = *projectID
	}
	alerts, err := a.store.ListAlerts(r.Context(), t.ID, pid, state, limit)
	if err != nil {
		a.logError(r, "list alerts failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载告警")
		return
	}
	firing, _ := a.store.CountFiringAlerts(r.Context(), t.ID, pid)
	httpx.JSON(w, http.StatusOK, map[string]any{"alerts": alerts, "firing": firing})
}
