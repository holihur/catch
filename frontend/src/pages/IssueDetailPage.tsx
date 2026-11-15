import * as React from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import {
  ArrowLeft,
  CheckCircle2,
  CircleSlash,
  Clock,
  GitBranch,
  MessageSquare,
  Code2,
  ExternalLink,
  RefreshCw,
  RotateCcw,
  Send,
  UserRound,
} from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import type { EventItem, IssueDetail, Project, User } from "@/lib/types"
import { cn, formatFullDate, formatNumber, relativeTime } from "@/lib/utils"
import { useAuth } from "@/context/AuthContext"
import { useProjects } from "@/context/ProjectContext"
import { firstSourceFrame, repoLabel } from "@/lib/source"
import { useSourceViewer } from "@/context/SourceViewerContext"
import { PageHeader } from "@/components/common/PageHeader"
import { LevelBadge, LevelMark, SourceBadge, StatusBadge } from "@/components/common/Badges"
import { CopyButton } from "@/components/common/CopyButton"
import { ErrorState } from "@/components/common/ErrorState"
import { StackTrace } from "@/components/common/StackTrace"
import { EmptyState } from "@/components/common/EmptyState"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ErrorTrendChart } from "@/components/charts/TrendChart"

export function IssueDetailPage() {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const { me } = useAuth()
  const { projects, selectedId } = useProjects()
  const { openSource } = useSourceViewer()
  const [data, setData] = React.useState<IssueDetail | null>(null)
  const [members, setMembers] = React.useState<User[]>([])
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<unknown>(null)
  const [busy, setBusy] = React.useState(false)
  const [selectedEventId, setSelectedEventId] = React.useState<string>("")
  const [tab, setTab] = React.useState("stack")
  const [comment, setComment] = React.useState("")
  const [commentBusy, setCommentBusy] = React.useState(false)

  const load = React.useCallback(async () => {
    setLoading(true)
    try {
      const [detail, memberRes] = await Promise.all([api.issue(id), api.members()])
      setData(detail)
      setMembers(memberRes.members)
      setSelectedEventId((prev) => {
        if (prev && detail.events.some((e) => e.id === prev)) return prev
        return detail.events[0]?.id ?? ""
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

  const selectedEvent = data?.events.find((e) => e.id === selectedEventId) ?? data?.events[0]
  const issue = data?.issue
  const project =
    projects.find((p) => p.id === issue?.projectId) ??
    projects.find((p) => p.id === selectedId) ??
    null
  const firstFrame = firstSourceFrame(project, selectedEvent?.release, [
    selectedEvent?.stack,
    issue?.culprit,
  ])

  const changeStatus = async (status: string) => {
    if (!issue) return
    setBusy(true)
    try {
      await api.updateIssue(issue.id, { status })
      toast.success(
        status === "resolved" ? "问题已标记为已解决" : status === "ignored" ? "问题已忽略" : "问题已重新打开",
      )
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新失败")
    } finally {
      setBusy(false)
    }
  }

  const assign = async (assigneeId: string) => {
    if (!issue) return
    setBusy(true)
    try {
      await api.updateIssue(issue.id, { assigneeId: assigneeId === "unassigned" ? "" : assigneeId })
      toast.success(assigneeId === "unassigned" ? "已取消指派" : "已更新负责人")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "指派失败")
    } finally {
      setBusy(false)
    }
  }

  const submitComment = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!issue || !comment.trim()) return
    setCommentBusy(true)
    try {
      await api.addComment(issue.id, comment.trim())
      setComment("")
      toast.success("评论已发布")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "评论失败")
    } finally {
      setCommentBusy(false)
    }
  }

  if (loading && !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-24 rounded-xl" />
        <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
          <Skeleton className="h-[420px] rounded-xl lg:col-span-2" />
          <Skeleton className="h-[420px] rounded-xl" />
        </div>
      </div>
    )
  }

  if (error && !data) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" size="sm" onClick={() => navigate("/issues")}>
          <ArrowLeft className="size-4" /> 返回问题列表
        </Button>
        <ErrorState error={error} onRetry={() => void load()} />
      </div>
    )
  }

  if (!data || !issue) return null

  return (
    <div className="space-y-6 sm:space-y-8">
      <div className="flex flex-wrap items-center gap-2">
        <Button asChild variant="ghost" size="sm">
          <Link to="/issues">
            <ArrowLeft className="size-4" /> 问题列表
          </Link>
        </Button>
        <span className="text-muted-foreground text-xs">/</span>
        <span className="text-muted-foreground font-mono text-xs">{issue.fingerprint.slice(0, 12)}</span>
        <CopyButton value={issue.fingerprint} label="复制指纹" />
        <div className="ml-auto flex items-center gap-2">
          <StatusBadge status={issue.status} />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm" disabled={busy}>
                <RefreshCw className={cn("size-4", busy && "animate-spin")} /> 变更状态
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>处理状态</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => void changeStatus("resolved")}>
                <CheckCircle2 className="size-4" /> 标记已解决
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => void changeStatus("open")}>
                <RotateCcw className="size-4" /> 重新打开
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => void changeStatus("ignored")}>
                <CircleSlash className="size-4" /> 忽略
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-2">
            <LevelBadge level={issue.level} />
            <span>{issue.title}</span>
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-2">
            <SourceBadge source={issue.source} />
            <span className="font-mono">{issue.culprit || "未知位置"}</span>
            <span>·</span>
            <span>{data.projectName ?? "未知项目"}</span>
          </span>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {firstFrame ? (
              <Button variant="outline" size="sm" onClick={() => openSource(firstFrame)}>
                <Code2 className="size-4" /> 源码
              </Button>
            ) : null}
            <UserRound className="text-muted-foreground size-4" />
            <Select value={issue.assigneeId || "unassigned"} onValueChange={(v) => void assign(v)} disabled={busy}>
              <SelectTrigger className="h-8 w-[160px] text-xs">
                <SelectValue placeholder="指派负责人" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="unassigned">未指派</SelectItem>
                {members.map((m) => (
                  <SelectItem key={m.id} value={m.id}>
                    {m.name || m.email}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        }
      />

      {issue.regressionCount > 0 ? (
        <div className="bg-destructive/5 text-destructive flex items-center gap-2 rounded-xl px-4 py-3 text-sm">
          <GitBranch className="size-4" />
          该问题已回归 {issue.regressionCount} 次，请检查修复是否覆盖根因，并补充回归测试。
        </div>
      ) : null}

      <div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">
        <MetricBox label="发生次数" value={formatNumber(issue.count)} icon={<RefreshCw className="size-4" />} />
        <MetricBox label="影响用户" value={formatNumber(data.userCount)} icon={<UserRound className="size-4" />} />
        <MetricBox label="首次发现" value={relativeTime(issue.firstSeen)} hint={formatFullDate(issue.firstSeen)} icon={<Clock className="size-4" />} />
        <MetricBox label="最近发生" value={relativeTime(issue.lastSeen)} hint={formatFullDate(issue.lastSeen)} icon={<Clock className="size-4" />} />
      </div>

      <div className="grid grid-cols-1 gap-4 sm:gap-5 lg:grid-cols-3">
        <div className="space-y-4 sm:space-y-5 lg:col-span-2">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between gap-2">
              <CardTitle className="text-base">事件详情</CardTitle>
              {selectedEvent ? (
                <div className="text-muted-foreground flex items-center gap-2 text-[11px]">
                  <span>{formatFullDate(selectedEvent.timestamp)}</span>
                  <span>·</span>
                  <span className="font-mono">{selectedEvent.id.slice(-8)}</span>
                </div>
              ) : null}
            </CardHeader>
            <CardContent>
              {selectedEvent ? (
                <EventSummary event={selectedEvent} project={project} />
              ) : (
                <EmptyState title="暂无事件" description="该问题还没有关联的事件。" />
              )}

              <Tabs value={tab} onValueChange={setTab} className="mt-4">
                <TabsList>
                  <TabsTrigger value="stack">堆栈</TabsTrigger>
                  <TabsTrigger value="breadcrumbs">面包屑</TabsTrigger>
                  <TabsTrigger value="context">上下文</TabsTrigger>
                  <TabsTrigger value="events">事件 ({data.events.length})</TabsTrigger>
                </TabsList>
                <TabsContent value="stack" className="pt-3">
                  <StackTrace
                    stack={selectedEvent?.stack}
                    project={project}
                    release={selectedEvent?.release}
                    sourceMap={selectedEvent?.sourceMap}
                  />
                </TabsContent>
                <TabsContent value="breadcrumbs" className="pt-3">
                  {selectedEvent?.breadcrumbs?.length ? (
                    <div className="space-y-2">
                      {selectedEvent.breadcrumbs.map((b, i) => (
                        <div key={i} className="bg-muted/30 flex gap-3 rounded-xl px-3 py-2.5 text-xs">
                          <span className="text-muted-foreground w-20 shrink-0 font-mono">
                            {new Date(b.timestamp).toLocaleTimeString("zh-CN", { hour12: false })}
                          </span>
                          <div className="min-w-0">
                            <p className="font-medium">
                              <span className="text-muted-foreground mr-1.5 font-mono">[{b.category}]</span>
                              {b.message}
                            </p>
                            {b.data ? (
                              <pre className="text-muted-foreground mt-1 overflow-x-auto text-[10px]">
                                {JSON.stringify(b.data, null, 2)}
                              </pre>
                            ) : null}
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-muted-foreground text-sm">该事件没有面包屑记录。</p>
                  )}
                </TabsContent>
                <TabsContent value="context" className="space-y-3 pt-3">
                  {selectedEvent?.tags && Object.keys(selectedEvent.tags).length > 0 ? (
                    <div>
                      <p className="text-muted-foreground mb-2 text-xs font-medium">标签</p>
                      <div className="flex flex-wrap gap-1.5">
                        {Object.entries(selectedEvent.tags).map(([k, v]) => (
                          <span key={k} className="bg-muted rounded-md px-2 py-1 font-mono text-[11px]">
                            <span className="text-muted-foreground">{k}=</span>
                            {v}
                          </span>
                        ))}
                      </div>
                    </div>
                  ) : null}
                  <div>
                    <p className="text-muted-foreground mb-2 text-xs font-medium">Context</p>
                    <pre className="bg-muted/40 scrollbar-thin max-h-80 overflow-auto rounded-xl p-3 text-xs">
                      {JSON.stringify(selectedEvent?.context ?? {}, null, 2)}
                    </pre>
                  </div>
                </TabsContent>
                <TabsContent value="events" className="pt-3">
                  <div className="divide-y divide-border/40 rounded-xl bg-muted/20">
                    {data.events.map((e) => (
                      <button
                        key={e.id}
                        className={cn(
                          "hover:bg-muted/50 flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors",
                          selectedEvent?.id === e.id && "bg-muted/60",
                        )}
                        onClick={() => {
                          setSelectedEventId(e.id)
                          setTab("stack")
                        }}
                      >
                        <LevelMark level={e.level} />
                        <span className="min-w-0 flex-1 truncate text-xs">{e.message}</span>
                        <span className="text-muted-foreground shrink-0 text-[11px]">
                          {relativeTime(e.timestamp)}
                        </span>
                      </button>
                    ))}
                  </div>
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <MessageSquare className="size-4" /> 处理记录 ({data.comments.length})
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {data.comments.length === 0 ? (
                <p className="text-muted-foreground text-sm">还没有评论，记录根因和修复方案，方便团队复盘。</p>
              ) : (
                <div className="space-y-3">
                  {data.comments.map((c) => (
                    <div key={c.id} className="flex gap-3">
                      <div className="bg-primary/10 text-primary flex size-7 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold">
                        {c.authorName.slice(0, 1).toUpperCase()}
                      </div>
                      <div className="bg-muted/30 min-w-0 flex-1 rounded-xl px-3 py-2.5">
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-xs font-medium">{c.authorName}</span>
                          <span className="text-muted-foreground text-[10px]">{relativeTime(c.createdAt)}</span>
                        </div>
                        <p className="mt-1 text-sm whitespace-pre-wrap">{c.body}</p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {me?.permissions.canWrite ? (
                <form onSubmit={submitComment} className="space-y-2">
                  <Textarea
                    placeholder="记录根因、修复方案或复盘结论…"
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    rows={3}
                  />
                  <div className="flex justify-end">
                    <Button type="submit" size="sm" disabled={commentBusy || !comment.trim()}>
                      <Send className="size-3.5" /> {commentBusy ? "发布中…" : "发布评论"}
                    </Button>
                  </div>
                </form>
              ) : null}
            </CardContent>
          </Card>
        </div>

        <div className="space-y-4 sm:space-y-5">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">发生趋势（7 天）</CardTitle>
            </CardHeader>
            <CardContent className="h-[180px] pl-1">
              <ErrorTrendChart errors={data.trend} logs={[]} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">元数据</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <MetaRow label="指纹">
                <span className="flex items-center gap-1 font-mono text-xs">
                  {issue.fingerprint.slice(0, 16)}…
                  <CopyButton value={issue.fingerprint} />
                </span>
              </MetaRow>
              <MetaRow label="项目">{data.projectName ?? "—"}</MetaRow>
              <MetaRow label="来源">
                <SourceBadge source={issue.source} />
              </MetaRow>
              <MetaRow label="级别">
                <LevelBadge level={issue.level} />
              </MetaRow>
              <MetaRow label="负责人">{issue.assigneeName || "未指派"}</MetaRow>
              <MetaRow label="首次发现">{formatFullDate(issue.firstSeen)}</MetaRow>
              <MetaRow label="最近发生">{formatFullDate(issue.lastSeen)}</MetaRow>
              {issue.resolvedAt ? (
                <MetaRow label="解决时间">{formatFullDate(issue.resolvedAt)}</MetaRow>
              ) : null}
              {issue.resolvedBy ? <MetaRow label="解决人">{issue.resolvedBy}</MetaRow> : null}
              {project?.repoUrl ? (
                <MetaRow label="源码仓库">
                  <a
                    href={project.repoUrl}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="text-primary inline-flex max-w-[220px] items-center gap-1 truncate hover:underline"
                    title={project.repoUrl}
                  >
                    <span className="truncate">{repoLabel(project)}</span>
                    <ExternalLink className="size-3 shrink-0" />
                  </a>
                </MetaRow>
              ) : null}
              {selectedEvent?.release ? <MetaRow label="版本">{selectedEvent.release}</MetaRow> : null}
              {selectedEvent?.environment ? <MetaRow label="环境">{selectedEvent.environment}</MetaRow> : null}
              {selectedEvent?.url ? (
                <MetaRow label="页面">
                  <span className="block max-w-[220px] truncate font-mono text-xs" title={selectedEvent.url}>
                    {selectedEvent.url}
                  </span>
                </MetaRow>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

function MetricBox({
  label,
  value,
  hint,
  icon,
}: {
  label: string
  value: string
  hint?: string
  icon: React.ReactNode
}) {
  return (
    <Card className="gap-0 py-0">
      <CardContent className="flex items-center gap-3 p-4">
        <span className="bg-muted text-muted-foreground flex size-8 items-center justify-center rounded-lg">
          {icon}
        </span>
        <div className="min-w-0">
          <p className="text-muted-foreground text-[11px]">{label}</p>
          <p className="truncate text-sm font-semibold">{value}</p>
          {hint ? <p className="text-muted-foreground truncate text-[10px]">{hint}</p> : null}
        </div>
      </CardContent>
    </Card>
  )
}

function MetaRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-muted-foreground shrink-0 text-xs">{label}</span>
      <span className="min-w-0 text-right text-xs">{children}</span>
    </div>
  )
}

function EventSummary({ event, project }: { event: EventItem; project: Project | null }) {
  const { openSource } = useSourceViewer()
  const frame = firstSourceFrame(project, event.release, [event.stack, event.culprit])
  return (
    <div className="bg-muted/30 space-y-2 rounded-xl p-3.5">
      <div className="flex flex-wrap items-center gap-2">
        <LevelBadge level={event.level} />
        <SourceBadge source={event.source} />
        {event.request?.method ? (
          <span className="bg-background rounded border px-1.5 py-0.5 font-mono text-[10px]">
            {event.request.method} {event.request.url}
          </span>
        ) : null}
        {event.request?.status ? (
          <span className="text-muted-foreground text-[10px]">HTTP {event.request.status}</span>
        ) : null}
        {event.request?.durationMs ? (
          <span className="text-muted-foreground text-[10px]">{event.request.durationMs}ms</span>
        ) : null}
      </div>
      <pre className="scrollbar-thin max-h-32 overflow-auto text-xs whitespace-pre-wrap">
        {event.message}
      </pre>
      <div className="text-muted-foreground flex flex-wrap gap-3 text-[11px]">
        <span>时间：{formatFullDate(event.timestamp)}</span>
        {event.user?.id ? <span>用户：{event.user.email || event.user.id}</span> : null}
        {event.service ? <span>服务：{event.service}</span> : null}
        {event.release ? <span>版本：{event.release}</span> : null}
        {frame ? (
          <button
            type="button"
            onClick={() => openSource(frame)}
            className="text-primary inline-flex items-center gap-1 hover:underline"
          >
            查看源码 <ExternalLink className="size-3" />
          </button>
        ) : null}
      </div>
    </div>
  )
}
