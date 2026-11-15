import * as React from "react"
import { Link, useNavigate } from "react-router-dom"
import { Bug, RefreshCw, Search, SlidersHorizontal, UserRound } from "lucide-react"

import { api } from "@/lib/api"
import type { Issue, IssuesResponse } from "@/lib/types"
import { useProjects } from "@/context/ProjectContext"
import { useDebounce } from "@/hooks/useDebounce"
import { formatNumber, relativeTime } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { LevelBadge, SourceBadge, StatusBadge } from "@/components/common/Badges"
import { EmptyState } from "@/components/common/EmptyState"
import { ErrorState } from "@/components/common/ErrorState"
import { Pagination } from "@/components/common/Pagination"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export function IssuesPage() {
  const navigate = useNavigate()
  const { selectedId } = useProjects()
  const [status, setStatus] = React.useState("open")
  const [level, setLevel] = React.useState("all")
  const [source, setSource] = React.useState("all")
  const [sort, setSort] = React.useState("lastSeen")
  const [query, setQuery] = React.useState("")
  const q = useDebounce(query, 350)
  const [page, setPage] = React.useState(1)
  const [data, setData] = React.useState<IssuesResponse | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [refreshing, setRefreshing] = React.useState(false)
  const [error, setError] = React.useState<unknown>(null)

  React.useEffect(() => {
    setPage(1)
  }, [status, level, source, sort, q, selectedId])

  const load = React.useCallback(
    async (silent = false) => {
      if (silent) setRefreshing(true)
      else setLoading(true)
      try {
        const res = await api.issues({
          projectId: selectedId || undefined,
          status,
          level: level === "all" ? undefined : level,
          source: source === "all" ? undefined : source,
          q: q || undefined,
          sort,
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
    [selectedId, status, level, source, q, sort, page],
  )

  React.useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="space-y-6 sm:space-y-8">
      <PageHeader
        title="问题"
        description="按指纹自动归并的错误分组，跟踪状态、回归和负责人。"
        actions={
          <Button variant="outline" onClick={() => void load(true)} disabled={refreshing}>
            <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} /> 刷新
          </Button>
        }
      />

      <div className="flex flex-col gap-3">
        <Tabs value={status} onValueChange={setStatus}>
          <TabsList>
            <TabsTrigger value="open">待处理</TabsTrigger>
            <TabsTrigger value="resolved">已解决</TabsTrigger>
            <TabsTrigger value="ignored">已忽略</TabsTrigger>
            <TabsTrigger value="all">全部</TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-full sm:w-72">
            <Search className="text-muted-foreground absolute top-1/2 left-3 size-3.5 -translate-y-1/2" />
            <Input
              placeholder="搜索标题、位置或指纹…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="h-8 pl-9 text-xs"
            />
          </div>
          <Select value={level} onValueChange={setLevel}>
            <SelectTrigger className="h-8 w-[130px] text-xs">
              <SlidersHorizontal className="size-3.5" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">所有级别</SelectItem>
              <SelectItem value="fatal">Fatal</SelectItem>
              <SelectItem value="error">Error</SelectItem>
              <SelectItem value="warn">Warn</SelectItem>
              <SelectItem value="info">Info</SelectItem>
            </SelectContent>
          </Select>
          <Select value={source} onValueChange={setSource}>
            <SelectTrigger className="h-8 w-[130px] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">所有来源</SelectItem>
              <SelectItem value="frontend">前端</SelectItem>
              <SelectItem value="backend">后端</SelectItem>
            </SelectContent>
          </Select>
          <Select value={sort} onValueChange={setSort}>
            <SelectTrigger className="h-8 w-[150px] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="lastSeen">最近发生</SelectItem>
              <SelectItem value="count">发生次数</SelectItem>
              <SelectItem value="firstSeen">最早发现</SelectItem>
            </SelectContent>
          </Select>
          <span className="text-muted-foreground ml-auto text-xs">
            {data ? `${formatNumber(data.total)} 个问题` : ""}
          </span>
        </div>
      </div>

      {error && !data ? <ErrorState error={error} onRetry={() => void load()} /> : null}

      {loading && !data ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-[70px] rounded-lg" />
          ))}
        </div>
      ) : data ? (
        <>
          {data.issues.length === 0 ? (
            <EmptyState
              icon={<Bug className="size-5" />}
              title="没有匹配的问题"
              description={
                status === "open"
                  ? "当前筛选条件下没有待处理问题，质量状态看起来不错。"
                  : "尝试调整状态、级别或搜索条件。"
              }
            />
          ) : (
            <Card className="gap-0 overflow-hidden py-2">
              <div className="space-y-0.5 px-2">
                {data.issues.map((issue) => (
                  <IssueRow key={issue.id} issue={issue} onOpen={() => navigate(`/issues/${issue.id}`)} />
                ))}
              </div>
            </Card>
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
    </div>
  )
}

function IssueRow({ issue, onOpen }: { issue: Issue; onOpen: () => void }) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault()
          onOpen()
        }
      }}
      className="group hover:bg-muted/40 flex cursor-pointer items-start gap-3 rounded-xl px-3 py-3.5 transition-colors sm:items-center"
    >
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <LevelBadge level={issue.level} className="mt-0.5 shrink-0" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <Link
              to={`/issues/${issue.id}`}
              onClick={(e) => e.stopPropagation()}
              className="group-hover:text-primary truncate text-sm font-medium transition-colors"
            >
              {issue.title}
            </Link>
            {issue.regressionCount > 0 ? (
              <span className="border-destructive/30 bg-destructive/10 text-destructive shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium">
                回归 ×{issue.regressionCount}
              </span>
            ) : null}
          </div>
          <div className="text-muted-foreground mt-1 flex items-center gap-2 text-[11px]">
            <StatusBadge status={issue.status} className="scale-90" />
            <SourceBadge source={issue.source} className="scale-90" />
            <span className="truncate font-mono">{issue.culprit || "—"}</span>
            {issue.assigneeName ? (
              <span className="flex shrink-0 items-center gap-1">
                <UserRound className="size-3" /> {issue.assigneeName}
              </span>
            ) : null}
          </div>
        </div>
      </div>
      <div className="shrink-0 pl-1 text-right sm:w-24 sm:pl-0">
        <p className="text-sm font-semibold tabular-nums">{formatNumber(issue.count)}</p>
        <p className="text-muted-foreground text-[10px]">{relativeTime(issue.lastSeen)}</p>
      </div>
    </div>
  )
}
