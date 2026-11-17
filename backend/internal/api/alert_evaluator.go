package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
)

// StartAlertEvaluator runs the metric alert loop until ctx is cancelled.
func (a *API) StartAlertEvaluator(ctx context.Context) {
	interval := a.cfg.AlertInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		// Let the API and database settle before the first pass.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		a.evaluateAlertRules(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.evaluateAlertRules(ctx)
			}
		}
	}()
}

func (a *API) evaluateAlertRules(ctx context.Context) {
	rules, err := a.store.EnabledAlertRules(ctx)
	if err != nil {
		a.logger.Slog.ErrorContext(ctx, "alert evaluator: list rules failed", "error", err)
		return
	}
	for _, rule := range rules {
		a.evaluateAlertRule(ctx, rule)
	}
}

func (a *API) evaluateAlertRule(ctx context.Context, rule model.AlertRule) {
	now := time.Now().UTC()
	from := now.Add(-time.Duration(rule.WindowSeconds) * time.Second)
	value, hasData, err := a.store.MetricAggregate(ctx, rule.TenantID, rule.ProjectID, rule.Metric, rule.Service, from, rule.Aggregation)
	if err != nil {
		a.logger.Slog.WarnContext(ctx, "alert evaluator: aggregate failed", "ruleId", rule.ID.Hex(), "error", err)
		return
	}
	silenced, silenceErr := a.store.IsRuleSilenced(ctx, rule.TenantID, rule.ProjectID, rule.ID, now)
	if silenceErr != nil {
		a.logger.Slog.WarnContext(ctx, "alert evaluator: silence lookup failed", "ruleId", rule.ID.Hex(), "error", silenceErr)
	}
	if !hasData {
		if rule.State == "firing" {
			a.resolveRuleAlerts(ctx, rule, !silenced)
		}
		_ = a.store.SetAlertRuleResult(ctx, rule.ID, "nodata", 0, false, nil, now)
		return
	}

	firing := compareMetric(value, rule.Operator, rule.Threshold)
	message := alertMessage(rule, value)
	if silenced {
		switch {
		case firing && rule.State == "firing":
			_ = a.store.TouchFiringAlert(ctx, rule.ID, value, message, now)
			_ = a.store.SetAlertRuleResult(ctx, rule.ID, "firing", value, true, rule.FiredAt, now)
		case firing:
			_ = a.store.SetAlertRuleResult(ctx, rule.ID, "silenced", value, true, rule.FiredAt, now)
		case !firing && rule.State == "firing":
			a.resolveRuleAlerts(ctx, rule, false)
			_ = a.store.SetAlertRuleResult(ctx, rule.ID, "ok", value, true, nil, now)
		default:
			_ = a.store.SetAlertRuleResult(ctx, rule.ID, "ok", value, true, nil, now)
		}
		return
	}
	switch {
	case firing && rule.State != "firing":
		alert := &model.Alert{
			TenantID: rule.TenantID, ProjectID: rule.ProjectID, RuleID: rule.ID,
			RuleName: rule.Name, Metric: rule.Metric, Service: rule.Service,
			Severity: rule.Severity, State: "firing", Operator: rule.Operator,
			Threshold: rule.Threshold, Value: value, Message: message,
			StartedAt: now, UpdatedAt: now,
		}
		if err := a.store.CreateAlert(ctx, alert); err != nil {
			if store.IsDuplicate(err) {
				// Another API instance won the firing race. Reuse its alert and
				// enqueue idempotently in case that instance crashed before enqueue.
				existing, _ := a.store.FiringAlert(ctx, rule.ID)
				_ = a.store.SetAlertRuleResult(ctx, rule.ID, "firing", value, true, &now, now)
				if existing != nil {
					a.enqueueAlertNotifications(ctx, rule, *existing, "firing")
				}
				return
			}
			a.logger.Slog.WarnContext(ctx, "alert evaluator: create alert failed", "ruleId", rule.ID.Hex(), "error", err)
			return
		}
		_ = a.store.SetAlertRuleResult(ctx, rule.ID, "firing", value, true, &now, now)
		a.enqueueAlertNotifications(ctx, rule, *alert, "firing")
		a.logger.Slog.WarnContext(ctx, "alert firing", "ruleId", rule.ID.Hex(), "rule", rule.Name, "metric", rule.Metric, "value", value)
	case firing:
		_ = a.store.TouchFiringAlert(ctx, rule.ID, value, message, now)
		_ = a.store.SetAlertRuleResult(ctx, rule.ID, "firing", value, true, rule.FiredAt, now)
	case !firing && rule.State == "firing":
		a.resolveRuleAlerts(ctx, rule, true)
		_ = a.store.SetAlertRuleResult(ctx, rule.ID, "ok", value, true, nil, now)
		a.logger.Slog.InfoContext(ctx, "alert resolved", "ruleId", rule.ID.Hex(), "rule", rule.Name, "value", value)
	default:
		_ = a.store.SetAlertRuleResult(ctx, rule.ID, "ok", value, true, nil, now)
	}
}

func compareMetric(value float64, operator string, threshold float64) bool {
	switch operator {
	case "gt":
		return value > threshold
	case "gte":
		return value >= threshold
	case "lt":
		return value < threshold
	case "lte":
		return value <= threshold
	}
	return false
}

func alertMessage(rule model.AlertRule, value float64) string {
	op := map[string]string{"gt": ">", "gte": "≥", "lt": "<", "lte": "≤"}[rule.Operator]
	agg := map[string]string{"avg": "平均", "max": "最大", "min": "最小", "sum": "总和", "last": "最新"}[rule.Aggregation]
	scope := rule.Metric
	if rule.Service != "" {
		scope = rule.Service + " / " + scope
	}
	return fmt.Sprintf("%s 的%s值为 %s，%s %s", scope, agg, strconv.FormatFloat(value, 'f', -1, 64), op, strconv.FormatFloat(rule.Threshold, 'f', -1, 64))
}
