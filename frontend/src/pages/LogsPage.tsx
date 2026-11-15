import * as React from "react"
import { Link } from "react-router-dom"
import { Code2, ExternalLink, RefreshCw, Search, ScrollText } from "lucide-react"

import { api } from "@/lib/api"
import type { EventItem, EventsResponse, Project } from "@/lib/types"
import { useProjects } from "@/context/ProjectContext"
import { firstSourceFrame } from "@/lib/source"
import { useSourceViewer } from "@/context/SourceViewerContext"
import { useDebounce } from "@/hooks/useDebounce"
import { useMediaQuery } from "@/hooks/useMediaQuery"
import { formatFullDate, formatNumber, relativeTime } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { EmptyState } from "@/components/common/EmptyState"
import { ErrorState } from "@/components/common/ErrorState"
import { Pagination } from "@/components/common/Pagination"
import { TimeRangeSelect } from "@/components/common/TimeRangeSelect"
import { LevelMark, SourceBadge } from "@/components/common/Badges"
import { StackTrace } from "@/components/common/StackTrace"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export function LogsPage() {
  const { selectedId, projects } = useProjects()
  const isMobile = useMediaQuery("(max-width: 767px)")
  const pageSize = isMobile ? 20 : 50
  const [source, setSource] = React.useState("all")
  const [level, setLevel] = React.useState("all")
  const [range, setRange] = React.useState("24h")
  const [query, setQuery] = React.useState("")
  const q = useDebounce(query, 350)
  const [page, setPage] = React.useState(1)
  const [auto, setAuto] = React.useState(true)
  const [data, setData] = React.useState<EventsResponse | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [refreshing, setRefreshing] = React.useState(false)
  const [error, setError] = React.useState<unknown>(null)
  const [selected, setSelected] = React.useState<EventItem | null>(null)

  React.useEffect(() => {
    setPage(1)
  }, [source, level, range, q, selectedId])

  const load = React.useCallback(
    async (silent = false) => {
      if (silent) setRefreshing(true)
      else setLoading(true)
      try {
        const res = await api.events({
          projectId: selectedId || undefined,
          kind: "log",
          source: source === "all" ? undefined : source,
          level: level === "all" ? undefined : level,
          q: q || undefined,
          range,
          page,
          pageSize,
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
    [selectedId, source, level, q, range, page, pageSize],
  )

  React.useEffect(() => {
    void load()
  }, [load])

  React.useEffect(() => {
    if (!auto) return
    const id = window.setInterval(() => void load(true), 10_000)
    return () => window.clearInterval(id)
  }, [auto, load])

  return (
    <div className="space-y-6 sm:space-y-8">
      <PageHeader
        title="日志"
        description="统一查看前端与后端日志，点击任意一行查看完整上下文。"
        actions={
          <>
            <div className="text-muted-foreground mr-1 hidden items-center gap-2 text-xs sm:flex">
              <Switch checked={auto} onCheckedChange={setAuto} id="log-auto" />
              <label htmlFor="log-auto" className="cursor-pointer select-none">
                每 10 秒刷新
              </label>
            </div>
            <TimeRangeSelect value={range} onChange={setRange} />
            <Button variant="outline" size="icon" onClick={() => void load(true)} disabled={refreshing} title="刷新">
              <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} />
            </Button>
          </>
        }
      />

      <div className="flex flex-col gap-3">
        <Tabs value={source} onValueChange={setSource}>
          <TabsList>
            <TabsTrigger value="all">全部来源</TabsTrigger>
            <TabsTrigger value="frontend">前端</TabsTrigger>
            <TabsTrigger value="backend">后端</TabsTrigger>
          </TabsList>
        </Tabs>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-full sm:w-80">
            <Search className="text-muted-foreground absolute top-1/2 left-3 size-3.5 -translate-y-1/2" />
            <Input
              placeholder="搜索日志内容…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="h-8 pl-9 text-xs"
            />
          </div>
          <Select value={level} onValueChange={setLevel}>
            <SelectTrigger className="h-8 w-[130px] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">所有级别</SelectItem>
              <SelectItem value="debug">Debug</SelectItem>
              <SelectItem value="info">Info</SelectItem>
              <SelectItem value="warn">Warn</SelectItem>
              <SelectItem value="error">Error</SelectItem>
              <SelectItem value="fatal">Fatal</SelectItem>
            </SelectContent>
          </Select>
          <div className="text-muted-foreground ml-auto flex items-center gap-2 text-xs">
            {auto ? (
              <span className="flex items-center gap-1.5">
                <span className="bg-success size-1.5 animate-pulse rounded-full" /> 实时
              </span>
            ) : null}
            {data ? <span>{formatNumber(data.total)} 条日志</span> : null}
          </div>
        </div>
      </div>

      {error && !data ? <ErrorState error={error} onRetry={() => void load()} /> : null}

      {loading && !data ? (
        <div className="space-y-2">
          {Array.from({ length: 10 }).map((_, i) => (
            <Skeleton key={i} className="h-10 rounded-md" />
          ))}
        </div>
      ) : data ? (
        <>
          {data.events.length === 0 ? (
            <EmptyState
              icon={<ScrollText className="size-5" />}
              title="暂无日志"
              description="尝试扩大时间范围、切换来源，或通过 SDK 接入后发送测试事件。"
            />
          ) : (
            <>
              {/* Mobile: readable cards instead of a horizontally scrolling table */}
              <div className="space-y-2 md:hidden">
                {data.events.map((e) => (
                  <button
                    key={e.id}
                    type="button"
                    onClick={() => setSelected(e)}
                    className="bg-card w-full rounded-2xl p-3.5 text-left shadow-sm transition-shadow active:shadow-md"
                  >
                    <div className="flex items-center gap-2">
                      <LevelMark level={e.level} />
                      <SourceBadge source={e.source} />
                      <span className="text-muted-foreground ml-auto text-[10px]">
                        {relativeTime(e.timestamp)}
                      </span>
                    </div>
                    <p className="mt-2 line-clamp-2 text-sm leading-snug">
                      {e.message}
                    </p>
                    <div className="text-muted-foreground mt-1.5 flex items-center gap-2 text-[10px]">
                      {e.service ? <span className="font-mono">{e.service}</span> : null}
                      {e.request?.status && e.request.status >= 400 ? (
                        <span className="text-destructive font-mono">HTTP {e.request.status}</span>
                      ) : null}
                      {e.culprit ? (
                        <span className="min-w-0 truncate font-mono">{e.culprit}</span>
                      ) : null}
                    </div>
                  </button>
                ))}
              </div>

              <Card className="hidden gap-0 overflow-hidden py-0 md:block">
                <Table>
                  <TableHeader>
                    <TableRow className="hover:bg-transparent">
                      <TableHead className="w-[92px]">时间</TableHead>
                      <TableHead className="w-[80px]">级别</TableHead>
                      <TableHead className="w-[70px]">来源</TableHead>
                      <TableHead>消息</TableHead>
                      <TableHead className="hidden w-[110px] lg:table-cell">服务</TableHead>
                      <TableHead className="hidden w-[120px] xl:table-cell">环境</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data.events.map((e) => (
                      <TableRow
                        key={e.id}
                        className="cursor-pointer"
                        onClick={() => setSelected(e)}
                      >
                        <TableCell className="text-muted-foreground text-[11px] tabular-nums">
                          {relativeTime(e.timestamp)}
                        </TableCell>
                        <TableCell>
                          <LevelMark level={e.level} />
                        </TableCell>
                        <TableCell>
                          <SourceBadge source={e.source} />
                        </TableCell>
                        <TableCell className="max-w-[560px]">
                          <div className="flex items-center gap-2">
                            <span className="truncate text-xs">{e.message}</span>
                            {e.request?.status && e.request.status >= 400 ? (
                              <Badge variant="outline" className="border-destructive/30 text-destructive shrink-0 font-mono text-[10px]">
                                {e.request.status}
                              </Badge>
                            ) : null}
                          </div>
                          {e.culprit || e.context ? (
                            <p className="text-muted-foreground mt-0.5 truncate text-[10px]">
                              {e.culprit || JSON.stringify(e.context).slice(0, 160)}
                            </p>
                          ) : null}
                        </TableCell>
                        <TableCell className="text-muted-foreground hidden font-mono text-[11px] lg:table-cell">
                          {e.service || "—"}
                        </TableCell>
                        <TableCell className="text-muted-foreground hidden text-[11px] xl:table-cell">
                          {e.environment || "—"}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            </>
          )}

          {data.total > data.pageSize ? (
            <Pagination
              page={data.page}
              pageSize={data.pageSize}
              total={data.total}
              onChange={setPage}
            />
          ) : null}
        </>
      ) : null}

      <EventDetailDialog event={selected} projects={projects} onClose={() => setSelected(null)} />
    </div>
  )
}

function EventDetailDialog({
  event,
  projects,
  onClose,
}: {
  event: EventItem | null
  projects: Project[]
  onClose: () => void
}) {
  const { openSource } = useSourceViewer()
  const project = event?.projectId ? projects.find((p) => p.id === event.projectId) ?? null : null
  const sourceFrame = firstSourceFrame(project, event?.release, [event?.stack, event?.culprit])
  return (
    <Dialog open={Boolean(event)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-3xl">
        {event ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex flex-wrap items-center gap-2 pr-6 text-base">
                <LevelMark level={event.level} />
                <SourceBadge source={event.source} />
                <span className="text-muted-foreground font-mono text-xs">{event.id.slice(-10)}</span>
              </DialogTitle>
              <DialogDescription className="break-all">
                {formatFullDate(event.timestamp)} · {event.service || "unknown"} · {event.environment || "unknown"}
              </DialogDescription>
              {sourceFrame ? (
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-1 w-fit"
                  onClick={() => openSource(sourceFrame)}
                >
                  <Code2 className="size-3.5" /> 查看源码 <ExternalLink className="size-3" />
                </Button>
              ) : null}
            </DialogHeader>

            <div className="space-y-4 text-sm">
              <pre className="bg-muted/40 scrollbar-thin max-h-56 overflow-auto rounded-xl p-3 text-xs whitespace-pre-wrap">
                {event.message}
              </pre>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <Info label="用户">
                  {event.user?.email || event.user?.id || event.user?.username || "—"}
                </Info>
                <Info label="URL">{event.url || "—"}</Info>
                <Info label="版本">{event.release || "—"}</Info>
                <Info label="平台">{event.platform || "—"}</Info>
                <Info label="请求">
                  {event.request ? `${event.request.method ?? ""} ${event.request.url ?? ""}` : "—"}
                </Info>
                <Info label="Request ID">{event.request?.requestId || "—"}</Info>
              </div>

              {event.tags && Object.keys(event.tags).length > 0 ? (
                <div>
                  <p className="text-muted-foreground mb-1.5 text-xs font-medium">标签</p>
                  <div className="flex flex-wrap gap-1.5">
                    {Object.entries(event.tags).map(([k, v]) => (
                      <span key={k} className="bg-muted rounded px-2 py-1 font-mono text-[11px]">
                        <span className="text-muted-foreground">{k}=</span>
                        {v}
                      </span>
                    ))}
                  </div>
                </div>
              ) : null}

              {event.context && Object.keys(event.context).length > 0 ? (
                <div>
                  <p className="text-muted-foreground mb-1.5 text-xs font-medium">Context</p>
                  <pre className="bg-muted/40 scrollbar-thin max-h-52 overflow-auto rounded-xl p-3 text-[11px]">
                    {JSON.stringify(event.context, null, 2)}
                  </pre>
                </div>
              ) : null}

              {event.stack ? (
                <div>
                  <p className="text-muted-foreground mb-1.5 text-xs font-medium">堆栈</p>
                  <StackTrace stack={event.stack} project={project} release={event.release} sourceMap={event.sourceMap} />
                </div>
              ) : null}

              {event.breadcrumbs?.length ? (
                <div>
                  <p className="text-muted-foreground mb-1.5 text-xs font-medium">面包屑</p>
                  <div className="space-y-1.5">
                    {event.breadcrumbs.map((b, i) => (
                      <div key={i} className="bg-muted/30 flex items-center gap-2 rounded-lg px-2 py-1.5 text-[11px]">
                        <span className="text-muted-foreground w-16 shrink-0 font-mono">
                          {new Date(b.timestamp).toLocaleTimeString("zh-CN", { hour12: false })}
                        </span>
                        <Badge variant="outline" className="shrink-0 text-[10px]">
                          {b.category}
                        </Badge>
                        <span className="truncate">{b.message}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}

              {event.issueId && !event.issueId.startsWith("000000") ? (
                <Button asChild variant="outline" size="sm">
                  <Link to={`/issues/${event.issueId}`}>查看关联问题</Link>
                </Button>
              ) : null}
            </div>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function Info({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="bg-muted/30 rounded-xl px-3 py-2">
      <p className="text-muted-foreground text-[10px]">{label}</p>
      <p className="mt-0.5 truncate font-mono text-xs" title={typeof children === "string" ? children : undefined}>
        {children}
      </p>
    </div>
  )
}
