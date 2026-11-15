import * as React from "react"
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"
import { Gauge, RefreshCw, Search } from "lucide-react"

import { api } from "@/lib/api"
import type { MetricPoint, MetricSeriesResponse, MetricSummary, MetricsListResponse } from "@/lib/types"
import { useProjects } from "@/context/ProjectContext"
import { useDebounce } from "@/hooks/useDebounce"
import { formatFullDate, formatMetricValue, formatNumber, plural, relativeTime } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { EmptyState } from "@/components/common/EmptyState"
import { ErrorState } from "@/components/common/ErrorState"
import { TimeRangeSelect } from "@/components/common/TimeRangeSelect"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

export function MetricsPage() {
  const { selectedId } = useProjects()
  const [range, setRange] = React.useState("24h")
  const [service, setService] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const q = useDebounce(query, 350)
  const [data, setData] = React.useState<MetricsListResponse | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [refreshing, setRefreshing] = React.useState(false)
  const [error, setError] = React.useState<unknown>(null)
  const [selected, setSelected] = React.useState<MetricSummary | null>(null)

  const load = React.useCallback(
    async (silent = false) => {
      if (silent) setRefreshing(true)
      else setLoading(true)
      try {
        const res = await api.metrics({
          projectId: selectedId || undefined,
          range,
          service: service === "all" ? undefined : service,
          q: q || undefined,
        })
        setData(res)
        setError(null)
      } catch (err) {
        setError(err)
      } finally {
        setLoading(false)
        setRefreshing(false)
      }
    },
    [selectedId, range, service, q],
  )

  React.useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="space-y-6 sm:space-y-8">
      <PageHeader
        title="指标"
        description="来自 OTLP Metrics 的最新值与时间序列。"
        actions={
          <>
            <TimeRangeSelect value={range} onChange={setRange} />
            <Button variant="outline" size="icon" onClick={() => void load(true)} disabled={refreshing} title="刷新">
              <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} />
            </Button>
          </>
        }
      />

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full sm:w-72">
          <Search className="text-muted-foreground absolute top-1/2 left-3 size-3.5 -translate-y-1/2" />
          <Input
            placeholder="搜索指标名称…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="h-8 pl-9 text-xs"
          />
        </div>
        <Select value={service} onValueChange={setService}>
          <SelectTrigger className="h-8 w-[170px] text-xs">
            <SelectValue placeholder="所有服务" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">所有服务</SelectItem>
            {(data?.services ?? []).map((s) => (
              <SelectItem key={s} value={s}>
                {s}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-muted-foreground ml-auto text-xs">
          {data ? `${formatNumber(data.metrics.length)} 个指标` : ""}
        </span>
      </div>

      {error && !data ? <ErrorState error={error} onRetry={() => void load()} /> : null}

      {loading && !data ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-36 rounded-lg" />
          ))}
        </div>
      ) : data ? (
        data.metrics.length === 0 ? (
          <EmptyState
            icon={<Gauge className="size-5" />}
            title="暂无指标"
            description="通过 OTLP /v1/metrics 上报后，这里会显示 gauge、sum、histogram 和 summary。"
          />
        ) : (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
            {data.metrics.map((metric) => (
              <button
                key={`${metric.name}:${metric.service}`}
                type="button"
                onClick={() => setSelected(metric)}
                className="bg-card hover:shadow-md focus-visible:ring-ring/50 rounded-xl p-4 text-left shadow-sm transition-shadow focus-visible:ring-2 focus-visible:outline-none"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate font-mono text-xs font-medium">{metric.name}</p>
                    {metric.description ? (
                      <p className="text-muted-foreground mt-0.5 line-clamp-1 text-[10px]">{metric.description}</p>
                    ) : null}
                  </div>
                  <Badge variant="outline" className="shrink-0 text-[9px] uppercase">
                    {metric.type.replace("_", " ")}
                  </Badge>
                </div>
                <div className="mt-4 flex items-end justify-between gap-2">
                  <span className="text-2xl font-semibold tracking-tight tabular-nums">
                    {formatMetricValue(metric.latest, metric.unit)}
                  </span>
                  <span className="text-muted-foreground text-[10px]">{relativeTime(metric.lastSeen)}</span>
                </div>
                <div className="text-muted-foreground mt-2 flex items-center justify-between gap-2 text-[10px]">
                  <span className="truncate font-mono">{metric.service || "—"}</span>
                  <span>{plural(metric.pointCount, "point")}</span>
                </div>
              </button>
            ))}
          </div>
        )
      ) : null}

      <MetricDialog
        metric={selected}
        range={range}
        projectId={selectedId || undefined}
        onClose={() => setSelected(null)}
      />
    </div>
  )
}

function MetricDialog({
  metric,
  range,
  projectId,
  onClose,
}: {
  metric: MetricSummary | null
  range: string
  projectId?: string
  onClose: () => void
}) {
  const [data, setData] = React.useState<MetricSeriesResponse | null>(null)
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [localRange, setLocalRange] = React.useState(range)

  React.useEffect(() => {
    setLocalRange(range)
  }, [range, metric])

  React.useEffect(() => {
    if (!metric) return
    let active = true
    setLoading(true)
    setError(null)
    api
      .metricSeries({
        projectId,
        name: metric.name,
        service: metric.service || undefined,
        range: localRange,
      })
      .then((res) => {
        if (active) setData(res)
      })
      .catch((err) => {
        if (active) setError(err instanceof Error ? err.message : "无法加载指标序列")
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [metric, projectId, localRange])

  const chartData = React.useMemo(
    () =>
      (data?.points ?? []).map((point) => ({
        ...point,
        time: new Date(point.timestamp).getTime(),
      })),
    [data],
  )
  const latest = chartData[chartData.length - 1] as (MetricPoint & { time: number }) | undefined
  const first = chartData[0] as (MetricPoint & { time: number }) | undefined

  return (
    <Dialog open={Boolean(metric)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        {metric ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex flex-wrap items-center gap-2 pr-8 font-mono text-base">
                {metric.name}
                <Badge variant="outline" className="text-[10px] uppercase">
                  {metric.type.replace("_", " ")}
                </Badge>
              </DialogTitle>
              <DialogDescription className="flex flex-wrap items-center gap-2">
                <span>{metric.service || "unknown service"}</span>
                {metric.unit ? <span>· {metric.unit}</span> : null}
                <span>· {plural(metric.pointCount, "point")}</span>
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-center justify-between gap-2">
              <div className="text-2xl font-semibold tabular-nums">
                {formatMetricValue(metric.latest, metric.unit)}
              </div>
              <TimeRangeSelect value={localRange} onChange={setLocalRange} />
            </div>

            <div className="h-[280px]">
              {loading ? (
                <Skeleton className="h-full w-full rounded-lg" />
              ) : error ? (
                <div className="text-destructive flex h-full items-center justify-center text-sm">{error}</div>
              ) : chartData.length === 0 ? (
                <div className="text-muted-foreground flex h-full items-center justify-center text-sm">
                  所选范围内没有数据点
                </div>
              ) : (
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={chartData} margin={{ top: 8, right: 12, left: -18, bottom: 0 }}>
                    <CartesianGrid stroke="var(--border)" strokeDasharray="3 4" vertical={false} />
                    <XAxis
                      dataKey="time"
                      type="number"
                      scale="time"
                      domain={["dataMin", "dataMax"]}
                      tickFormatter={(v) =>
                        new Date(v).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false })
                      }
                      tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                      axisLine={false}
                      tickLine={false}
                      minTickGap={30}
                    />
                    <YAxis
                      tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                      axisLine={false}
                      tickLine={false}
                    />
                    <Tooltip content={<MetricTooltip unit={metric.unit} />} />
                    <Line
                      type="monotone"
                      dataKey="value"
                      name={metric.name}
                      stroke="var(--chart-1)"
                      strokeWidth={2}
                      dot={chartData.length < 40}
                      activeDot={{ r: 4 }}
                      isAnimationActive={false}
                    />
                  </LineChart>
                </ResponsiveContainer>
              )}
            </div>

            <div className="grid grid-cols-2 gap-3 text-xs sm:grid-cols-4">
              <MetricStat label="当前" value={latest ? formatMetricValue(latest.value, metric.unit) : "—"} />
              <MetricStat label="最早" value={first ? formatMetricValue(first.value, metric.unit) : "—"} />
              <MetricStat label="数据点" value={formatNumber(chartData.length)} />
              <MetricStat
                label="最近"
                value={latest ? relativeTime(new Date(latest.time).toISOString()) : "—"}
              />
            </div>

            <Tabs defaultValue="series">
              <TabsList>
                <TabsTrigger value="series">数据点</TabsTrigger>
                <TabsTrigger value="attributes">属性</TabsTrigger>
              </TabsList>
              <TabsContent value="series" className="pt-3">
                <div className="scrollbar-thin max-h-48 overflow-auto rounded-lg border border-border/40">
                  <table className="w-full text-[11px]">
                    <thead className="bg-muted/40 text-muted-foreground sticky top-0">
                      <tr>
                        <th className="px-2 py-1.5 text-left">时间</th>
                        <th className="px-2 py-1.5 text-right">值</th>
                        <th className="px-2 py-1.5 text-right">count</th>
                      </tr>
                    </thead>
                    <tbody>
                      {[...chartData].reverse().slice(0, 100).map((point) => (
                        <tr key={point.id} className="border-t border-border/30">
                          <td className="px-2 py-1 font-mono">{formatFullDate(point.timestamp)}</td>
                          <td className="px-2 py-1 text-right font-mono">{formatMetricValue(point.value)}</td>
                          <td className="px-2 py-1 text-right font-mono">{point.count ?? "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </TabsContent>
              <TabsContent value="attributes" className="pt-3">
                <pre className="bg-muted/40 max-h-52 overflow-auto rounded-lg p-3 text-[11px]">
                  {JSON.stringify(metric.attributes ?? {}, null, 2)}
                </pre>
              </TabsContent>
            </Tabs>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

interface TooltipPayload {
  value?: number
  dataKey?: string
  payload?: MetricPoint & { time: number }
}

function MetricTooltip({
  active,
  payload,
  unit,
}: {
  active?: boolean
  payload?: TooltipPayload[]
  unit?: string
}) {
  if (!active || !payload?.length) return null
  const point = payload[0]?.payload
  return (
    <div className="bg-popover text-popover-foreground rounded-lg border px-3 py-2 text-xs shadow-lg">
      <p className="text-muted-foreground mb-1">{point ? formatFullDate(point.timestamp) : ""}</p>
      <p className="font-mono font-medium">{formatMetricValue(payload[0]?.value ?? 0, unit)}</p>
    </div>
  )
}

function MetricStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-muted/30 rounded-lg p-2.5">
      <p className="text-muted-foreground text-[10px]">{label}</p>
      <p className="mt-0.5 truncate font-mono text-xs font-medium">{value}</p>
    </div>
  )
}
