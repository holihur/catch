package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/store"
)

type suggestion struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // critical | warning | info | success
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Action   string `json:"action"`
	IssueID  string `json:"issueId,omitempty"`
}

func (a *API) insights(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	projectID, err := a.projectFilter(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	from, to, err := timeRange(r, "7d")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_range", err.Error())
		return
	}
	span := to.Sub(from)
	prevFrom := from.Add(-span)

	statusCounts, err := a.store.IssueStatusCounts(r.Context(), t.ID, projectID)
	if err != nil {
		a.logError(r, "insights status counts failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法生成改进洞察")
		return
	}
	openLevels, err := a.store.IssueLevelCounts(r.Context(), t.ID, projectID, model.IssueOpen)
	if err != nil {
		a.logError(r, "insights level counts failed", err)
	}
	newIssues, err := a.store.NewIssueCount(r.Context(), t.ID, projectID, from, to)
	if err != nil {
		a.logError(r, "insights new issues failed", err)
	}
	mttr, resolvedCount, err := a.store.MTTRAverage(r.Context(), t.ID, projectID, from, to)
	if err != nil {
		a.logError(r, "insights mttr failed", err)
	}
	regressions, err := a.store.RegressionCount(r.Context(), t.ID, projectID, from)
	if err != nil {
		a.logError(r, "insights regressions failed", err)
	}
	stale, err := a.store.StaleIssueCount(r.Context(), t.ID, projectID, to.Add(-7*24*time.Hour))
	if err != nil {
		a.logError(r, "insights stale failed", err)
	}
	errorsNow, _ := a.store.CountEvents(r.Context(), countFilter(t, projectID, model.KindError, from, to))
	errorsPrev, _ := a.store.CountEvents(r.Context(), countFilter(t, projectID, model.KindError, prevFrom, from))
	logsNow, _ := a.store.CountEvents(r.Context(), countFilter(t, projectID, model.KindLog, from, to))
	logLevels, _ := a.store.LevelCounts(r.Context(), t.ID, projectID, model.KindLog, from, to)
	trend, err := a.store.IssueTrend(r.Context(), t.ID, projectID, from, to)
	if err != nil {
		a.logError(r, "insights issue trend failed", err)
	}
	spikes, err := a.store.IssueSpikes(r.Context(), t.ID, projectID, to)
	if err != nil {
		a.logError(r, "insights spikes failed", err)
	}
	topIssues, err := a.store.TopIssues(r.Context(), t.ID, projectID, from, to, 5)
	if err != nil {
		a.logError(r, "insights top issues failed", err)
	}

	errorLogs := logLevels["error"] + logLevels["fatal"]
	logErrorRatio := 0.0
	if logsNow > 0 {
		logErrorRatio = float64(errorLogs) / float64(logsNow)
	}
	totalTop := int64(0)
	for _, i := range topIssues {
		totalTop += i.Count
	}
	topShare := 0.0
	if len(topIssues) > 0 && totalTop > 0 {
		topShare = float64(topIssues[0].Count) / float64(totalTop)
	}
	change := errorChange(errorsNow, errorsPrev)
	mttrHours := mttr.Hours()

	suggestions := buildSuggestions(suggestionInput{
		openFatal:    openLevels["fatal"],
		openErrors:   openLevels["error"],
		openIssues:   statusCounts[model.IssueOpen],
		regressions:  regressions,
		stale:        stale,
		mttrHours:    mttrHours,
		logErrorRate: logErrorRatio,
		errorChange:  change,
		errorsNow:    errorsNow,
		errorsPrev:   errorsPrev,
		newIssues:    newIssues,
		resolved:     resolvedCount,
		topShare:     topShare,
		topIssues:    topIssues,
		spikes:       spikes,
	})

	if suggestions == nil {
		suggestions = []suggestion{}
	}
	if spikes == nil {
		spikes = []store.IssueSpike{}
	}
	if topIssues == nil {
		topIssues = []model.Issue{}
	}
	if trend == nil {
		trend = []store.IssueTrendPoint{}
	}

	score := healthScore(suggestionInput{
		openFatal:    openLevels["fatal"],
		openErrors:   openLevels["error"],
		openIssues:   statusCounts[model.IssueOpen],
		regressions:  regressions,
		stale:        stale,
		mttrHours:    mttrHours,
		logErrorRate: logErrorRatio,
		errorChange:  change,
		errorsNow:    errorsNow,
		errorsPrev:   errorsPrev,
		newIssues:    newIssues,
		resolved:     resolvedCount,
		topShare:     topShare,
		topIssues:    topIssues,
		spikes:       spikes,
	})

	httpx.JSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to, "rangeDays": int(span.Hours() / 24),
		"health": map[string]any{
			"score": score, "grade": grade(score),
		},
		"metrics": map[string]any{
			"openIssues":      statusCounts[model.IssueOpen],
			"resolvedIssues":  statusCounts[model.IssueResolved],
			"ignoredIssues":   statusCounts[model.IssueIgnored],
			"newIssues":       newIssues,
			"resolvedInRange": resolvedCount,
			"mttrHours":       math.Round(mttrHours*10) / 10,
			"regressions":     regressions,
			"staleIssues":     stale,
			"errors":          errorsNow,
			"prevErrors":      errorsPrev,
			"errorChange":     math.Round(change*10) / 10,
			"logs":            logsNow,
			"logErrorRate":    math.Round(logErrorRatio*1000) / 1000,
			"topIssueShare":   math.Round(topShare*1000) / 1000,
			"openFatal":       openLevels["fatal"],
			"openErrors":      openLevels["error"],
		},
		"trend":       trend,
		"spikes":      spikes,
		"topIssues":   topIssues,
		"suggestions": suggestions,
	})
}

type suggestionInput struct {
	openFatal    int64
	openErrors   int64
	openIssues   int64
	regressions  int64
	stale        int64
	mttrHours    float64
	logErrorRate float64
	errorChange  float64
	errorsNow    int64
	errorsPrev   int64
	newIssues    int64
	resolved     int64
	topShare     float64
	topIssues    []model.Issue
	spikes       []store.IssueSpike
}

func buildSuggestions(in suggestionInput) []suggestion {
	var out []suggestion
	if in.openFatal > 0 {
		out = append(out, suggestion{
			ID: "fatal-open", Severity: "critical",
			Title:  fmt.Sprintf("%d 个未处理的致命错误", in.openFatal),
			Detail: "致命错误通常意味着数据损坏、进程崩溃或核心链路不可用。",
			Action: "在问题列表中按 fatal 筛选，指派负责人并优先修复。",
		})
	}
	if len(in.spikes) > 0 && in.spikes[0].Current > in.spikes[0].Previous && in.spikes[0].Current >= 5 {
		sp := in.spikes[0]
		out = append(out, suggestion{
			ID: "spike", Severity: "warning",
			Title:   fmt.Sprintf("「%s」24 小时错误量激增", truncateText(sp.Title, 60)),
			Detail:  fmt.Sprintf("从过去 24 小时的 %d 次上升到最近 24 小时的 %d 次。", sp.Previous, sp.Current),
			Action:  "检查最近发布、配置变更和依赖服务状态，并补充回归测试。",
			IssueID: sp.IssueID.Hex(),
		})
	}
	if in.regressions > 0 {
		out = append(out, suggestion{
			ID: "regression", Severity: "warning",
			Title:  fmt.Sprintf("%d 个已修复问题发生回归", in.regressions),
			Detail: "问题被标记解决后再次出现，说明修复未覆盖根因或缺少回归测试。",
			Action: "复盘点修复方案，补充自动化回归用例，并检查最近的发布版本。",
		})
	}
	if in.openErrors > 0 && in.openFatal == 0 {
		out = append(out, suggestion{
			ID: "error-open", Severity: "warning",
			Title:  fmt.Sprintf("%d 个未解决的 error 级问题", in.openErrors),
			Detail: "error 级问题会直接影响用户流程，长期积压会掩盖真正的关键故障。",
			Action: "按发生次数排序，优先处理 Top 3；其余设置负责人和解决期限。",
		})
	}
	if in.errorChange >= 20 && in.errorsNow >= 5 {
		out = append(out, suggestion{
			ID: "error-trend", Severity: "warning",
			Title:  fmt.Sprintf("错误量较上一周期上升 %.0f%%", in.errorChange),
			Detail: fmt.Sprintf("当前周期 %d 次，上一周期 %d 次。", in.errorsNow, in.errorsPrev),
			Action: "对比发布记录与流量变化，确认是否为真实质量下降。",
		})
	}
	if in.logErrorRate >= 0.05 {
		out = append(out, suggestion{
			ID: "log-error-rate", Severity: "warning",
			Title:  fmt.Sprintf("日志错误率达到 %.1f%%", in.logErrorRate*100),
			Detail: "高错误日志占比通常伴随上游依赖不稳定或异常处理不完整。",
			Action: "在日志页筛选 backend + error，定位高频错误并补齐告警。",
		})
	}
	if in.mttrHours > 72 && in.resolved > 0 {
		out = append(out, suggestion{
			ID: "mttr", Severity: "info",
			Title:  fmt.Sprintf("平均修复时长 %.1f 小时", in.mttrHours),
			Detail: "修复周期偏长会放大用户影响，并增加回归和上下文丢失的风险。",
			Action: "为高优问题设置 SLA（如 P1 24 小时内），并在详情页记录处理结论。",
		})
	}
	if in.stale > 0 {
		out = append(out, suggestion{
			ID: "stale", Severity: "info",
			Title:  fmt.Sprintf("%d 个问题超过 7 天未复现", in.stale),
			Detail: "长期未复现的问题会稀释看板信号，让团队忽略真正活跃的风险。",
			Action: "确认影响已消失后标记为已解决；仍有关注价值可添加评论后忽略。",
		})
	}
	if in.newIssues > in.resolved && in.newIssues > 0 {
		out = append(out, suggestion{
			ID: "backlog", Severity: "warning",
			Title:  "新增问题多于已解决问题",
			Detail: fmt.Sprintf("本周期新增 %d 个问题，仅解决 %d 个，积压正在扩大。", in.newIssues, in.resolved),
			Action: "安排固定的缺陷治理时间，优先清理高频和影响用户最多的问题。",
		})
	}
	if in.topShare >= 0.3 && len(in.topIssues) > 0 {
		out = append(out, suggestion{
			ID: "noise", Severity: "info",
			Title:   fmt.Sprintf("Top 1 问题占 Top 5 错误量的 %.0f%%", in.topShare*100),
			Detail:  "噪音集中意味着治理单个问题可以显著降低整体错误量。",
			Action:  "先修复「" + truncateText(in.topIssues[0].Title, 50) + "」，观察一周趋势。",
			IssueID: in.topIssues[0].ID.Hex(),
		})
	}
	if len(out) == 0 {
		out = append(out, suggestion{
			ID: "healthy", Severity: "success",
			Title:  "质量指标稳定，没有发现需要立即处理的风险",
			Detail: "当前周期内没有致命错误、回归或明显积压，继续保持。",
			Action: "持续观察趋势，并为关键链路补充错误预算与告警规则。",
		})
	}
	rank := map[string]int{"critical": 0, "warning": 1, "info": 2, "success": 3}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func healthScore(in suggestionInput) int {
	score := 100.0
	if in.openFatal > 0 {
		score -= math.Min(30, 12*float64(in.openFatal))
	}
	if in.openErrors > 0 {
		score -= math.Min(18, 2*float64(in.openErrors))
	}
	if in.regressions > 0 {
		score -= math.Min(15, 5*float64(in.regressions))
	}
	if in.errorChange >= 50 {
		score -= 15
	} else if in.errorChange >= 20 {
		score -= 8
	}
	if in.mttrHours > 72 {
		score -= 10
	} else if in.mttrHours > 24 {
		score -= 5
	}
	if in.logErrorRate >= 0.1 {
		score -= 10
	} else if in.logErrorRate >= 0.05 {
		score -= 5
	}
	if in.stale > 10 {
		score -= 5
	}
	if in.newIssues > in.resolved+5 {
		score -= 5
	}
	score = math.Max(0, math.Min(100, score))
	return int(math.Round(score))
}

func grade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "E"
	}
}

func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
