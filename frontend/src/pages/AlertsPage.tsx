import * as React from "react"
import {
  AlertTriangle,
  Bell,
  BellOff,
  BellRing,
  CalendarClock,
  CheckCircle2,
  Loader2,
  Mail,
  Pencil,
  Plus,
  Send,
  Trash2,
  Webhook,
} from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import type {
  AlertItem,
  AlertRule,
  AlertSilence,
  NotificationChannel,
  NotificationDelivery,
  Project,
} from "@/lib/types"
import { useAuth } from "@/context/AuthContext"
import { useProjects } from "@/context/ProjectContext"
import { cn, formatFullDate, formatNumber, relativeTime } from "@/lib/utils"
import { PageHeader } from "@/components/common/PageHeader"
import { EmptyState } from "@/components/common/EmptyState"
import { ErrorState } from "@/components/common/ErrorState"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"

const OPERATOR_LABEL: Record<string, string> = { gt: ">", gte: "≥", lt: "<", lte: "≤" }
const AGG_LABEL: Record<string, string> = { avg: "平均", max: "最大", min: "最小", sum: "总和", last: "最新" }

function ruleCondition(rule: AlertRule): string {
  return `${AGG_LABEL[rule.aggregation] ?? rule.aggregation}(${rule.metric}) ${OPERATOR_LABEL[rule.operator] ?? rule.operator} ${formatNumber(rule.threshold)}`
}

function windowLabel(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`
  return `${Math.round(seconds / 60)} 分钟`
}

export function AlertsPage() {
  const { selected, projects } = useProjects()
  const { me } = useAuth()
  const canManage = Boolean(me?.permissions.canManage)
  const [rules, setRules] = React.useState<AlertRule[]>([])
  const [alerts, setAlerts] = React.useState<AlertItem[]>([])
  const [firingTotal, setFiringTotal] = React.useState(0)
  const [status, setStatus] = React.useState("all")
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<unknown>(null)
  const [editing, setEditing] = React.useState<AlertRule | "new" | null>(null)
  const [metricNames, setMetricNames] = React.useState<string[]>([])
  const [channels, setChannels] = React.useState<NotificationChannel[]>([])
  const [smtpConfigured, setSmtpConfigured] = React.useState(false)
  const [channelsOpen, setChannelsOpen] = React.useState(false)
  const [silences, setSilences] = React.useState<AlertSilence[]>([])
  const [silencesOpen, setSilencesOpen] = React.useState(false)
  const projectId = selected?.id

  const load = React.useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [ruleRes, alertRes, channelRes, silenceRes] = await Promise.all([
        api.alertRules(projectId),
        api.alerts({ projectId, status }),
        api.notificationChannels(),
        api.alertSilences(projectId),
      ])
      setRules(ruleRes.rules ?? [])
      setAlerts(alertRes.alerts ?? [])
      setFiringTotal(alertRes.firing ?? 0)
      setChannels(channelRes.channels ?? [])
      setSmtpConfigured(channelRes.smtpConfigured)
      setSilences(silenceRes.silences ?? [])
    } catch (err) {
      setError(err)
    } finally {
      setLoading(false)
    }
  }, [projectId, status])

  React.useEffect(() => {
    void load()
  }, [load])

  React.useEffect(() => {
    const timer = window.setInterval(() => void load(), 20_000)
    return () => window.clearInterval(timer)
  }, [load])

  React.useEffect(() => {
    if (!editing) return
    let active = true
    api
      .metrics({ projectId, range: "24h" })
      .then((res) => {
        if (active) setMetricNames((res.metrics ?? []).map((m) => m.name).slice(0, 200))
      })
      .catch(() => undefined)
    return () => {
      active = false
    }
  }, [editing, projectId])

  const toggleRule = async (rule: AlertRule, enabled: boolean) => {
    try {
      await api.updateAlertRule(rule.id, { enabled })
      setRules((prev) => prev.map((r) => (r.id === rule.id ? { ...r, enabled } : r)))
      toast.success(enabled ? "告警规则已启用" : "告警规则已暂停")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新失败")
    }
  }

  const removeRule = async (rule: AlertRule) => {
    if (!window.confirm(`删除告警规则「${rule.name}」？当前 firing 告警会自动恢复。`)) return
    try {
      await api.deleteAlertRule(rule.id)
      toast.success("告警规则已删除")
      await load()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    }
  }

  const firingCount = firingTotal
  const now = Date.now()
  const activeSilences = silences.filter(
    (silence) => new Date(silence.startsAt).getTime() <= now && new Date(silence.endsAt).getTime() > now,
  )

  return (
    <div className="space-y-6">
      <PageHeader
        title="告警"
        description="基于 OTLP 指标持续评估阈值，异常时创建 firing 告警，恢复后自动标记 resolved。"
        actions={
          canManage ? (
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" size="sm" onClick={() => setSilencesOpen(true)}>
                <BellOff className="size-4" /> 静默
              </Button>
              <Button variant="outline" size="sm" onClick={() => setChannelsOpen(true)}>
                <Send className="size-4" /> 通知渠道
              </Button>
              <Button size="sm" disabled={!projectId} onClick={() => setEditing("new")}>
                <Plus className="size-4" /> 新建规则
              </Button>
            </div>
          ) : null
        }
      />

      {!projectId && canManage ? (
        <p className="text-muted-foreground text-xs">请先在顶部选择一个项目，再创建告警规则。</p>
      ) : null}

      {activeSilences.length > 0 ? (
        <div className="flex items-start gap-2.5 rounded-xl bg-amber-500/10 px-3 py-2.5 text-sm text-amber-800 dark:text-amber-300">
          <BellOff className="mt-0.5 size-4 shrink-0" />
          <div className="min-w-0">
            <p className="font-medium">维护窗口生效中，告警通知已静默</p>
            <p className="mt-0.5 text-[11px] opacity-80">
              {activeSilences.map((silence) => `${silence.name}（至 ${formatFullDate(silence.endsAt)}）`).join("；")}
            </p>
          </div>
        </div>
      ) : null}

      {loading && rules.length === 0 && alerts.length === 0 ? (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <Skeleton className="h-36 rounded-xl" />
          <Skeleton className="h-36 rounded-xl" />
        </div>
      ) : error ? (
        <ErrorState error={error} onRetry={() => void load()} />
      ) : (
        <>
          <section className="space-y-3">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-2">
                <BellRing className={cn("size-4", firingCount > 0 ? "text-red-500" : "text-muted-foreground")} />
                <h2 className="text-sm font-semibold">告警事件</h2>
                {firingCount > 0 ? (
                  <Badge variant="outline" className="border-red-500/30 text-[10px] text-red-600 dark:text-red-400">
                    {firingCount} firing
                  </Badge>
                ) : null}
              </div>
              <div className="flex items-center gap-1">
                {[
                  ["all", "全部"],
                  ["firing", "Firing"],
                  ["resolved", "已恢复"],
                ].map(([value, label]) => (
                  <Button
                    key={value}
                    variant={status === value ? "secondary" : "ghost"}
                    size="sm"
                    onClick={() => setStatus(value)}
                  >
                    {label}
                  </Button>
                ))}
              </div>
            </div>

            {alerts.length === 0 ? (
              <EmptyState
                icon={<CheckCircle2 className="size-5" />}
                title={status === "firing" ? "当前没有 firing 告警" : "还没有告警事件"}
                description="规则被触发后会在这里生成一条告警，恢复后保留历史记录。"
              />
            ) : (
              <div className="space-y-2">
                {alerts.map((alert) => (
                  <Card key={alert.id} className="gap-0">
                    <CardContent className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between">
                      <div className="flex min-w-0 items-start gap-2.5">
                        <span
                          className={cn(
                            "mt-1.5 size-2 shrink-0 rounded-full",
                            alert.state === "firing"
                              ? alert.severity === "critical"
                                ? "bg-red-500"
                                : "bg-amber-500"
                              : "bg-emerald-500",
                          )}
                          title={alert.state}
                        />
                        <div className="min-w-0">
                          <p className="text-sm font-medium">{alert.message}</p>
                          <p className="text-muted-foreground mt-0.5 flex flex-wrap items-center gap-x-2 text-[11px]">
                            <span>{alert.ruleName}</span>
                            <span>· {relativeTime(alert.startedAt)}开始</span>
                            {alert.resolvedAt ? <span>· {relativeTime(alert.resolvedAt)}恢复</span> : null}
                          </p>
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-2 pl-5 sm:pl-0">
                        <Badge variant="outline" className="text-[10px] uppercase">
                          {alert.severity}
                        </Badge>
                        <Badge variant={alert.state === "firing" ? "destructive" : "secondary"} className="text-[10px]">
                          {alert.state === "firing" ? "Firing" : "Resolved"}
                        </Badge>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </section>

          <section className="space-y-3">
            <div className="flex items-center gap-2">
              <Bell className="text-muted-foreground size-4" />
              <h2 className="text-sm font-semibold">规则</h2>
              <span className="text-muted-foreground text-xs">{rules.length} 条</span>
            </div>

            {rules.length === 0 ? (
              <EmptyState
                icon={<AlertTriangle className="size-5" />}
                title="还没有告警规则"
                description="选择指标、聚合方式、阈值和统计窗口，Catch 会定期评估（默认 30 秒）。"
                action={
                  canManage && projectId ? (
                    <Button size="sm" onClick={() => setEditing("new")}>
                      <Plus className="size-4" /> 新建规则
                    </Button>
                  ) : undefined
                }
              />
            ) : (
              <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                {rules.map((rule) => (
                  <Card key={rule.id} className="gap-0">
                    <CardContent className="space-y-3 p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                            {rule.name}
                            <Badge
                              variant="outline"
                              className={cn(
                                "text-[10px] uppercase",
                                rule.severity === "critical"
                                  ? "border-red-500/30 text-red-600 dark:text-red-400"
                                  : "border-amber-500/30 text-amber-600 dark:text-amber-400",
                              )}
                            >
                              {rule.severity}
                            </Badge>
                            {!rule.enabled ? (
                              <Badge variant="secondary" className="text-[10px]">
                                已暂停
                              </Badge>
                            ) : null}
                          </p>
                          <p className="text-muted-foreground mt-1 font-mono text-[11px]">{ruleCondition(rule)}</p>
                        </div>
                        <Switch
                          checked={rule.enabled}
                          disabled={!canManage}
                          onCheckedChange={(checked) => void toggleRule(rule, checked)}
                          aria-label="启用告警规则"
                        />
                      </div>

                      <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-[11px]">
                        <span className="flex items-center gap-1.5">
                          <span
                            className={cn(
                              "size-1.5 rounded-full",
                              rule.state === "firing"
                                ? "bg-red-500"
                                : rule.state === "ok"
                                  ? "bg-emerald-500"
                                  : "bg-muted-foreground/50",
                            )}
                          />
                          {rule.state === "firing"
                            ? "Firing"
                            : rule.state === "ok"
                              ? "正常"
                              : rule.state === "nodata"
                                ? "无数据"
                                : rule.state === "silenced"
                                  ? "静默中"
                                  : "待评估"}
                        </span>
                        <span>窗口 {windowLabel(rule.windowSeconds)}</span>
                        {rule.service ? <span>服务 {rule.service}</span> : <span>全部服务</span>}
                        <span>通知 {ruleChannelLabel(rule, channels)}</span>
                        <span>
                          当前 {rule.hasData ? formatNumber(rule.value) : "—"}
                          {rule.lastEvaluated ? ` · ${relativeTime(rule.lastEvaluated)}评估` : ""}
                        </span>
                      </div>

                      {canManage ? (
                        <div className="flex justify-end gap-1">
                          <Button variant="ghost" size="sm" onClick={() => setEditing(rule)}>
                            <Pencil className="size-3.5" /> 编辑
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => void removeRule(rule)}
                          >
                            <Trash2 className="size-3.5" /> 删除
                          </Button>
                        </div>
                      ) : null}
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </section>
        </>
      )}

      {editing ? (
        <RuleDialog
          key={editing === "new" ? "new" : editing.id}
          rule={editing === "new" ? null : editing}
          projectId={projectId}
          metricNames={metricNames}
          channels={channels}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await load()
          }}
        />
      ) : null}

      {silencesOpen ? (
        <SilencesDialog
          silences={silences}
          projectId={projectId}
          projects={projects}
          rules={rules}
          onClose={() => setSilencesOpen(false)}
          onChanged={load}
        />
      ) : null}

      {channelsOpen ? (
        <NotificationChannelsDialog
          channels={channels}
          smtpConfigured={smtpConfigured}
          onClose={() => setChannelsOpen(false)}
          onChanged={load}
        />
      ) : null}
    </div>
  )
}

function ruleChannelLabel(rule: AlertRule, channels: NotificationChannel[]): string {
  const selected = channels.filter((channel) => rule.channelIds?.includes(channel.id))
  return selected.length ? selected.map((channel) => channel.name).join("、") : "未绑定"
}

function RuleDialog({
  rule,
  projectId,
  metricNames,
  channels,
  onClose,
  onSaved,
}: {
  rule: AlertRule | null
  projectId?: string
  metricNames: string[]
  channels: NotificationChannel[]
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const [name, setName] = React.useState(rule?.name ?? "")
  const [metric, setMetric] = React.useState(rule?.metric ?? "")
  const [service, setService] = React.useState(rule?.service ?? "")
  const [aggregation, setAggregation] = React.useState(rule?.aggregation ?? "avg")
  const [operator, setOperator] = React.useState(rule?.operator ?? "gt")
  const [threshold, setThreshold] = React.useState(String(rule?.threshold ?? 100))
  const [windowSeconds, setWindowSeconds] = React.useState(String(rule?.windowSeconds ?? 300))
  const [severity, setSeverity] = React.useState(rule?.severity ?? "warning")
  const [enabled, setEnabled] = React.useState(rule?.enabled ?? true)
  const [channelIds, setChannelIds] = React.useState<string[]>(rule?.channelIds ?? [])
  const [busy, setBusy] = React.useState(false)

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    const thresholdValue = Number(threshold)
    if (!name.trim() || !metric.trim()) {
      toast.error("请填写规则名称和指标名称")
      return
    }
    if (!Number.isFinite(thresholdValue)) {
      toast.error("阈值必须是有效数字")
      return
    }
    setBusy(true)
    try {
      const body = {
        name: name.trim(),
        metric: metric.trim(),
        service: service.trim(),
        aggregation,
        operator,
        threshold: thresholdValue,
        windowSeconds: Number(windowSeconds),
        severity,
        enabled,
        channelIds,
      }
      if (rule) {
        await api.updateAlertRule(rule.id, body)
        toast.success("告警规则已更新")
      } else {
        if (!projectId) {
          toast.error("请先选择项目")
          return
        }
        await api.createAlertRule({ ...body, projectId })
        toast.success("告警规则已创建")
      }
      await onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <form onSubmit={save} className="space-y-4">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Bell className="size-4" /> {rule ? "编辑告警规则" : "新建告警规则"}
            </DialogTitle>
            <DialogDescription>
              对统计窗口内的指标做聚合，满足条件时创建告警。连续满足只保留一条 firing 记录。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-2">
            <Label htmlFor="alert-name">规则名称</Label>
            <Input
              id="alert-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：API 延迟过高"
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="alert-metric">指标名称</Label>
              <Input
                id="alert-metric"
                list="alert-metric-names"
                value={metric}
                onChange={(e) => setMetric(e.target.value)}
                placeholder="http.server.duration"
                className="font-mono text-xs"
              />
              <datalist id="alert-metric-names">
                {metricNames.map((name) => (
                  <option key={name} value={name} />
                ))}
              </datalist>
            </div>
            <div className="space-y-2">
              <Label htmlFor="alert-service">服务（可选）</Label>
              <Input
                id="alert-service"
                value={service}
                onChange={(e) => setService(e.target.value)}
                placeholder="留空表示全部服务"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div className="space-y-2">
              <Label>聚合</Label>
              <Select value={aggregation} onValueChange={(v) => setAggregation(v as AlertRule["aggregation"])}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="avg">平均</SelectItem>
                  <SelectItem value="max">最大</SelectItem>
                  <SelectItem value="min">最小</SelectItem>
                  <SelectItem value="sum">总和</SelectItem>
                  <SelectItem value="last">最新</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>比较</Label>
              <Select value={operator} onValueChange={(v) => setOperator(v as AlertRule["operator"])}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="gt">&gt;</SelectItem>
                  <SelectItem value="gte">≥</SelectItem>
                  <SelectItem value="lt">&lt;</SelectItem>
                  <SelectItem value="lte">≤</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="alert-threshold">阈值</Label>
              <Input
                id="alert-threshold"
                type="number"
                step="any"
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
                className="font-mono"
              />
            </div>
            <div className="space-y-2">
              <Label>窗口</Label>
              <Select value={windowSeconds} onValueChange={setWindowSeconds}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="60">1 分钟</SelectItem>
                  <SelectItem value="300">5 分钟</SelectItem>
                  <SelectItem value="900">15 分钟</SelectItem>
                  <SelectItem value="3600">1 小时</SelectItem>
                  <SelectItem value="21600">6 小时</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label>通知渠道</Label>
            {channels.length === 0 ? (
              <p className="text-muted-foreground text-[11px]">
                还没有通知渠道。先在「通知渠道」中创建 Webhook 或邮件渠道，再回到这里绑定。
              </p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {channels.map((channel) => (
                  <label
                    key={channel.id}
                    className={cn(
                      "flex cursor-pointer items-center gap-1.5 rounded-lg px-2 py-1 text-[11px] transition-colors",
                      channelIds.includes(channel.id) ? "bg-primary/10 text-primary" : "bg-muted/40 text-muted-foreground",
                    )}
                  >
                    <input
                      type="checkbox"
                      className="accent-primary size-3"
                      checked={channelIds.includes(channel.id)}
                      onChange={(e) =>
                        setChannelIds((prev) =>
                          e.target.checked ? [...prev, channel.id] : prev.filter((id) => id !== channel.id),
                        )
                      }
                    />
                    {channel.type === "email" ? <Mail className="size-3" /> : <Webhook className="size-3" />}
                    {channel.name}
                    {!channel.enabled ? <span>（已暂停）</span> : null}
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="w-40 space-y-2">
              <Label>级别</Label>
              <Select value={severity} onValueChange={(v) => setSeverity(v as AlertRule["severity"])}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="warning">Warning</SelectItem>
                  <SelectItem value="critical">Critical</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <Switch checked={enabled} onCheckedChange={setEnabled} /> 启用
            </label>
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

function NotificationChannelsDialog({
  channels,
  smtpConfigured,
  onClose,
  onChanged,
}: {
  channels: NotificationChannel[]
  smtpConfigured: boolean
  onClose: () => void
  onChanged: () => Promise<void>
}) {
  const [editing, setEditing] = React.useState<NotificationChannel | "new" | null>(null)
  const [deliveries, setDeliveries] = React.useState<NotificationDelivery[]>([])
  const [loading, setLoading] = React.useState(true)

  const loadDeliveries = React.useCallback(async () => {
    setLoading(true)
    try {
      const res = await api.notificationDeliveries({ limit: 20 })
      setDeliveries(res.deliveries ?? [])
    } catch {
      setDeliveries([])
    } finally {
      setLoading(false)
    }
  }, [])

  React.useEffect(() => {
    void loadDeliveries()
  }, [loadDeliveries])

  const toggle = async (channel: NotificationChannel, enabled: boolean) => {
    try {
      await api.updateNotificationChannel(channel.id, { enabled })
      await onChanged()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新失败")
    }
  }

  const remove = async (channel: NotificationChannel) => {
    if (!window.confirm(`删除通知渠道「${channel.name}」？已绑定该渠道的告警规则会自动解除绑定。`)) return
    try {
      await api.deleteNotificationChannel(channel.id)
      toast.success("通知渠道已删除")
      await onChanged()
      await loadDeliveries()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        {editing ? (
          <ChannelForm
            key={editing === "new" ? "new" : editing.id}
            channel={editing === "new" ? null : editing}
            smtpConfigured={smtpConfigured}
            onCancel={() => setEditing(null)}
            onSaved={async () => {
              setEditing(null)
              await onChanged()
              await loadDeliveries()
            }}
          />
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <Send className="size-4" /> 通知渠道
              </DialogTitle>
              <DialogDescription>
                告警触发和恢复时，Catch 会异步投递通知；失败自动重试。Webhook 支持 HMAC-SHA256 签名。
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-muted-foreground text-xs">
                租户级渠道，可在告警规则中多选绑定。{!smtpConfigured ? "当前未配置 SMTP，邮件渠道会投递失败。" : null}
              </p>
              <Button size="sm" className="self-start sm:self-auto" onClick={() => setEditing("new")}>
                <Plus className="size-4" /> 新建渠道
              </Button>
            </div>

            {channels.length === 0 ? (
              <div className="bg-muted/30 text-muted-foreground rounded-xl px-4 py-8 text-center text-sm">
                还没有通知渠道。创建 Webhook 或邮件渠道后，在规则中绑定即可。
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                {channels.map((channel) => (
                  <Card key={channel.id} className="gap-0">
                    <CardContent className="space-y-3 p-3">
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <p className="flex items-center gap-1.5 text-sm font-medium">
                            {channel.type === "email" ? <Mail className="size-3.5" /> : <Webhook className="size-3.5" />}
                            {channel.name}
                            <Badge variant="outline" className="text-[10px] uppercase">
                              {channel.type}
                            </Badge>
                          </p>
                          <p className="text-muted-foreground mt-1 truncate font-mono text-[11px]">
                            {channel.type === "email"
                              ? (channel.recipients ?? []).join(", ")
                              : channel.webhookUrl}
                          </p>
                          {channel.type === "webhook" && channel.secretSet ? (
                            <p className="text-muted-foreground mt-1 text-[10px]">已配置签名密钥</p>
                          ) : null}
                        </div>
                        <Switch
                          checked={channel.enabled}
                          onCheckedChange={(checked) => void toggle(channel, checked)}
                          aria-label="启用通知渠道"
                        />
                      </div>
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => setEditing(channel)}>
                          <Pencil className="size-3.5" /> 编辑
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => void remove(channel)}
                        >
                          <Trash2 className="size-3.5" /> 删除
                        </Button>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}

            <section className="space-y-2">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-semibold">最近投递</h3>
                <span className="text-muted-foreground text-xs">{deliveries.length} 条</span>
              </div>
              {loading ? (
                <Skeleton className="h-20 rounded-lg" />
              ) : deliveries.length === 0 ? (
                <p className="text-muted-foreground text-xs">还没有通知投递记录。</p>
              ) : (
                <div className="scrollbar-thin max-h-52 space-y-1.5 overflow-auto">
                  {deliveries.map((delivery) => (
                    <div
                      key={delivery.id}
                      className="bg-muted/25 flex flex-col gap-1 rounded-lg px-2.5 py-2 text-[11px] sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="flex min-w-0 items-center gap-2">
                        <span
                          className={cn(
                            "size-1.5 shrink-0 rounded-full",
                            delivery.status === "sent"
                              ? "bg-emerald-500"
                              : delivery.status === "failed"
                                ? "bg-red-500"
                                : "bg-amber-500",
                          )}
                        />
                        <span className="truncate">
                          {delivery.channelName} · {delivery.event} · 尝试 {delivery.attempts} 次
                        </span>
                        {delivery.error ? (
                          <span className="text-destructive truncate" title={delivery.error}>
                            {delivery.error}
                          </span>
                        ) : null}
                      </div>
                      <span className="text-muted-foreground shrink-0">{relativeTime(delivery.createdAt)}</span>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function ChannelForm({
  channel,
  smtpConfigured,
  onCancel,
  onSaved,
}: {
  channel: NotificationChannel | null
  smtpConfigured: boolean
  onCancel: () => void
  onSaved: () => Promise<void>
}) {
  const [name, setName] = React.useState(channel?.name ?? "")
  const [type, setType] = React.useState<"webhook" | "email">(channel?.type ?? "webhook")
  const [enabled, setEnabled] = React.useState(channel?.enabled ?? true)
  const [webhookUrl, setWebhookUrl] = React.useState(channel?.webhookUrl ?? "")
  const [secret, setSecret] = React.useState("")
  const [recipients, setRecipients] = React.useState((channel?.recipients ?? []).join(", "))
  const [busy, setBusy] = React.useState(false)
  const [testing, setTesting] = React.useState(false)

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) {
      toast.error("请填写渠道名称")
      return
    }
    setBusy(true)
    try {
      const body = {
        name: name.trim(),
        type,
        enabled,
        webhookUrl: type === "webhook" ? webhookUrl.trim() : "",
        recipients:
          type === "email"
            ? recipients
                .split(/[,\n]/)
                .map((item) => item.trim())
                .filter(Boolean)
            : [],
        ...(secret.trim() ? { secret } : {}),
      }
      if (channel) {
        await api.updateNotificationChannel(channel.id, body)
        toast.success("通知渠道已更新")
      } else {
        await api.createNotificationChannel(body)
        toast.success("通知渠道已创建")
      }
      await onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  const test = async () => {
    if (!channel) return
    setTesting(true)
    try {
      const res = await api.testNotificationChannel(channel.id)
      toast.success(`测试通知已发送${res.statusCode ? ` (${res.statusCode})` : ""}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "测试投递失败")
    } finally {
      setTesting(false)
    }
  }

  return (
    <form onSubmit={save} className="space-y-4">
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          {type === "email" ? <Mail className="size-4" /> : <Webhook className="size-4" />}
          {channel ? "编辑通知渠道" : "新建通知渠道"}
        </DialogTitle>
        <DialogDescription>
          {type === "email"
            ? "告警触发/恢复时发送纯文本邮件。"
            : "以 JSON POST 推送告警，可通过签名头校验来源。"}
        </DialogDescription>
      </DialogHeader>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="channel-name">名称</Label>
          <Input
            id="channel-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例如：值班 Webhook"
          />
        </div>
        <div className="space-y-2">
          <Label>类型</Label>
          <Select value={type} onValueChange={(value) => setType(value as "webhook" | "email")}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="webhook">Webhook</SelectItem>
              <SelectItem value="email">邮件</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {type === "webhook" ? (
        <>
          <div className="space-y-2">
            <Label htmlFor="channel-webhook">Webhook URL</Label>
            <Input
              id="channel-webhook"
              value={webhookUrl}
              onChange={(e) => setWebhookUrl(e.target.value)}
              placeholder="https://example.com/hooks/catch"
              className="font-mono text-xs"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="channel-secret">签名密钥（可选）</Label>
            <Input
              id="channel-secret"
              type="password"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder={channel?.secretSet ? "留空不修改；输入新值会覆盖" : "用于 X-Catch-Signature HMAC-SHA256"}
            />
          </div>
        </>
      ) : (
        <div className="space-y-2">
          <Label htmlFor="channel-recipients">收件人</Label>
          <Input
            id="channel-recipients"
            value={recipients}
            onChange={(e) => setRecipients(e.target.value)}
            placeholder="oncall@example.com, ops@example.com"
          />
          {!smtpConfigured ? (
            <p className="text-amber-600 text-[11px] dark:text-amber-400">
              服务端未配置 SMTP，保存后测试或真实投递会失败。请设置 SMTP_HOST / SMTP_PORT / SMTP_FROM。
            </p>
          ) : null}
        </div>
      )}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <label className="flex items-center gap-2 text-sm">
          <Switch checked={enabled} onCheckedChange={setEnabled} /> 启用
        </label>
        {channel ? (
          <Button type="button" variant="outline" size="sm" onClick={() => void test()} disabled={testing || busy}>
            {testing ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />} 发送测试通知
          </Button>
        ) : null}
      </div>

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onCancel} disabled={busy}>
          取消
        </Button>
        <Button type="submit" disabled={busy}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : null} 保存
        </Button>
      </DialogFooter>
    </form>
  )
}

function toLocalInput(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function silenceStatus(silence: AlertSilence): "active" | "upcoming" | "expired" {
  const now = Date.now()
  if (new Date(silence.endsAt).getTime() <= now) return "expired"
  if (new Date(silence.startsAt).getTime() > now) return "upcoming"
  return "active"
}

function SilencesDialog({
  silences,
  projectId,
  projects,
  rules,
  onClose,
  onChanged,
}: {
  silences: AlertSilence[]
  projectId?: string
  projects: Project[]
  rules: AlertRule[]
  onClose: () => void
  onChanged: () => Promise<void>
}) {
  const [creating, setCreating] = React.useState(false)

  const endNow = async (silence: AlertSilence) => {
    try {
      await api.updateAlertSilence(silence.id, { endsAt: new Date().toISOString() })
      toast.success("维护窗口已结束")
      await onChanged()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "结束失败")
    }
  }

  const remove = async (silence: AlertSilence) => {
    if (!window.confirm(`删除静默窗口「${silence.name}」？`)) return
    try {
      await api.deleteAlertSilence(silence.id)
      toast.success("静默窗口已删除")
      await onChanged()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败")
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        {creating ? (
          <SilenceForm
            projectId={projectId}
            projects={projects}
            rules={rules}
            onCancel={() => setCreating(false)}
            onSaved={async () => {
              setCreating(false)
              await onChanged()
            }}
          />
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <CalendarClock className="size-4" /> 静默 / 维护窗口
              </DialogTitle>
              <DialogDescription>
                在窗口内不创建新的 firing 告警，也不发送 firing/resolved 通知；窗口结束后会按当前指标重新评估。
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-center justify-between gap-2">
              <p className="text-muted-foreground text-xs">可按项目或指定规则缩小范围；留空规则表示整个项目。</p>
              <Button size="sm" onClick={() => setCreating(true)}>
                <Plus className="size-4" /> 新建窗口
              </Button>
            </div>

            {silences.length === 0 ? (
              <div className="bg-muted/30 text-muted-foreground rounded-xl px-4 py-8 text-center text-sm">
                还没有静默窗口。发布、迁移或值班交接时可以临时创建。
              </div>
            ) : (
              <div className="space-y-2">
                {silences.map((silence) => {
                  const status = silenceStatus(silence)
                  const projectName = silence.projectId
                    ? projects.find((project) => project.id === silence.projectId)?.name || "未知项目"
                    : "全部项目"
                  return (
                    <div key={silence.id} className="bg-muted/25 space-y-2 rounded-xl px-3 py-3">
                      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                        <div className="min-w-0">
                          <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                            {silence.name}
                            <Badge
                              variant={status === "active" ? "default" : "secondary"}
                              className="text-[10px]"
                            >
                              {status === "active" ? "生效中" : status === "upcoming" ? "未开始" : "已结束"}
                            </Badge>
                          </p>
                          <p className="text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px]">
                            <span>{projectName}</span>
                            <span>
                              {formatFullDate(silence.startsAt)} → {formatFullDate(silence.endsAt)}
                            </span>
                            <span>
                              {silence.ruleIds?.length
                                ? `${silence.ruleIds.length} 条规则`
                                : "全部规则"}
                            </span>
                          </p>
                          {silence.reason ? (
                            <p className="text-muted-foreground mt-1 line-clamp-2 text-[11px]">{silence.reason}</p>
                          ) : null}
                        </div>
                        <div className="flex shrink-0 gap-1">
                          {status === "active" ? (
                            <Button variant="ghost" size="sm" onClick={() => void endNow(silence)}>
                              立即结束
                            </Button>
                          ) : null}
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => void remove(silence)}
                          >
                            <Trash2 className="size-3.5" /> 删除
                          </Button>
                        </div>
                      </div>
                      {silence.ruleIds?.length ? (
                        <p className="text-muted-foreground text-[10px]">
                          指定规则：
                          {silence.ruleIds
                            .map((id) => rules.find((rule) => rule.id === id)?.name || id)
                            .join("、")}
                        </p>
                      ) : null}
                    </div>
                  )
                })}
              </div>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function SilenceForm({
  projectId,
  projects,
  rules,
  onCancel,
  onSaved,
}: {
  projectId?: string
  projects: Project[]
  rules: AlertRule[]
  onCancel: () => void
  onSaved: () => Promise<void>
}) {
  const now = new Date()
  const [name, setName] = React.useState("")
  const [reason, setReason] = React.useState("")
  const [scope, setScope] = React.useState(projectId || "all")
  const [startsAt, setStartsAt] = React.useState(toLocalInput(now))
  const [endsAt, setEndsAt] = React.useState(toLocalInput(new Date(now.getTime() + 60 * 60 * 1000)))
  const [ruleIds, setRuleIds] = React.useState<string[]>([])
  const [busy, setBusy] = React.useState(false)

  const eligibleRules = scope === "all" ? rules : rules.filter((rule) => rule.projectId === scope)

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) {
      toast.error("请填写静默名称")
      return
    }
    const start = new Date(startsAt)
    const end = new Date(endsAt)
    if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime())) {
      toast.error("请填写有效的开始和结束时间")
      return
    }
    if (end <= start) {
      toast.error("结束时间必须晚于开始时间")
      return
    }
    if (end <= new Date()) {
      toast.error("结束时间必须晚于当前时间")
      return
    }
    setBusy(true)
    try {
      await api.createAlertSilence({
        name: name.trim(),
        reason: reason.trim(),
        projectId: scope === "all" ? undefined : scope,
        ruleIds,
        startsAt: start.toISOString(),
        endsAt: end.toISOString(),
      })
      toast.success("静默窗口已创建")
      await onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={save} className="space-y-4">
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <CalendarClock className="size-4" /> 新建静默窗口
        </DialogTitle>
        <DialogDescription>
          窗口内评估仍会更新规则状态，但不会产生新的 firing 告警或通知。
        </DialogDescription>
      </DialogHeader>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="silence-name">名称</Label>
          <Input
            id="silence-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例如：订单服务发布"
          />
        </div>
        <div className="space-y-2">
          <Label>作用范围</Label>
          <Select
            value={scope}
            onValueChange={(value) => {
              setScope(value)
              setRuleIds([])
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部项目</SelectItem>
              {projects.map((project) => (
                <SelectItem key={project.id} value={project.id}>
                  {project.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="silence-reason">原因（可选）</Label>
        <Input
          id="silence-reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="例如：数据库迁移，预计 30 分钟"
        />
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="silence-start">开始时间</Label>
          <Input
            id="silence-start"
            type="datetime-local"
            value={startsAt}
            onChange={(e) => setStartsAt(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="silence-end">结束时间</Label>
          <Input
            id="silence-end"
            type="datetime-local"
            value={endsAt}
            onChange={(e) => setEndsAt(e.target.value)}
          />
        </div>
      </div>

      <div className="space-y-2">
        <Label>指定规则（可选）</Label>
        {eligibleRules.length === 0 ? (
          <p className="text-muted-foreground text-[11px]">当前范围下没有可选的告警规则。</p>
        ) : (
          <div className="scrollbar-thin flex max-h-32 flex-wrap gap-1.5 overflow-auto">
            {eligibleRules.map((rule) => (
              <label
                key={rule.id}
                className={cn(
                  "flex cursor-pointer items-center gap-1.5 rounded-lg px-2 py-1 text-[11px] transition-colors",
                  ruleIds.includes(rule.id) ? "bg-primary/10 text-primary" : "bg-muted/40 text-muted-foreground",
                )}
              >
                <input
                  type="checkbox"
                  className="accent-primary size-3"
                  checked={ruleIds.includes(rule.id)}
                  onChange={(e) =>
                    setRuleIds((prev) =>
                      e.target.checked ? [...prev, rule.id] : prev.filter((id) => id !== rule.id),
                    )
                  }
                />
                {rule.name}
              </label>
            ))}
          </div>
        )}
        <p className="text-muted-foreground text-[10px]">不选择规则表示静默该范围内的全部规则。</p>
      </div>

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onCancel} disabled={busy}>
          取消
        </Button>
        <Button type="submit" disabled={busy}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : null} 创建
        </Button>
      </DialogFooter>
    </form>
  )
}
