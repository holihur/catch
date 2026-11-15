import * as React from "react"
import { Link } from "react-router-dom"
import {
  ArrowRight,
  Bug,
  CircleAlert,
  RefreshCw,
  ScrollText,
  Sparkles,
  Users,
} from "lucide-react"

import { api } from "@/lib/api"
import type { Overview } from "@/lib/types"
import { useProjects } from "@/context/ProjectContext"
import { formatNumber, formatPercent, relativeTime, statusMeta } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { StatCard } from "@/components/common/StatCard"
import { TimeRangeSelect } from "@/components/common/TimeRangeSelect"
import { ErrorState } from "@/components/common/ErrorState"
import { EmptyState } from "@/components/common/EmptyState"
import { LevelMark, SourceBadge } from "@/components/common/Badges"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Badge } from "@/components/ui/badge"
import { ErrorTrendChart, LevelDonut } from "@/components/charts/TrendChart"

export function OverviewPage() {
  const { selectedId } = useProjects()
  const [range, setRange] = React.useState("24h")
  const [data, setData] = React.useState<Overview | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [refreshing, setRefreshing] = React.useState(false)
  const [error, setError] = React.useState<unknown>(null)
  const [auto, setAuto] = React.useState(true)

  const load = React.useCallback(
    async (silent = false) => {
      if (silent) setRefreshing(true)
      else setLoading(true)
      try {
        const res = await api.overview({ projectId: selectedId || undefined, range })
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

  React.useEffect(() => {
    if (!auto) return
    const id = window.setInterval(() => void load(true), 30_000)
    return () => window.clearInterval(id)
  }, [auto, load])

  return (
    <div className="space-y-6 sm:space-y-8">
      <PageHeader
        title="概览"
        description="前端与后端错误的实时视图，聚焦最近发生的风险与趋势。"
        actions={
          <>
            <div className="text-muted-foreground mr-1 hidden items-center gap-2 text-xs sm:flex">
              <Switch checked={auto} onCheckedChange={setAuto} id="auto-refresh" />
              <label htmlFor="auto-refresh" className="cursor-pointer select-none">
                自动刷新
              </label>
            </div>
            <TimeRangeSelect value={range} onChange={setRange} />
            <Button variant="outline" size="icon" onClick={() => void load(true)} disabled={refreshing} title="刷新">
              <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} />
            </Button>
          </>
        }
      />

      {error && !data ? <ErrorState error={error} onRetry={() => void load()} /> : null}

      {loading && !data ? (
        <OverviewSkeleton />
      ) : data ? (
        <>
          <div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">
            <StatCard
              label="错误事件"
              value={data.stats.errors}
              icon={<Bug className="size-4" />}
              accent="danger"
              delta={data.stats.errorChange}
              deltaLabel="环比"
              hint={`上一周期 ${formatNumber(data.stats.prevErrors)} 次`}
              invertDelta
            />
            <StatCard
              label="日志事件"
              value={data.stats.logs}
              icon={<ScrollText className="size-4" />}
              accent="info"
              hint={`错误率 ${formatPercent(data.stats.errorRate)}`}
            />
            <StatCard
              label="待处理问题"
              value={data.stats.openIssues}
              icon={<CircleAlert className="size-4" />}
              accent="warning"
              hint={`新增 ${formatNumber(data.stats.newIssues)} · 已解决 ${formatNumber(data.stats.resolvedIssues)}`}
            />
            <StatCard
              label="受影响用户"
              value={data.stats.affectedUsers}
              icon={<Users className="size-4" />}
              accent="success"
              hint="按错误事件去重统计"
            />
          </div>

          <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
            <Card className="lg:col-span-2">
              <CardHeader className="flex flex-row items-center justify-between gap-2">
                <div>
                  <CardTitle className="text-base">错误与日志趋势</CardTitle>
                  <p className="text-muted-foreground mt-1 text-xs">
                    {data.stats.errors.toLocaleString("zh-CN")} 次错误 ·{" "}
                    {data.stats.logs.toLocaleString("zh-CN")} 条日志
                  </p>
                </div>
                <div className="text-muted-foreground flex items-center gap-3 text-[11px]">
                  <span className="flex items-center gap-1.5">
                    <span className="size-2 rounded-full bg-chart-4" /> 错误
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="size-2 rounded-full bg-chart-2" /> 日志
                  </span>
                </div>
              </CardHeader>
              <CardContent className="h-[240px] pl-2 sm:h-[300px]">
                <ErrorTrendChart errors={data.timeseries.errors} logs={data.timeseries.logs} />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">日志级别分布</CardTitle>
                <p className="text-muted-foreground mt-1 text-xs">按事件级别统计的日志占比</p>
              </CardHeader>
              <CardContent>
                <LevelDonut levels={data.levels} />
              </CardContent>
            </Card>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
            <Card className="lg:col-span-2">
              <CardHeader className="flex flex-row items-center justify-between">
                <CardTitle className="text-base">最近错误</CardTitle>
                <Button asChild variant="ghost" size="sm">
                  <Link to="/issues">
                    全部问题 <ArrowRight className="size-3.5" />
                  </Link>
                </Button>
              </CardHeader>
              <CardContent className="space-y-1">
                {data.recentErrors.length === 0 ? (
                  <EmptyState title="暂无错误" description="所选时间范围内没有捕获到错误。" className="py-10" />
                ) : (
                  data.recentErrors.map((e) => {
                    const href = e.issueId && !e.issueId.startsWith("000000") ? `/issues/${e.issueId}` : "/logs"
                    return (
                      <Link
                        key={e.id}
                        to={href}
                        className="hover:bg-muted/60 group flex items-start gap-3 rounded-lg px-2 py-2.5 transition-colors"
                      >
                        <LevelMark level={e.level} className="mt-0.5" />
                        <div className="min-w-0 flex-1">
                          <p className="group-hover:text-primary truncate text-sm font-medium transition-colors">
                            {e.message.split("\n")[0]}
                          </p>
                          <div className="text-muted-foreground mt-1 flex items-center gap-2 text-[11px]">
                            <SourceBadge source={e.source} className="shrink-0 scale-90" />
                            <span className="min-w-0 flex-1 truncate font-mono">
                              {e.culprit || e.service || e.url}
                            </span>
                            <span className="shrink-0">· {relativeTime(e.timestamp)}</span>
                          </div>
                        </div>
                        <ArrowRight className="text-muted-foreground size-3.5 self-center opacity-0 transition-opacity group-hover:opacity-100" />
                      </Link>
                    )
                  })
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between">
                <CardTitle className="text-base">高频问题</CardTitle>
                <Button asChild variant="ghost" size="sm">
                  <Link to="/issues">
                    更多 <ArrowRight className="size-3.5" />
                  </Link>
                </Button>
              </CardHeader>
              <CardContent className="space-y-2">
                {data.topIssues.length === 0 ? (
                  <EmptyState title="暂无问题" description="没有需要关注的高频错误。" className="py-10" />
                ) : (
                  data.topIssues.map((issue) => (
                    <Link
                      key={issue.id}
                      to={`/issues/${issue.id}`}
                      className="hover:bg-muted/60 flex items-center gap-3 rounded-lg px-2 py-2 transition-colors"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium">{issue.title}</p>
                        <div className="text-muted-foreground mt-1 flex items-center gap-1.5 text-[11px]">
                          <Badge variant="outline" className={statusMeta[issue.status]?.className}>
                            {statusMeta[issue.status]?.label ?? issue.status}
                          </Badge>
                          <span>· {relativeTime(issue.lastSeen)}</span>
                        </div>
                      </div>
                      <span className="font-mono text-sm font-semibold tabular-nums">
                        {formatNumber(issue.count)}
                      </span>
                    </Link>
                  ))
                )}
              </CardContent>
            </Card>
          </div>

          <Card className="bg-gradient-to-r from-primary/5 via-transparent to-chart-2/5">
            <CardContent className="flex flex-col items-start justify-between gap-4 py-5 sm:flex-row sm:items-center">
              <div className="flex items-start gap-3">
                <div className="bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg">
                  <Sparkles className="size-4" />
                </div>
                <div>
                  <p className="text-sm font-medium">从数据到行动</p>
                  <p className="text-muted-foreground mt-0.5 text-xs">
                    查看质量健康分、回归问题与可执行的改进建议。
                  </p>
                </div>
              </div>
              <Button asChild variant="outline" size="sm">
                <Link to="/insights">
                  打开持续改进 <ArrowRight className="size-3.5" />
                </Link>
              </Button>
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  )
}

function OverviewSkeleton() {
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-[132px] rounded-xl" />
        ))}
      </div>
      <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
        <Skeleton className="h-[390px] rounded-xl lg:col-span-2" />
        <Skeleton className="h-[390px] rounded-xl" />
      </div>
    </div>
  )
}

