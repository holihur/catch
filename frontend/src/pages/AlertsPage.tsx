import * as React from "react"
import {
  AlertTriangle,
  Bell,
  BellRing,
  CheckCircle2,
  Loader2,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import { api } from "@/lib/api"
import type { AlertItem, AlertRule } from "@/lib/types"
import { useAuth } from "@/context/AuthContext"
import { useProjects } from "@/context/ProjectContext"
import { cn, formatNumber, relativeTime } from "@/lib/utils"
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
  const { selected } = useProjects()
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
  const projectId = selected?.id

  const load = React.useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [ruleRes, alertRes] = await Promise.all([
        api.alertRules(projectId),
        api.alerts({ projectId, status }),
      ])
      setRules(ruleRes.rules ?? [])
      setAlerts(alertRes.alerts ?? [])
      setFiringTotal(alertRes.firing ?? 0)
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

  return (
    <div className="space-y-6">
      <PageHeader
        title="告警"
        description="基于 OTLP 指标持续评估阈值，异常时创建 firing 告警，恢复后自动标记 resolved。"
        actions={
          canManage ? (
            <Button size="sm" disabled={!projectId} onClick={() => setEditing("new")}>
              <Plus className="size-4" /> 新建规则
            </Button>
          ) : null
        }
      />

      {!projectId && canManage ? (
        <p className="text-muted-foreground text-xs">请先在顶部选择一个项目，再创建告警规则。</p>
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
                          {rule.state === "firing" ? "Firing" : rule.state === "ok" ? "正常" : rule.state === "nodata" ? "无数据" : "待评估"}
                        </span>
                        <span>窗口 {windowLabel(rule.windowSeconds)}</span>
                        {rule.service ? <span>服务 {rule.service}</span> : <span>全部服务</span>}
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
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null)
            await load()
          }}
        />
      ) : null}
    </div>
  )
}

function RuleDialog({
  rule,
  projectId,
  metricNames,
  onClose,
  onSaved,
}: {
  rule: AlertRule | null
  projectId?: string
  metricNames: string[]
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
