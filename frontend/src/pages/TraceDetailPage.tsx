import * as React from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { ArrowLeft, Clock, GitBranch, RefreshCw, Waypoints } from "lucide-react"

import { api } from "@/lib/api"
import type { TraceDetailResponse, TraceSpan } from "@/lib/types"
import { cn, formatDuration, formatFullDate, plural, relativeTime } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { CopyButton } from "@/components/common/CopyButton"
import { ErrorState } from "@/components/common/ErrorState"
import { EmptyState } from "@/components/common/EmptyState"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

export function TraceDetailPage() {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const [data, setData] = React.useState<TraceDetailResponse | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<unknown>(null)
  const [selectedSpanId, setSelectedSpanId] = React.useState("")

  const load = React.useCallback(async () => {
    setLoading(true)
    try {
      const res = await api.trace(id)
      setData(res)
      setSelectedSpanId((prev) => {
        if (prev && res.spans.some((s) => s.id === prev)) return prev
        return res.spans.find((s) => s.status === "error")?.id ?? res.spans[0]?.id ?? ""
      })
      setError(null)
    } catch (err) {
      setError(err)
    } finally {
      setLoading(false)
    }
  }, [id])

  React.useEffect(() => {
    void load()
  }, [load])

  if (loading && !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-24 rounded-xl" />
        <div className="grid gap-4 lg:grid-cols-3">
          <Skeleton className="h-[420px] rounded-xl lg:col-span-2" />
          <Skeleton className="h-[420px] rounded-xl" />
        </div>
      </div>
    )
  }

  if (error && !data) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" size="sm" onClick={() => navigate("/traces")}>
          <ArrowLeft className="size-4" /> 返回 Trace 列表
        </Button>
        <ErrorState error={error} onRetry={() => void load()} />
      </div>
    )
  }
  if (!data) return null

  const { trace, spans } = data
  const selected = spans.find((s) => s.id === selectedSpanId) ?? null
  const waterfall = buildWaterfall(spans)

  return (
    <div className="space-y-6 sm:space-y-8">
      <div className="flex flex-wrap items-center gap-2">
        <Button asChild variant="ghost" size="sm">
          <Link to="/traces">
            <ArrowLeft className="size-4" /> Trace 列表
          </Link>
        </Button>
        <span className="text-muted-foreground font-mono text-xs">{trace.traceId}</span>
        <CopyButton value={trace.traceId} label="复制 Trace ID" />
        <Button variant="ghost" size="icon-sm" className="ml-auto" onClick={() => void load()} title="刷新">
          <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
        </Button>
      </div>

      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-2">
            <span
              className={cn(
                "size-3 shrink-0 rounded-full",
                trace.errorCount > 0 ? "bg-destructive" : "bg-success",
              )}
            />
            {trace.rootName || "unnamed trace"}
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-2">
            <GitBranch className="size-3.5" />
            {trace.service || "unknown service"}
            <span>·</span>
            <span>{formatFullDate(trace.startTime)}</span>
            <span>·</span>
            <span>{relativeTime(trace.startTime)}</span>
          </span>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <Badge variant="outline" className="font-mono">
              {formatDuration(trace.durationMs)}
            </Badge>
            <Badge variant="outline">{plural(trace.spanCount, "span")}</Badge>
            {trace.errorCount > 0 ? (
              <Badge variant="outline" className="border-destructive/30 text-destructive">
                {plural(trace.errorCount, "error")}
              </Badge>
            ) : null}
          </div>
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="flex items-center gap-2 text-base">
              <Waypoints className="size-4" /> 调用链瀑布图
            </CardTitle>
            <span className="text-muted-foreground text-[11px]">
              点击 span 查看属性与事件
            </span>
          </CardHeader>
          <CardContent>
            {waterfall.length === 0 ? (
              <EmptyState title="没有 span" description="该 Trace 没有存储到 span 数据。" />
            ) : (
              <div className="space-y-1.5">
                {waterfall.map((item) => (
                  <button
                    key={item.span.id}
                    type="button"
                    onClick={() => setSelectedSpanId(item.span.id)}
                    className={cn(
                      "hover:bg-muted/50 w-full rounded-lg px-2.5 py-2 text-left transition-colors",
                      selectedSpanId === item.span.id && "bg-primary/5",
                    )}
                  >
                    <div className="flex items-start gap-2">
                      <span
                        className={cn(
                          "mt-1.5 size-2 shrink-0 rounded-full",
                          item.span.status === "error" ? "bg-destructive" : "bg-chart-2",
                        )}
                      />
                      <div
                        className="min-w-0 flex-1"
                        style={{ paddingLeft: `${Math.min(item.depth, 12) * 12}px` }}
                      >
                        <div className="flex items-center gap-2">
                          <span className="truncate text-xs font-medium">{item.span.name || "unnamed span"}</span>
                          {item.span.service ? (
                            <span className="text-muted-foreground hidden shrink-0 font-mono text-[10px] sm:inline">
                              {item.span.service}
                            </span>
                          ) : null}
                        </div>
                        <div className="text-muted-foreground mt-0.5 flex items-center gap-2 font-mono text-[10px]">
                          <span>{item.span.kind || "span"}</span>
                          <span>·</span>
                          <span>{item.span.spanId}</span>
                          <span className="hidden sm:inline">· {formatDuration(item.span.durationMs)}</span>
                        </div>
                      </div>
                      <span className="shrink-0 font-mono text-[10px] tabular-nums">
                        {formatDuration(item.span.durationMs)}
                      </span>
                    </div>
                    <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted">
                      <div
                        className={cn(
                          "h-full rounded-full",
                          item.span.status === "error" ? "bg-destructive" : "bg-primary",
                        )}
                        style={{ marginLeft: `${item.offset}%`, width: `${item.width}%` }}
                      />
                    </div>
                  </button>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="self-start lg:sticky lg:top-24">
          <CardHeader>
            <CardTitle className="text-base">Span 详情</CardTitle>
          </CardHeader>
          <CardContent>
            {selected ? (
              <SpanDetail span={selected} />
            ) : (
              <p className="text-muted-foreground py-8 text-center text-sm">选择一个 span</p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function SpanDetail({ span }: { span: TraceSpan }) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <p className="break-words text-sm font-medium">{span.name}</p>
        <div className="text-muted-foreground flex flex-wrap gap-2 text-[11px]">
          <Badge variant="outline" className="font-mono text-[10px]">
            {span.status}
          </Badge>
          <span className="font-mono">{span.service || "unknown"}</span>
          <span className="flex items-center gap-1">
            <Clock className="size-3" /> {formatDuration(span.durationMs)}
          </span>
        </div>
        <p className="text-muted-foreground break-all font-mono text-[10px]">
          trace {span.traceId} · span {span.spanId}
          {span.parentSpanId ? ` · parent ${span.parentSpanId}` : ""}
        </p>
        <p className="text-muted-foreground text-[10px]">
          {formatFullDate(span.startTime)} → {formatFullDate(span.endTime)}
        </p>
        {span.statusMessage ? (
          <p className="bg-destructive/5 text-destructive rounded-lg px-2.5 py-2 text-xs">
            {span.statusMessage}
          </p>
        ) : null}
      </div>

      <Tabs defaultValue="attributes">
        <TabsList className="w-full">
          <TabsTrigger value="attributes">属性</TabsTrigger>
          <TabsTrigger value="events">事件 ({span.events?.length ?? 0})</TabsTrigger>
          <TabsTrigger value="resource">资源</TabsTrigger>
        </TabsList>
        <TabsContent value="attributes" className="pt-3">
          <JsonBlock value={span.attributes} />
        </TabsContent>
        <TabsContent value="events" className="space-y-2 pt-3">
          {span.events?.length ? (
            span.events.map((event, i) => (
              <div key={i} className="bg-muted/30 rounded-lg p-2.5">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-medium">{event.name}</span>
                  <span className="text-muted-foreground text-[10px]">{relativeTime(event.time)}</span>
                </div>
                <JsonBlock value={event.attributes} className="mt-2" />
              </div>
            ))
          ) : (
            <p className="text-muted-foreground text-xs">没有 span 事件</p>
          )}
        </TabsContent>
        <TabsContent value="resource" className="pt-3">
          <JsonBlock value={span.resource} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function JsonBlock({ value, className }: { value?: Record<string, unknown>; className?: string }) {
  if (!value || Object.keys(value).length === 0) {
    return <p className="text-muted-foreground text-xs">空</p>
  }
  return (
    <pre className={cn("bg-muted/40 scrollbar-thin max-h-64 overflow-auto rounded-lg p-2.5 text-[10px]", className)}>
      {JSON.stringify(value, null, 2)}
    </pre>
  )
}

function buildWaterfall(spans: TraceSpan[]) {
  if (spans.length === 0) return []
  const start = Math.min(...spans.map((s) => new Date(s.startTime).getTime()))
  const end = Math.max(...spans.map((s) => new Date(s.endTime).getTime()))
  const total = Math.max(1, end - start)
  const byId = new Map(spans.map((s) => [s.spanId, s]))
  const depthOf = (span: TraceSpan): number => {
    let depth = 0
    let current: TraceSpan | undefined = span
    const seen = new Set<string>()
    while (current?.parentSpanId && byId.has(current.parentSpanId) && depth < 20 && !seen.has(current.parentSpanId)) {
      seen.add(current.parentSpanId)
      current = byId.get(current.parentSpanId)
      depth++
    }
    return depth
  }
  return [...spans]
    .sort((a, b) => {
      const at = new Date(a.startTime).getTime()
      const bt = new Date(b.startTime).getTime()
      if (at !== bt) return at - bt
      return b.durationMs - a.durationMs
    })
    .map((span) => {
      const spanStart = new Date(span.startTime).getTime()
      const offset = Math.max(0, Math.min(99.4, ((spanStart - start) / total) * 100))
      const width = Math.min(100 - offset, Math.max(0.6, (span.durationMs / total) * 100))
      return { span, depth: depthOf(span), offset, width }
    })
}
