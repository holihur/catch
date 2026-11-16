import * as React from "react"
import {
  Building2,
  Check,
  Code2,
  Copy,
  ExternalLink,
  FileCode2,
  GitBranch,
  KeyRound,
  Loader2,
  Percent,
  Upload,
  Plus,
  RefreshCw,
  Send,
  ShieldCheck,
  Trash2,
  Users,
  Webhook,
} from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import type { Project, SourceMapMeta, TenantResponse, User } from "@/lib/types"
import { useAuth } from "@/context/AuthContext"
import { useProjects } from "@/context/ProjectContext"
import { sendTestEvent, telemetryEnabled, telemetryInfo } from "@/lib/telemetry"
import { copyText, formatBytes, formatFullDate, relativeTime } from "@/lib/utils"
import { buildSourceUrl, detectProvider, repoLabel } from "@/lib/source"
import { PageHeader } from "@/components/common/PageHeader"
import { ConfirmDialog } from "@/components/common/ConfirmDialog"
import { CopyButton } from "@/components/common/CopyButton"
import { ErrorState } from "@/components/common/ErrorState"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

const ROLE_LABEL: Record<string, string> = {
  owner: "Owner",
  admin: "Admin",
  member: "Member",
  viewer: "Viewer",
}

export function SettingsPage() {
  const { me, refresh } = useAuth()
  const { projects, selected, refreshProjects, setSelectedId } = useProjects()
  const [tab, setTab] = React.useState("projects")
  const [tenantRes, setTenantRes] = React.useState<TenantResponse | null>(null)
  const [members, setMembers] = React.useState<User[]>([])
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<unknown>(null)
  const [createOpen, setCreateOpen] = React.useState(false)
  const [sourceProject, setSourceProject] = React.useState<Project | null>(null)
  const [sourceMapProject, setSourceMapProject] = React.useState<Project | null>(null)
  const [samplingProject, setSamplingProject] = React.useState<Project | null>(null)
  const [confirm, setConfirm] = React.useState<{ type: "delete" | "rotate"; project: Project } | null>(null)
  const [confirmBusy, setConfirmBusy] = React.useState(false)

  const canManage = Boolean(me?.permissions.canManage)

  const load = React.useCallback(async () => {
    setLoading(true)
    try {
      const [t, m] = await Promise.all([api.tenant(), api.members()])
      setTenantRes(t)
      setMembers(m.members)
      setError(null)
    } catch (err) {
      setError(err)
    } finally {
      setLoading(false)
    }
  }, [])

  React.useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="space-y-6 sm:space-y-8">
      <PageHeader
        title="设置"
        description="管理项目、采集密钥、成员角色与 OIDC 单点登录。"
        actions={
          <Button variant="outline" size="icon" onClick={() => void load()} title="刷新">
            <RefreshCw className="size-4" />
          </Button>
        }
      />

      {error && !tenantRes ? <ErrorState error={error} onRetry={() => void load()} /> : null}

      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList className="grid h-auto w-full grid-cols-2 gap-1 p-1 sm:inline-flex sm:w-fit">
          <TabsTrigger value="projects">
            <KeyRound className="size-3.5" /> 项目与密钥
          </TabsTrigger>
          <TabsTrigger value="members">
            <Users className="size-3.5" /> 成员
          </TabsTrigger>
          <TabsTrigger value="tenant">
            <Building2 className="size-3.5" /> 租户与 SSO
          </TabsTrigger>
          <TabsTrigger value="sdk">
            <Code2 className="size-3.5" /> SDK 接入
          </TabsTrigger>
        </TabsList>

        <TabsContent value="projects" className="space-y-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="text-sm font-medium">采集项目</p>
              <p className="text-muted-foreground text-xs">每个项目拥有独立采集密钥，用于隔离不同应用的事件。</p>
            </div>
            {canManage ? (
              <Button size="sm" className="self-start sm:self-auto" onClick={() => setCreateOpen(true)}>
                <Plus className="size-4" /> 新建项目
              </Button>
            ) : null}
          </div>
          {loading && projects.length === 0 ? (
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <Skeleton className="h-44 rounded-xl" />
              <Skeleton className="h-44 rounded-xl" />
            </div>
          ) : projects.length === 0 ? (
            <Card>
              <CardContent className="text-muted-foreground py-10 text-center text-sm">
                还没有项目，创建一个项目后即可获得采集密钥。
              </CardContent>
            </Card>
          ) : (
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {projects.map((p) => (
                <Card key={p.id} className="gap-4">
                  <CardHeader className="flex flex-row items-start justify-between space-y-0">
                    <div>
                      <CardTitle className="flex items-center gap-2 text-base">
                        {p.name}
                        {p.id === selected?.id ? (
                          <Badge variant="outline" className="text-[10px]">
                            当前
                          </Badge>
                        ) : null}
                      </CardTitle>
                      <CardDescription className="mt-1 font-mono text-xs">
                        {p.slug} · {p.platform || "未设置平台"} · Trace 采样{" "}
                        {Math.round((p.traceSampleRate ?? 1) * 100)}%
                      </CardDescription>
                      {p.repoUrl ? (
                        <a
                          href={p.repoUrl}
                          target="_blank"
                          rel="noreferrer noopener"
                          className="text-muted-foreground hover:text-primary mt-1.5 inline-flex max-w-[260px] items-center gap-1 text-[11px]"
                          title={p.repoUrl}
                        >
                          <GitBranch className="size-3 shrink-0" />
                          <span className="truncate">{repoLabel(p)}</span>
                          <ExternalLink className="size-2.5 shrink-0" />
                        </a>
                      ) : p.repoLinkTemplate ? (
                        <span className="text-muted-foreground mt-1.5 inline-flex max-w-[260px] items-center gap-1 text-[11px]">
                          <GitBranch className="size-3 shrink-0" />
                          <span className="truncate">自定义源码链接模板</span>
                        </span>
                      ) : null}
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setSelectedId(p.id)}
                      disabled={p.id === selected?.id}
                    >
                      {p.id === selected?.id ? "已选择" : "切换"}
                    </Button>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div>
                      <p className="text-muted-foreground mb-1 text-[10px] font-medium tracking-wider uppercase">
                        Ingest Key
                      </p>
                      <div className="bg-muted/50 flex items-center gap-2 rounded-xl px-3 py-2">
                        <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{p.ingestKey}</code>
                        <CopyButton value={p.ingestKey} label="复制采集密钥" />
                      </div>
                      {!canManage ? (
                        <p className="text-muted-foreground mt-1 text-[10px]">仅 Admin 及以上角色可查看和轮换密钥</p>
                      ) : null}
                    </div>
                    <div className="flex items-center justify-between">
                      <p className="text-muted-foreground text-[10px]">
                        创建于 {formatFullDate(p.createdAt)}
                      </p>
                      {canManage ? (
                        <div className="flex flex-wrap justify-end gap-1">
                          <Button variant="ghost" size="sm" onClick={() => setSourceProject(p)}>
                            <Code2 className="size-3.5" /> 源码
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => setSourceMapProject(p)}>
                            <FileCode2 className="size-3.5" /> Source Maps
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => setSamplingProject(p)}>
                            <Percent className="size-3.5" /> 采样
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setConfirm({ type: "rotate", project: p })}
                          >
                            <RefreshCw className="size-3.5" /> 轮换
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => setConfirm({ type: "delete", project: p })}
                          >
                            <Trash2 className="size-3.5" /> 删除
                          </Button>
                        </div>
                      ) : null}
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="members" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">成员与角色</CardTitle>
              <CardDescription>
                OIDC 用户首次登录时自动加入租户；默认角色为 {tenantRes?.tenant.defaultRole ?? "member"}。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-2 md:hidden">
                {members.map((m) => (
                  <div key={m.id} className="bg-muted/30 flex items-center gap-3 rounded-xl p-3">
                    <div className="bg-primary/10 text-primary flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold">
                      {(m.name || m.email).slice(0, 1).toUpperCase()}
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{m.name || m.email}</p>
                      <p className="text-muted-foreground truncate text-[11px]">{m.email}</p>
                    </div>
                    {canManage && m.role !== "owner" && m.id !== me?.user.id ? (
                      <Select value={m.role} onValueChange={(role) => void changeRole(m, role)}>
                        <SelectTrigger className="h-7 w-[104px] text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="viewer">Viewer</SelectItem>
                          <SelectItem value="member">Member</SelectItem>
                          <SelectItem value="admin">Admin</SelectItem>
                        </SelectContent>
                      </Select>
                    ) : (
                      <Badge variant="outline" className="shrink-0 font-mono text-[10px]">
                        {ROLE_LABEL[m.role] ?? m.role}
                      </Badge>
                    )}
                    {canManage && m.role !== "owner" && m.id !== me?.user.id ? (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        className="text-muted-foreground hover:text-destructive shrink-0"
                        title="移除成员"
                        onClick={() => void removeMember(m)}
                      >
                        <Trash2 className="size-3.5" />
                      </Button>
                    ) : null}
                  </div>
                ))}
              </div>

              <div className="hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>成员</TableHead>
                    <TableHead>角色</TableHead>
                    <TableHead className="hidden md:table-cell">最近登录</TableHead>
                    {canManage ? <TableHead className="text-right">操作</TableHead> : null}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {members.map((m) => (
                    <TableRow key={m.id}>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <div className="bg-primary/10 text-primary flex size-7 items-center justify-center rounded-full text-[11px] font-semibold">
                            {(m.name || m.email).slice(0, 1).toUpperCase()}
                          </div>
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium">{m.name || m.email}</p>
                            <p className="text-muted-foreground truncate text-[11px]">{m.email}</p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        {canManage && m.role !== "owner" && m.id !== me?.user.id ? (
                          <Select value={m.role} onValueChange={(role) => void changeRole(m, role)}>
                            <SelectTrigger className="h-7 w-[110px] text-xs">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="viewer">Viewer</SelectItem>
                              <SelectItem value="member">Member</SelectItem>
                              <SelectItem value="admin">Admin</SelectItem>
                            </SelectContent>
                          </Select>
                        ) : (
                          <Badge variant="outline" className="font-mono text-[10px]">
                            {ROLE_LABEL[m.role] ?? m.role}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground hidden text-xs md:table-cell">
                        {relativeTime(m.lastLoginAt)}
                      </TableCell>
                      {canManage ? (
                        <TableCell className="text-right">
                          {m.role !== "owner" && m.id !== me?.user.id ? (
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              className="text-destructive hover:text-destructive"
                              title="移除成员"
                              onClick={() => void removeMember(m)}
                            >
                              <Trash2 className="size-3.5" />
                            </Button>
                          ) : null}
                        </TableCell>
                      ) : null}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="tenant" className="space-y-4">
          {tenantRes ? (
            <TenantSettings
              data={tenantRes}
              canManage={canManage}
              onUpdated={() => {
                void load()
                void refresh()
              }}
            />
          ) : (
            <Skeleton className="h-64 rounded-xl" />
          )}
        </TabsContent>

        <TabsContent value="sdk" className="space-y-4">
          <SdkSettings projects={projects} selected={selected} canManage={canManage} />
        </TabsContent>
      </Tabs>

      <CreateProjectDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={async (p) => {
          await refreshProjects()
          setSelectedId(p.id)
          setCreateOpen(false)
          toast.success("项目已创建")
        }}
      />

      {sourceMapProject ? (
        <ProjectSourceMapsDialog
          project={sourceMapProject}
          onClose={() => setSourceMapProject(null)}
        />
      ) : null}

      {samplingProject ? (
        <ProjectSamplingDialog
          project={samplingProject}
          onClose={() => setSamplingProject(null)}
          onSaved={async () => {
            await refreshProjects()
            setSamplingProject(null)
          }}
        />
      ) : null}

      {sourceProject ? (
        <ProjectSourceDialog
          project={sourceProject}
          canManage={canManage}
          onClose={() => setSourceProject(null)}
          onSaved={async () => {
            await refreshProjects()
            setSourceProject(null)
          }}
        />
      ) : null}

      <ConfirmDialog
        open={Boolean(confirm)}
        onOpenChange={(open) => !open && setConfirm(null)}
        title={confirm?.type === "delete" ? "删除项目" : "轮换采集密钥"}
        description={
          confirm?.type === "delete" ? (
            <>
              删除 <strong>{confirm.project.name}</strong> 后，问题分组会被移除，旧密钥立即失效。此操作不可撤销。
            </>
          ) : (
            <>
              轮换后旧密钥立即失效，使用旧密钥的客户端将无法继续上报。请及时更新部署配置。
            </>
          )
        }
        confirmLabel={confirm?.type === "delete" ? "删除项目" : "确认轮换"}
        destructive={confirm?.type === "delete"}
        loading={confirmBusy}
        onConfirm={async () => {
          if (!confirm) return
          setConfirmBusy(true)
          try {
            if (confirm.type === "delete") {
              await api.deleteProject(confirm.project.id)
              toast.success("项目已删除")
            } else {
              const updated = await api.rotateKey(confirm.project.id)
              toast.success("密钥已轮换")
              await copyText(updated.ingestKey)
            }
            setConfirm(null)
            await refreshProjects()
          } catch (err) {
            toast.error(err instanceof Error ? err.message : "操作失败")
          } finally {
            setConfirmBusy(false)
          }
        }}
      />
    </div>
  )

  async function changeRole(member: User, role: string) {
    try {
      await api.updateMember(member.id, role)
      setMembers((prev) => prev.map((m) => (m.id === member.id ? { ...m, role } : m)))
      toast.success(`已将 ${member.name || member.email} 的角色更新为 ${ROLE_LABEL[role]}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新角色失败")
    }
  }

  async function removeMember(member: User) {
    if (!window.confirm(`确定移除 ${member.name || member.email} 吗？其会话将立即失效。`)) return
    try {
      await api.removeMember(member.id)
      setMembers((prev) => prev.filter((m) => m.id !== member.id))
      toast.success("成员已移除")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "移除成员失败")
    }
  }
}

function ProjectSourceMapsDialog({
  project,
  onClose,
}: {
  project: Project
  onClose: () => void
}) {
  const [maps, setMaps] = React.useState<SourceMapMeta[]>([])
  const [loading, setLoading] = React.useState(true)
  const [release, setRelease] = React.useState("")
  const [bundle, setBundle] = React.useState("")
  const [file, setFile] = React.useState<File | null>(null)
  const [busy, setBusy] = React.useState(false)
  const [deleting, setDeleting] = React.useState<string | null>(null)
  const fileRef = React.useRef<HTMLInputElement>(null)

  const load = React.useCallback(async () => {
    setLoading(true)
    try {
      const res = await api.sourceMaps(project.id)
      setMaps(res.sourceMaps)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "加载 Source Map 失败")
    } finally {
      setLoading(false)
    }
  }, [project.id])

  React.useEffect(() => {
    void load()
  }, [load])

  const upload = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!file) {
      toast.error("请选择 .map 文件")
      return
    }
    if (!release.trim()) {
      toast.error("请填写 release，例如 web@1.2.3")
      return
    }
    setBusy(true)
    try {
      const bundleName = bundle.trim() || file.name.replace(/\.map$/i, "")
      await api.uploadSourceMap(project.id, {
        release: release.trim(),
        file: bundleName,
        map: file,
      })
      toast.success("Source Map 已上传，将用于后续事件")
      setFile(null)
      if (fileRef.current) fileRef.current.value = ""
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "上传失败")
    } finally {
      setBusy(false)
    }
  }

  const remove = async (map: SourceMapMeta) => {
    if (!window.confirm(`删除 ${map.file} (${map.release}) 的 Source Map？`)) return
    setDeleting(map.id)
    try {
      await api.deleteSourceMap(project.id, map.id)
      toast.success("Source Map 已删除")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    } finally {
      setDeleting(null)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileCode2 className="size-4" /> Source Maps
          </DialogTitle>
          <DialogDescription>
            为 <strong>{project.name}</strong> 上传构建产物的 Source Map。上传后，新的堆栈事件会自动还原到原始源码位置。
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={upload} className="space-y-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="sm-release">Release</Label>
              <Input
                id="sm-release"
                placeholder="web@1.2.3"
                value={release}
                onChange={(e) => setRelease(e.target.value)}
              />
              <p className="text-muted-foreground text-[10px]">
                必须与 SDK 的 VITE_RELEASE / release 字段完全一致
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="sm-bundle">Bundle 文件（可选）</Label>
              <Input
                id="sm-bundle"
                placeholder="assets/index-abc.js"
                value={bundle}
                onChange={(e) => setBundle(e.target.value)}
              />
              <p className="text-muted-foreground text-[10px]">
                留空时自动使用上传文件名（去掉 .map）
              </p>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="sm-file">Source Map 文件</Label>
            <Input
              ref={fileRef}
              id="sm-file"
              type="file"
              accept=".map,application/json"
              className="h-auto py-1.5 text-xs file:mr-3 file:rounded-md file:border-0 file:bg-primary/10 file:px-2 file:py-1 file:text-xs file:font-medium file:text-primary"
              onChange={(e) => {
                const next = e.target.files?.[0] ?? null
                setFile(next)
                if (next && !bundle.trim()) {
                  setBundle(next.name.replace(/\.map$/i, ""))
                }
              }}
            />
            <div className="text-muted-foreground flex items-center justify-between text-[10px]">
              <span>支持 .map（≤20MB），上传后 gzip 存储</span>
              {file ? <span>{formatBytes(file.size)}</span> : null}
            </div>
          </div>
          <Button type="submit" size="sm" disabled={busy} className="w-full sm:w-auto">
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
            上传 Source Map
          </Button>
        </form>

        <div className="bg-muted/30 text-muted-foreground rounded-xl p-3 text-[11px] leading-relaxed">
          请在发布前上传，或确保事件到达前已完成上传。已入库的历史事件不会自动重写；如需重新处理，请重新上报事件。
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">已上传（{maps.length}）</p>
            <Button variant="ghost" size="sm" onClick={() => void load()} disabled={loading}>
              <RefreshCw className={loading ? "size-3.5 animate-spin" : "size-3.5"} /> 刷新
            </Button>
          </div>
          {loading ? (
            <p className="text-muted-foreground py-4 text-center text-xs">加载中…</p>
          ) : maps.length === 0 ? (
            <p className="text-muted-foreground bg-muted/20 rounded-xl px-4 py-6 text-center text-xs">
              还没有 Source Map
            </p>
          ) : (
            maps.map((map) => (
              <div key={map.id} className="bg-muted/30 flex items-start gap-3 rounded-xl p-3">
                <FileCode2 className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="truncate font-mono text-xs">{map.file}</span>
                    <Badge variant="outline" className="font-mono text-[10px]">
                      {map.release}
                    </Badge>
                  </div>
                  <p className="text-muted-foreground mt-1 text-[10px]">
                    {formatBytes(map.size)} · {map.sourceCount} sources · {relativeTime(map.updatedAt)}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-muted-foreground hover:text-destructive shrink-0"
                  title="删除 Source Map"
                  disabled={deleting === map.id}
                  onClick={() => void remove(map)}
                >
                  {deleting === map.id ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Trash2 className="size-3.5" />
                  )}
                </Button>
              </div>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ProjectSamplingDialog({
  project,
  onClose,
  onSaved,
}: {
  project: Project
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const [percent, setPercent] = React.useState(Math.round((project.traceSampleRate ?? 1) * 100))
  const [errorAlways, setErrorAlways] = React.useState(project.traceErrorAlwaysSample ?? true)
  const [busy, setBusy] = React.useState(false)

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!Number.isFinite(percent) || percent < 0 || percent > 100) {
      toast.error("采样率必须在 0-100 之间")
      return
    }
    setBusy(true)
    try {
      await api.updateProject(project.id, {
        traceSampleRate: percent / 100,
        traceErrorAlwaysSample: errorAlways,
      })
      toast.success("Trace 采样设置已保存")
      await onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={save} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Percent className="size-4" /> Trace 采样
            </DialogTitle>
            <DialogDescription>
              控制 <strong>{project.name}</strong> 的 span 入库比例。按 trace ID 稳定采样，同一条 Trace 不会只存一部分。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="trace-sample-rate">采样率（%）</Label>
            <Input
              id="trace-sample-rate"
              type="number"
              min={0}
              max={100}
              step={1}
              value={percent}
              onChange={(e) => setPercent(Number(e.target.value))}
            />
            <p className="text-muted-foreground text-[11px]">
              100% 保存全部 Trace，0% 只保留错误 Trace（若开启下方选项）。
            </p>
          </div>
          <div className="flex items-center justify-between gap-4 rounded-xl bg-muted/40 px-3 py-2.5">
            <div>
              <p className="text-sm font-medium">错误 Trace 始终保留</p>
              <p className="text-muted-foreground text-[11px]">
                含 ERROR span 或 exception 事件的 Trace 不受采样率影响。
              </p>
            </div>
            <Switch checked={errorAlways} onCheckedChange={setErrorAlways} />
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose} disabled={busy}>
              取消
            </Button>
            <Button type="submit" disabled={busy}>
              {busy ? <Loader2 className="size-4 animate-spin" /> : null} 保存
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ProjectSourceDialog({
  project,
  canManage,
  onClose,
  onSaved,
}: {
  project: Project
  canManage: boolean
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const [provider, setProvider] = React.useState(project.repoProvider || "github")
  const [repoUrl, setRepoUrl] = React.useState(project.repoUrl || "")
  const [branch, setBranch] = React.useState(project.repoBranch || "main")
  const [root, setRoot] = React.useState(project.repoRoot || "")
  const [template, setTemplate] = React.useState(project.repoLinkTemplate || "")
  const [busy, setBusy] = React.useState(false)

  const previewProject: Project = {
    ...project,
    repoProvider: provider,
    repoUrl: repoUrl.trim(),
    repoBranch: branch.trim() || "main",
    repoRoot: root.trim(),
    repoLinkTemplate: template.trim(),
  }
  const preview = buildSourceUrl(previewProject, "src/App.tsx", 42, 7)

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.updateProject(project.id, {
        repoProvider: provider,
        repoUrl: repoUrl.trim(),
        repoBranch: branch.trim(),
        repoRoot: root.trim(),
        repoLinkTemplate: template.trim(),
      })
      toast.success("源码映射已保存")
      await onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <form onSubmit={save} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Code2 className="size-4" /> 源码映射
            </DialogTitle>
            <DialogDescription>
              配置 <strong>{project.name}</strong> 的仓库后，堆栈帧和错误位置可以直接跳转到对应源码行。
            </DialogDescription>
          </DialogHeader>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="repo-provider">源码平台</Label>
              <Select value={provider} onValueChange={setProvider} disabled={!canManage}>
                <SelectTrigger id="repo-provider" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="github">GitHub</SelectItem>
                  <SelectItem value="gitlab">GitLab</SelectItem>
                  <SelectItem value="bitbucket">Bitbucket</SelectItem>
                  <SelectItem value="gitea">Gitea</SelectItem>
                  <SelectItem value="custom">自定义 URL 模板</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="repo-branch">分支</Label>
              <Input
                id="repo-branch"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                placeholder="main"
                disabled={!canManage}
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="repo-url">仓库地址</Label>
              <Input
                id="repo-url"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                onBlur={() => {
                  if (repoUrl.trim() && !project.repoProvider) {
                    setProvider(detectProvider(repoUrl.trim()) || "custom")
                  }
                }}
                placeholder="https://github.com/owner/repo"
                disabled={!canManage}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="repo-root">源码根目录</Label>
              <Input
                id="repo-root"
                value={root}
                onChange={(e) => setRoot(e.target.value)}
                placeholder="frontend"
                disabled={!canManage}
              />
              <p className="text-muted-foreground text-[10px]">
                堆栈路径会拼接到该目录下，例如 frontend/src/App.tsx
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="repo-template">自定义模板（可选）</Label>
              <Input
                id="repo-template"
                value={template}
                onChange={(e) => setTemplate(e.target.value)}
                placeholder="https://git.example.com/repo/blob/{branch}/{path}#L{line}"
                disabled={!canManage}
              />
              <p className="text-muted-foreground text-[10px]">
                支持 {"{path} {line} {column} {branch}"}，设置后优先使用
              </p>
            </div>
          </div>

          <div className="bg-muted/40 space-y-1.5 rounded-xl p-3 text-xs">
            <p className="text-muted-foreground">链接预览</p>
            {preview ? (
              <a
                href={preview}
                target="_blank"
                rel="noreferrer noopener"
                className="text-primary block break-all font-mono text-[11px] hover:underline"
              >
                {preview}
              </a>
            ) : (
              <p className="text-muted-foreground">填写仓库地址或自定义模板后显示</p>
            )}
            <p className="text-muted-foreground pt-1 text-[10px]">
              捆绑后的 index-*.js 和 node_modules 帧会被忽略；如需还原压缩代码请接入 Source Map。
            </p>
          </div>

          {canManage ? (
            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
                取消
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}
                保存源码映射
              </Button>
            </DialogFooter>
          ) : null}
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CreateProjectDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (project: Project) => void
}) {
  const [name, setName] = React.useState("")
  const [slug, setSlug] = React.useState("")
  const [platform, setPlatform] = React.useState("javascript-react")
  const [busy, setBusy] = React.useState(false)

  React.useEffect(() => {
    if (open) {
      setName("")
      setSlug("")
      setPlatform("javascript-react")
    }
  }, [open])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setBusy(true)
    try {
      const p = await api.createProject({
        name: name.trim(),
        slug: slug.trim() || undefined,
        platform: platform.trim() || undefined,
      })
      onCreated(p)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建项目失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>新建项目</DialogTitle>
            <DialogDescription>项目用于隔离不同应用的日志和错误，并提供独立采集密钥。</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="project-name">名称</Label>
            <Input
              id="project-name"
              placeholder="例如：Web 控制台"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="project-slug">标识（可选）</Label>
              <Input
                id="project-slug"
                placeholder="web-console"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="project-platform">平台（可选）</Label>
              <Input
                id="project-platform"
                placeholder="javascript-react"
                value={platform}
                onChange={(e) => setPlatform(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
              取消
            </Button>
            <Button type="submit" disabled={busy || !name.trim()}>
              {busy ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
              创建项目
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function TenantSettings({
  data,
  canManage,
  onUpdated,
}: {
  data: TenantResponse
  canManage: boolean
  onUpdated: () => void
}) {
  const [name, setName] = React.useState(data.tenant.name)
  const [nameBusy, setNameBusy] = React.useState(false)
  const [enabled, setEnabled] = React.useState(Boolean(data.tenant.oidc?.enabled))
  const [issuer, setIssuer] = React.useState(data.tenant.oidc?.issuer ?? "")
  const [clientId, setClientId] = React.useState(data.tenant.oidc?.clientId ?? "")
  const [secret, setSecret] = React.useState("")
  const [scopes, setScopes] = React.useState((data.tenant.oidc?.scopes ?? []).join(", "))
  const [domains, setDomains] = React.useState((data.tenant.allowedDomains ?? []).join(", "))
  const [busy, setBusy] = React.useState(false)
  const [testing, setTesting] = React.useState(false)

  React.useEffect(() => {
    setName(data.tenant.name)
    setEnabled(Boolean(data.tenant.oidc?.enabled))
    setIssuer(data.tenant.oidc?.issuer ?? "")
    setClientId(data.tenant.oidc?.clientId ?? "")
    setScopes((data.tenant.oidc?.scopes ?? []).join(", "))
    setDomains((data.tenant.allowedDomains ?? []).join(", "))
  }, [data])

  const callbackURL = `${window.location.origin}/api/auth/oidc/callback`

  const saveName = async () => {
    if (!name.trim() || name === data.tenant.name) return
    setNameBusy(true)
    try {
      await api.updateTenant(name.trim())
      toast.success("租户名称已更新")
      onUpdated()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新失败")
    } finally {
      setNameBusy(false)
    }
  }

  const saveOIDC = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.updateOIDC({
        issuer: issuer.trim(),
        clientId: clientId.trim(),
        clientSecret: secret.trim(),
        scopes: scopes.split(",").map((s) => s.trim()).filter(Boolean),
        enabled,
        allowedDomains: domains.split(",").map((s) => s.trim()).filter(Boolean),
      })
      setSecret("")
      toast.success("OIDC 配置已保存")
      onUpdated()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  const testLogin = async () => {
    setTesting(true)
    try {
      const res = await api.oidcStart(data.tenant.slug, "/settings")
      window.location.assign(res.url)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "无法启动 OIDC 登录")
      setTesting(false)
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">租户信息</CardTitle>
          <CardDescription>租户标识用于登录入口，创建后不可修改。</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="w-full space-y-2 sm:max-w-sm">
            <Label htmlFor="tenant-name">名称</Label>
            <Input
              id="tenant-name"
              value={name}
              disabled={!canManage}
              onChange={(e) => setName(e.target.value)}
              onBlur={() => void saveName()}
            />
          </div>
          <div className="bg-muted/50 rounded-md border px-3 py-2 text-xs">
            <span className="text-muted-foreground">登录标识：</span>
            <span className="font-mono">{data.tenant.slug}</span>
          </div>
          {nameBusy ? <Loader2 className="text-muted-foreground size-4 animate-spin" /> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="size-4" /> OIDC 单点登录
          </CardTitle>
          <CardDescription>
            每个租户可独立配置身份提供方。Client Secret 使用 AES-GCM 加密存储，不会返回给浏览器。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={saveOIDC} className="space-y-4">
            <div className="bg-muted/30 flex items-center justify-between rounded-xl p-4">
              <div>
                <p className="text-sm font-medium">启用 OIDC 登录</p>
                <p className="text-muted-foreground text-xs">
                  关闭后仍可使用开发登录（仅限非生产环境）。
                </p>
              </div>
              <Switch checked={enabled} onCheckedChange={setEnabled} disabled={!canManage} />
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="issuer">Issuer</Label>
                <Input
                  id="issuer"
                  placeholder="http://localhost:8188/t/acme"
                  value={issuer}
                  disabled={!canManage}
                  onChange={(e) => setIssuer(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="client-id">Client ID</Label>
                <Input
                  id="client-id"
                  placeholder="app_demo_acme"
                  value={clientId}
                  disabled={!canManage}
                  onChange={(e) => setClientId(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="client-secret">Client Secret</Label>
                <Input
                  id="client-secret"
                  type="password"
                  placeholder={data.hasSecret ? "已配置，留空保持不变" : "输入 client secret"}
                  value={secret}
                  disabled={!canManage}
                  onChange={(e) => setSecret(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="scopes">额外 Scopes</Label>
                <Input
                  id="scopes"
                  placeholder="roles, offline_access"
                  value={scopes}
                  disabled={!canManage}
                  onChange={(e) => setScopes(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="domains">允许的邮箱域名</Label>
                <Input
                  id="domains"
                  placeholder="acme.test, example.com（留空不限制）"
                  value={domains}
                  disabled={!canManage}
                  onChange={(e) => setDomains(e.target.value)}
                />
              </div>
            </div>

            <div className="bg-muted/40 space-y-2 rounded-xl p-4 text-xs">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <p className="text-muted-foreground">回调地址（需在身份提供方注册）</p>
                  <code className="mt-1 block break-all font-mono">{callbackURL}</code>
                </div>
                <CopyButton value={callbackURL} />
              </div>
              {issuer ? (
                <div className="flex items-start justify-between gap-2 border-t pt-2">
                  <div>
                    <p className="text-muted-foreground">Discovery</p>
                    <code className="mt-1 block break-all font-mono">
                      {issuer.replace(/\/$/, "")}/.well-known/openid-configuration
                    </code>
                  </div>
                  <CopyButton value={`${issuer.replace(/\/$/, "")}/.well-known/openid-configuration`} />
                </div>
              ) : null}
            </div>

            {canManage ? (
              <div className="flex flex-wrap justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void testLogin()}
                  disabled={testing || !enabled}
                >
                  {testing ? <Loader2 className="size-4 animate-spin" /> : <ShieldCheck className="size-4" />}
                  测试 OIDC 登录
                </Button>
                <Button type="submit" disabled={busy}>
                  {busy ? <Loader2 className="size-4 animate-spin" /> : <Check className="size-4" />}
                  保存 OIDC 配置
                </Button>
              </div>
            ) : (
              <p className="text-muted-foreground text-xs">仅 Admin 及以上角色可以修改 OIDC 配置。</p>
            )}
          </form>
        </CardContent>
      </Card>
    </>
  )
}

function SdkSettings({
  projects,
  selected,
  canManage,
}: {
  projects: Project[]
  selected: Project | null
  canManage: boolean
}) {
  const project = selected ?? projects[0] ?? null
  const [testBusy, setTestBusy] = React.useState<"log" | "error" | null>(null)
  const info = telemetryInfo()
  const key = project?.ingestKey ?? "ck_..."
  const masked = !canManage && key !== "ck_..."

  const frontendSnippet = `# .env
VITE_API_URL=            # 与 API 同域时可留空
VITE_INGEST_KEY=${key}
VITE_ENV=production
VITE_RELEASE=web@1.2.3`
  const curlSnippet = `curl -X POST ${window.location.origin}/api/ingest \\
  -H "X-Ingest-Key: ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{"events":[
    {"kind":"error","source":"frontend","level":"error",
     "message":"TypeError: x is undefined",
     "stack":"TypeError: x\\n    at App (App.tsx:42:7)",
     "url":"https://app.example.com/checkout",
     "user":{"id":"u_123"},"tags":{"release":"web@1.2.3"}},
    {"kind":"log","source":"backend","level":"info",
     "message":"order created","service":"orders-api",
     "context":{"orderId":"ord_123","durationMs":42}}
  ]}'`
  const goSnippet = `// 任何服务端都可以通过 ingest key 上报日志/错误
POST ${window.location.origin}/api/ingest
X-Ingest-Key: ${key}
Content-Type: application/json

{"events":[{"kind":"log","source":"backend","service":"orders-api",
  "level":"warn","message":"slow query","context":{"ms":830}}]}`

  const test = async (kind: "log" | "error") => {
    if (!project) return
    if (masked) {
      toast.error("当前角色无法查看完整密钥，无法发送测试事件")
      return
    }
    setTestBusy(kind)
    try {
      const res = await sendTestEvent(project.ingestKey, kind)
      toast.success(`测试事件已接收（${res.accepted} 条），可在日志或问题页查看`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "发送失败")
    } finally {
      setTestBusy(null)
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Webhook className="size-4" /> 采集状态
          </CardTitle>
          <CardDescription>浏览器 SDK 与后端服务使用同一个采集接口，按项目密钥隔离。</CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="bg-muted/30 rounded-xl p-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">浏览器 SDK</span>
              <Badge variant="outline" className={telemetryEnabled() ? "border-success/40 text-success" : "border-warning/40"}>
                {telemetryEnabled() ? "已启用" : "未配置密钥"}
              </Badge>
            </div>
            <p className="text-muted-foreground mt-2 break-all font-mono text-[11px]">
              {info.endpoint}
            </p>
            <p className="text-muted-foreground mt-1 text-[11px]">
              环境 {info.environment} · 版本 {info.release}
            </p>
          </div>
          <div className="bg-muted/30 rounded-xl p-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">当前项目</span>
              <Badge variant="outline" className="font-mono text-[10px]">
                {project?.slug ?? "—"}
              </Badge>
            </div>
            <p className="text-muted-foreground mt-2 text-[11px]">使用该项目的密钥发送测试事件：</p>
            <div className="mt-2 flex gap-2">
              <Button size="sm" variant="outline" disabled={!project || testBusy !== null} onClick={() => void test("log")}>
                {testBusy === "log" ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}
                测试日志
              </Button>
              <Button size="sm" variant="outline" disabled={!project || testBusy !== null} onClick={() => void test("error")}>
                {testBusy === "error" ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}
                测试错误
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <CodeCard
        title="前端接入"
        description="构建时注入项目采集密钥。SDK 会自动捕获 window.onerror、unhandledrejection、React 错误边界和 API 5xx。"
        code={masked ? frontendSnippet.replace(key, "ck_••••（需要 Admin 权限查看）") : frontendSnippet}
      />
      <CodeCard
        title="通用上报（cURL）"
        description="支持单条事件、事件数组或 {events:[...]} 批量格式；单批最多 100 条、1MB。"
        code={curlSnippet}
      />
      <CodeCard title="后端服务接入" description="任何语言都可以通过 HTTP 上报，source=backend 的事件会与前端日志一起展示。" code={goSnippet} />
    </>
  )
}

function CodeCard({ title, description, code }: { title: string; description: string; code: string }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div>
          <CardTitle className="text-base">{title}</CardTitle>
          <CardDescription className="mt-1">{description}</CardDescription>
        </div>
        <Button variant="ghost" size="icon-sm" onClick={() => void copyText(code).then(() => toast.success("已复制"))}>
          <Copy className="size-3.5" />
        </Button>
      </CardHeader>
      <CardContent>
        <pre className="bg-muted/40 scrollbar-thin overflow-x-auto rounded-xl p-3.5 font-mono text-[11px] leading-relaxed">
          {code}
        </pre>
      </CardContent>
    </Card>
  )
}
