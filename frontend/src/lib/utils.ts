import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatNumber(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return "0"
  if (Math.abs(n) >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (Math.abs(n) >= 10_000) return `${(n / 1_000).toFixed(1)}k`
  return new Intl.NumberFormat("zh-CN").format(n)
}

export function formatPercent(value: number, digits = 1): string {
  return `${(value * 100).toFixed(digits)}%`
}

export function formatDate(value: string | Date | undefined | null): string {
  if (!value) return "—"
  const d = typeof value === "string" ? new Date(value) : value
  if (Number.isNaN(d.getTime())) return "—"
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(d)
}

export function formatFullDate(value: string | Date | undefined | null): string {
  if (!value) return "—"
  const d = typeof value === "string" ? new Date(value) : value
  if (Number.isNaN(d.getTime())) return "—"
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(d)
}

const RTF = new Intl.RelativeTimeFormat("zh-CN", { numeric: "auto" })

export function relativeTime(value: string | Date | undefined | null): string {
  if (!value) return "—"
  const d = typeof value === "string" ? new Date(value) : value
  const diff = d.getTime() - Date.now()
  const abs = Math.abs(diff)
  if (abs < 60_000) return "刚刚"
  if (abs < 3_600_000) return RTF.format(Math.round(diff / 60_000), "minute")
  if (abs < 86_400_000) return RTF.format(Math.round(diff / 3_600_000), "hour")
  if (abs < 30 * 86_400_000) return RTF.format(Math.round(diff / 86_400_000), "day")
  return formatDate(d)
}

export function humanDuration(seconds: number): string {
  if (!seconds || seconds <= 0) return "—"
  if (seconds < 60) return `${Math.round(seconds)} 秒`
  if (seconds < 3600) return `${(seconds / 60).toFixed(1)} 分钟`
  if (seconds < 86400) return `${(seconds / 3600).toFixed(1)} 小时`
  return `${(seconds / 86400).toFixed(1)} 天`
}

export function shortId(id: string): string {
  return id?.slice(-6) || ""
}

export const levelMeta: Record<string, { label: string; className: string; dot: string }> = {
  fatal: {
    label: "Fatal",
    className: "border-destructive/40 bg-destructive/10 text-destructive",
    dot: "bg-destructive",
  },
  error: {
    label: "Error",
    className: "border-destructive/30 bg-destructive/10 text-destructive",
    dot: "bg-destructive",
  },
  warn: {
    label: "Warn",
    className: "border-warning/40 bg-warning/15 text-warning-foreground dark:text-warning",
    dot: "bg-warning",
  },
  info: {
    label: "Info",
    className: "border-chart-2/40 bg-chart-2/10 text-chart-2",
    dot: "bg-chart-2",
  },
  debug: {
    label: "Debug",
    className: "border-border bg-muted text-muted-foreground",
    dot: "bg-muted-foreground",
  },
}

export function levelClass(level: string): string {
  return levelMeta[level]?.className ?? levelMeta.info.className
}

export const statusMeta: Record<string, { label: string; className: string }> = {
  open: { label: "待处理", className: "border-warning/40 bg-warning/15 text-warning-foreground dark:text-warning" },
  resolved: { label: "已解决", className: "border-success/40 bg-success/10 text-success" },
  ignored: { label: "已忽略", className: "border-border bg-muted text-muted-foreground" },
}

export const sourceMeta: Record<string, { label: string; className: string }> = {
  frontend: { label: "前端", className: "border-chart-2/40 bg-chart-2/10 text-chart-2" },
  backend: { label: "后端", className: "border-chart-1/40 bg-chart-1/10 text-chart-1" },
}

export function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(text)
  }
  const el = document.createElement("textarea")
  el.value = text
  document.body.appendChild(el)
  el.select()
  document.execCommand("copy")
  document.body.removeChild(el)
  return Promise.resolve()
}

export function truncate(s: string, n = 120): string {
  return s.length > n ? `${s.slice(0, n)}…` : s
}
