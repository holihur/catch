import * as React from "react"
import { Link } from "react-router-dom"
import {
  AlertTriangle,
  ArrowRight,
  Bug,
  CheckCircle2,
  Clock,
  GitBranch,
  Info,
  Lightbulb,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  TrendingDown,
  TrendingUp,
} from "lucide-react"

import { api } from "@/lib/api"
import type { Insights, Suggestion } from "@/lib/types"
import { useProjects } from "@/context/ProjectContext"
import { cn, formatNumber, formatPercent, relativeTime } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { TimeRangeSelect } from "@/components/common/TimeRangeSelect"
import { ErrorState } from "@/components/common/ErrorState"
import { EmptyState } from "@/components/common/EmptyState"
import { StatCard } from "@/components/common/StatCard"
import { StatusBadge } from "@/components/common/Badges"
import { IssueTrendChart } from "@/components/charts/TrendChart"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Badge } from "@/components/ui/badge"

const RANGE_OPTIONS = [
  { value: "7d", label: "最近 7 天" },
  { value: "30d", label: "最近 30 天" },
] as const

export function InsightsPage() {
  const { selectedId } = useProjects()
  const [range, setRange] = React.useState("7d")
  const [data, setData] = React.useState<Insights | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [refreshing, setRefreshing] = React.useState(false)
  const [error, setError] = React.useState<unknown>(null)

  const load = React.useCallback(
    async (silent = false) => {
      if (silent) setRefreshing(true)
      else setLoading(true)
      try {
        const res = await api.insights({ projectId: selectedId || undefined, range })
        setData(res)
        setError(null)
      } catch (err) {
        setError(err)
      } finally {
        setLoading(false)
        setRefreshing(false)
      }
    },
    [selectedId, range],
  )

  React.useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="space-y-6 sm:space-y-8">
      <PageHeader
        title="持续改进"
        description="把错误数据转化为可执行的改进项：健康分、回归、修复效率和趋势洞察。"
        actions={
          <>
            <TimeRangeSelect value={range} onChange={setRange} options={RANGE_OPTIONS} />
            <Button variant="outline" size="icon" onClick={() => void load(true)} disabled={refreshing} title="刷新">
              <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} />
            </Button>
          </>
        }
      />

      {error && !data ? <ErrorState error={error} onRetry={() => void load()} /> : null}

      {loading && !data ? (
        <div className="space-y-4">
          <Skeleton className="h-52 rounded-xl" />
          <div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-28 rounded-xl" />
            ))}
          </div>
          <Skeleton className="h-80 rounded-xl" />
        </div>
      ) : data ? (
        <>
          <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
            <HealthCard data={data} />
            <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:col-span-2">
              <StatCard
                label="平均修复时长"
                value={data.metrics.mttrHours > 0 ? `${data.metrics.mttrHours.toFixed(1)}h` : "—"}
                icon={<Clock className="size-4" />}
                accent="info"
                hint={`本周期解决 ${formatNumber(data.metrics.resolvedInRange)} 个问题`}
              />
              <StatCard
                label="回归问题"
                value={data.metrics.regressions}
                icon={<GitBranch className="size-4" />}
                accent={data.metrics.regressions > 0 ? "danger" : "success"}
                hint={data.metrics.regressions > 0 ? "已解决后再次出现" : "没有发现回归"}
              />
              <StatCard
                label="超过 7 天未复现"
                value={data.metrics.staleIssues}
                icon={<Clock className="size-4" />}
                accent="warning"
                hint="可确认后关闭以减少噪音"
              />
              <StatCard
                label="错误量环比"
                value={`${data.metrics.errorChange > 0 ? "+" : ""}${data.metrics.errorChange.toFixed(1)}%`}
                icon={data.metrics.errorChange > 0 ? <TrendingUp className="size-4" /> : <TrendingDown className="size-4" />}
                accent={data.metrics.errorChange > 0 ? "danger" : "success"}
                hint={`${formatNumber(data.metrics.errors)} 次 vs 上周期 ${formatNumber(data.metrics.prevErrors)} 次`}
              />
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
            <Card className="lg:col-span-2">
              <CardHeader className="flex flex-row items-center justify-between">
                <div>
                  <CardTitle className="text-base">问题新增与解决</CardTitle>
                  <p className="text-muted-foreground mt-1 text-xs">
                    每日新增 vs 已解决，观察积压是否在收敛
                  </p>
                </div>
                <div className="text-muted-foreground flex items-center gap-3 text-[11px]">
                  <span className="flex items-center gap-1.5">
                    <span className="size-2 rounded-full bg-chart-4" /> 新增
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="size-2 rounded-full bg-chart-5" /> 解决
                  </span>
                </div>
              </CardHeader>
              <CardContent className="h-[230px] pl-1 sm:h-[260px]">
                <IssueTrendChart data={data.trend} />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">质量指标</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <MetricBar label="日志错误率" value={data.metrics.logErrorRate} max={0.2} format="percent" warn={0.05} />
                <MetricBar
                  label="Top 1 问题占比"
                  value={data.metrics.topIssueShare}
                  max={1}
                  format="percent"
                  warn={0.3}
                />
                <div className="grid grid-cols-2 gap-3 border-t border-border/50 pt-3 text-sm">
                  <MiniMetric label="待处理" value={formatNumber(data.metrics.openIssues)} />
                  <MiniMetric label="已解决" value={formatNumber(data.metrics.resolvedIssues)} />
                  <MiniMetric label="已忽略" value={formatNumber(data.metrics.ignoredIssues)} />
                  <MiniMetric label="本周期新增" value={formatNumber(data.metrics.newIssues)} />
                  <MiniMetric label="Fatal 待处理" value={formatNumber(data.metrics.openFatal)} danger={data.metrics.openFatal > 0} />
                  <MiniMetric label="Error 待处理" value={formatNumber(data.metrics.openErrors)} danger={data.metrics.openErrors > 0} />
                </div>
              </CardContent>
            </Card>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
            <Card className="lg:col-span-2">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <Lightbulb className="size-4 text-warning" /> 改进建议
                </CardTitle>
                <p className="text-muted-foreground mt-1 text-xs">
                  基于当前指标与规则生成，按严重程度排序
                </p>
              </CardHeader>
              <CardContent className="space-y-3">
                {data.suggestions.length === 0 ? (
                  <EmptyState title="暂无建议" description="当前没有需要处理的改进项。" />
                ) : (
                  data.suggestions.map((s) => <SuggestionCard key={s.id} suggestion={s} />)
                )}
              </CardContent>
            </Card>

            <div className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">错误量激增</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  {data.spikes.length === 0 ? (
                    <p className="text-muted-foreground text-sm">最近 24 小时没有明显的错误量激增。</p>
                  ) : (
                    data.spikes.map((s) => (
                      <Link
                        key={s.issueId}
                        to={`/issues/${s.issueId}`}
                        className="hover:bg-muted/50 block rounded-xl bg-muted/20 p-3 transition-colors"
                      >
                        <p className="line-clamp-2 text-xs font-medium">{s.title}</p>
                        <div className="text-muted-foreground mt-2 flex items-center gap-2 text-[11px]">
                          <span className="font-mono">{s.previous}</span>
                          <ArrowRight className="size-3" />
                          <span
                            className={cn(
                              "font-mono font-semibold",
                              s.current > s.previous ? "text-destructive" : "text-success",
                            )}
                          >
                            {s.current}
                          </span>
                          <span className="ml-auto">最近 24h</span>
                        </div>
                      </Link>
                    ))
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="text-base">高频问题</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  {data.topIssues.length === 0 ? (
                    <p className="text-muted-foreground text-sm">暂无高频问题。</p>
                  ) : (
                    data.topIssues.map((issue) => (
                      <Link
                        key={issue.id}
                        to={`/issues/${issue.id}`}
                        className="hover:bg-muted/50 flex items-center gap-2 rounded-lg px-2 py-2 transition-colors"
                      >
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-xs font-medium">{issue.title}</p>
                          <div className="text-muted-foreground mt-0.5 flex items-center gap-1.5 text-[10px]">
                            <StatusBadge status={issue.status} className="scale-90" />
                            <span>{relativeTime(issue.lastSeen)}</span>
                          </div>
                        </div>
                        <span className="font-mono text-xs font-semibold">{formatNumber(issue.count)}</span>
                      </Link>
                    ))
                  )}
                </CardContent>
              </Card>
            </div>
          </div>

          <Card>
            <CardContent className="py-5">
              <div className="mb-4 flex items-center gap-2">
                <Sparkles className="text-primary size-4" />
                <p className="text-sm font-medium">改进闭环</p>
                <span className="text-muted-foreground text-xs">从发现到验证，每一步都有数据支撑</span>
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
                {[
                  { icon: Bug, title: "捕获", desc: "前端 / 后端事件" },
                  { icon: GitBranch, title: "归并", desc: "指纹聚合与回归" },
                  { icon: AlertTriangle, title: "定级", desc: "fatal / error 分级" },
                  { icon: CheckCircle2, title: "修复", desc: "负责人 + 评论记录" },
                  { icon: ShieldCheck, title: "验证", desc: "回归与趋势对比" },
                  { icon: TrendingUp, title: "复盘", desc: "健康分与改进建议" },
                ].map((step, i) => (
                  <div key={step.title} className="bg-muted/30 rounded-xl p-3">
                    <div className="text-muted-foreground mb-2 flex items-center justify-between">
                      <step.icon className="size-4" />
                      <span className="font-mono text-[10px]">0{i + 1}</span>
                    </div>
                    <p className="text-xs font-medium">{step.title}</p>
                    <p className="text-muted-foreground mt-0.5 text-[10px]">{step.desc}</p>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  )
}

function HealthCard({ data }: { data: Insights }) {
  const { score, grade } = data.health
  const circumference = 2 * Math.PI * 46
  const offset = circumference * (1 - score / 100)
  const color = score >= 90 ? "var(--success)" : score >= 75 ? "var(--primary)" : score >= 60 ? "var(--warning)" : "var(--destructive)"
  return (
    <Card className="overflow-hidden">
      <CardContent className="flex h-full flex-col items-center justify-center gap-4 py-6">
        <div className="relative">
          <svg width="132" height="132" viewBox="0 0 120 120" className="-rotate-90">
            <circle cx="60" cy="60" r="46" fill="none" stroke="var(--border)" strokeWidth="8" />
            <circle
              cx="60"
              cy="60"
              r="46"
              fill="none"
              stroke={color}
              strokeWidth="8"
              strokeLinecap="round"
              strokeDasharray={circumference}
              strokeDashoffset={offset}
              className="transition-all duration-700"
            />
          </svg>
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className="text-3xl font-semibold tabular-nums">{score}</span>
            <span className="text-muted-foreground text-[10px]">健康分</span>
          </div>
        </div>
        <div className="text-center">
          <Badge variant="outline" className="mb-1 font-mono">
            等级 {grade}
          </Badge>
          <p className="text-sm font-medium">
            {score >= 90
              ? "质量状态优秀"
              : score >= 75
                ? "质量状态良好"
                : score >= 60
                  ? "需要关注"
                  : "存在明显风险"}
          </p>
          <p className="text-muted-foreground mt-1 text-[11px]">
            基于未处理问题、回归、MTTR 与错误趋势计算
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

const SEVERITY_STYLE: Record<string, { surface: string; icon: React.ReactNode; badge: string; label: string }> = {
  critical: {
    surface: "bg-destructive/5",
    icon: <AlertTriangle className="text-destructive size-4" />,
    badge: "border-destructive/30 text-destructive",
    label: "紧急",
  },
  warning: {
    surface: "bg-warning/5",
    icon: <AlertTriangle className="text-warning size-4" />,
    badge: "border-warning/40 text-warning-foreground dark:text-warning",
    label: "警告",
  },
  info: {
    surface: "bg-muted/30",
    icon: <Info className="text-chart-2 size-4" />,
    badge: "border-chart-2/40 text-chart-2",
    label: "建议",
  },
  success: {
    surface: "bg-success/5",
    icon: <CheckCircle2 className="text-success size-4" />,
    badge: "border-success/40 text-success",
    label: "健康",
  },
}

function SuggestionCard({ suggestion }: { suggestion: Suggestion }) {
  const style = SEVERITY_STYLE[suggestion.severity] ?? SEVERITY_STYLE.info
  return (
    <div className={cn("rounded-xl p-4", style.surface)}>
      <div className="flex items-start gap-3">
        <div className="mt-0.5 shrink-0">{style.icon}</div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm font-medium">{suggestion.title}</p>
            <Badge variant="outline" className={cn("text-[10px]", style.badge)}>
              {style.label}
            </Badge>
          </div>
          <p className="text-muted-foreground mt-1 text-xs leading-relaxed">{suggestion.detail}</p>
          <p className="mt-2 text-xs">
            <span className="text-muted-foreground">建议动作：</span>
            {suggestion.action}
          </p>
          {suggestion.issueId ? (
            <Button asChild variant="link" size="sm" className="mt-1 h-auto p-0 text-xs">
              <Link to={`/issues/${suggestion.issueId}`}>
                查看相关问题 <ArrowRight className="size-3" />
              </Link>
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  )
}

function MetricBar({
  label,
  value,
  max,
  format,
  warn,
}: {
  label: string
  value: number
  max: number
  format: "percent"
  warn: number
}) {
  const pct = Math.min(100, (value / max) * 100)
  const danger = value >= warn * 2
  const warning = value >= warn
  return (
    <div>
      <div className="mb-1 flex items-center justify-between text-xs">
        <span className="text-muted-foreground">{label}</span>
        <span className={cn("font-mono", danger ? "text-destructive" : warning ? "text-warning" : "")}>
          {format === "percent" ? formatPercent(value) : value}
        </span>
      </div>
      <div className="bg-muted h-1.5 overflow-hidden rounded-full">
        <div
          className={cn(
            "h-full rounded-full transition-all",
            danger ? "bg-destructive" : warning ? "bg-warning" : "bg-success",
          )}
          style={{ width: `${Math.max(2, pct)}%` }}
        />
      </div>
    </div>
  )
}

function MiniMetric({ label, value, danger }: { label: string; value: string; danger?: boolean }) {
  return (
    <div>
      <p className="text-muted-foreground text-[10px]">{label}</p>
      <p className={cn("text-sm font-semibold tabular-nums", danger && "text-destructive")}>{value}</p>
    </div>
  )
}
