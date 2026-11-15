import * as React from "react"
import { useNavigate } from "react-router-dom"
import { GitBranch, RefreshCw, Search, Waypoints } from "lucide-react"

import { api } from "@/lib/api"
import type { TraceListResponse } from "@/lib/types"
import { useProjects } from "@/context/ProjectContext"
import { useDebounce } from "@/hooks/useDebounce"
import { formatDuration, formatNumber, plural, relativeTime } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { EmptyState } from "@/components/common/EmptyState"
import { ErrorState } from "@/components/common/ErrorState"
import { Pagination } from "@/components/common/Pagination"
import { TimeRangeSelect } from "@/components/common/TimeRangeSelect"
import { CopyButton } from "@/components/common/CopyButton"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export function TracesPage() {
  const navigate = useNavigate()
  const { selectedId } = useProjects()
  const [range, setRange] = React.useState("1h")
  const [status, setStatus] = React.useState("all")
  const [service, setService] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const q = useDebounce(query, 350)
  const [page, setPage] = React.useState(1)
  const [data, setData] = React.useState<TraceListResponse | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [refreshing, setRefreshing] = React.useState(false)
  const [error, setError] = React.useState<unknown>(null)

  React.useEffect(() => {
    setPage(1)
  }, [range, status, service, q, selectedId])

  const load = React.useCallback(
    async (silent = false) => {
      if (silent) setRefreshing(true)
      else setLoading(true)
      try {
        const res = await api.traces({
          projectId: selectedId || undefined,
          range,
          status,
          service: service === "all" ? undefined : service,
          q: q || undefined,
          page,
          pageSize: 25,
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
    [selectedId, range, status, service, q, page],
  )

  React.useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="space-y-6 sm:space-y-8">
      <PageHeader
        title="Trace"
        description="分布式调用链瀑布图；错误 span 会同时进入问题列表。"
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
            placeholder="搜索 traceId、span 或服务…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="h-8 pl-9 text-xs"
          />
        </div>
        <Select value={service} onValueChange={setService}>
          <SelectTrigger className="h-8 w-[160px] text-xs">
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
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger className="h-8 w-[130px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            <SelectItem value="error">含错误</SelectItem>
            <SelectItem value="ok">无错误</SelectItem>
          </SelectContent>
        </Select>
        <span className="text-muted-foreground ml-auto text-xs">
          {data ? `${formatNumber(data.total)} 条 Trace` : ""}
        </span>
      </div>

      {error && !data ? <ErrorState error={error} onRetry={() => void load()} /> : null}

      {loading && !data ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-[76px] rounded-lg" />
          ))}
        </div>
      ) : data ? (
        <>
          {data.traces.length === 0 ? (
            <EmptyState
              icon={<Waypoints className="size-5" />}
              title="暂无 Trace"
              description="接入 OTLP traces 后，这里会展示完整调用链。请确认 service.version 与上报数据一致。"
            />
          ) : (
            <Card className="gap-0 py-2">
              <div className="space-y-0.5 px-2">
                {data.traces.map((trace) => (
                  <div
                    key={trace.traceId}
                    role="button"
                    tabIndex={0}
                    onClick={() => navigate(`/traces/${trace.traceId}`)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault()
                        navigate(`/traces/${trace.traceId}`)
                      }
                    }}
                    className="hover:bg-muted/40 group flex cursor-pointer items-start gap-3 rounded-lg px-3 py-3.5 transition-colors"
                  >
                    <span
                      className={`mt-1.5 size-2.5 shrink-0 rounded-full ${
                        trace.errorCount > 0 ? "bg-destructive" : "bg-success"
                      }`}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="group-hover:text-primary truncate text-sm font-medium transition-colors">
                          {trace.rootName || "unnamed trace"}
                        </span>
                        {trace.errorCount > 0 ? (
                          <Badge variant="outline" className="border-destructive/30 text-destructive shrink-0 text-[10px]">
                            {plural(trace.errorCount, "error")}
                          </Badge>
                        ) : null}
                      </div>
                      <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-2 text-[11px]">
                        <span className="font-mono">{trace.traceId}</span>
                        <span onClick={(e) => e.stopPropagation()}>
                          <CopyButton value={trace.traceId} label="复制 Trace ID" />
                        </span>
                        {trace.service ? (
                          <span className="flex items-center gap-1">
                            <GitBranch className="size-3" /> {trace.service}
                          </span>
                        ) : null}
                        <span>· {relativeTime(trace.startTime)}</span>
                      </div>
                    </div>
                    <div className="shrink-0 text-right">
                      <p className="text-sm font-semibold tabular-nums">{formatDuration(trace.durationMs)}</p>
                      <p className="text-muted-foreground text-[10px]">{plural(trace.spanCount, "span")}</p>
                    </div>
                  </div>
                ))}
              </div>
            </Card>
          )}
          {data.total > data.pageSize ? (
            <Pagination page={data.page} pageSize={data.pageSize} total={data.total} onChange={setPage} />
          ) : null}
        </>
      ) : null}
    </div>
  )
}
