import * as React from "react"
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Line,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

import type { IssueTrendPoint, TrendPoint } from "@/lib/types"
import { formatNumber, levelMeta } from "@/lib/utils"

interface TooltipPayload {
  name?: string
  value?: number
  color?: string
  dataKey?: string
}

function ChartTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean
  payload?: TooltipPayload[]
  label?: string
}) {
  if (!active || !payload?.length) return null
  return (
    <div className="bg-popover text-popover-foreground rounded-lg border px-3 py-2 text-xs shadow-lg">
      <p className="text-muted-foreground mb-1.5">{formatTick(String(label ?? ""))}</p>
      <div className="space-y-1">
        {payload.map((item) => (
          <div key={item.dataKey} className="flex items-center justify-between gap-4">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ background: item.color }} />
              {item.name}
            </span>
            <span className="font-mono font-medium tabular-nums">{formatNumber(item.value ?? 0)}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function formatTick(value: string): string {
  if (!value) return ""
  if (value.includes("T")) {
    const d = new Date(value)
    return Number.isNaN(d.getTime())
      ? value
      : `${String(d.getHours()).padStart(2, "0")}:00`
  }
  return value.slice(5)
}

export function ErrorTrendChart({ errors, logs }: { errors: TrendPoint[]; logs: TrendPoint[] }) {
  const data = React.useMemo(() => {
    const map = new Map<string, { t: string; errors: number; logs: number }>()
    for (const p of logs) map.set(p.t, { t: p.t, errors: 0, logs: p.count })
    for (const p of errors) {
      const cur = map.get(p.t) ?? { t: p.t, errors: 0, logs: 0 }
      cur.errors = p.count
      map.set(p.t, cur)
    }
    return Array.from(map.values()).sort((a, b) => a.t.localeCompare(b.t))
  }, [errors, logs])

  if (data.length === 0) {
    return <ChartEmpty />
  }

  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={data} margin={{ top: 8, right: 8, left: -18, bottom: 0 }}>
        <defs>
          <linearGradient id="fillErrors" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--chart-4)" stopOpacity={0.32} />
            <stop offset="95%" stopColor="var(--chart-4)" stopOpacity={0.02} />
          </linearGradient>
          <linearGradient id="fillLogs" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--chart-2)" stopOpacity={0.18} />
            <stop offset="95%" stopColor="var(--chart-2)" stopOpacity={0.01} />
          </linearGradient>
        </defs>
        <CartesianGrid stroke="var(--border)" strokeDasharray="3 4" vertical={false} />
        <XAxis
          dataKey="t"
          tickFormatter={formatTick}
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
          axisLine={false}
          tickLine={false}
          minTickGap={28}
        />
        <YAxis
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
          axisLine={false}
          tickLine={false}
          allowDecimals={false}
        />
        <Tooltip content={<ChartTooltip />} cursor={{ stroke: "var(--border)" }} />
        {logs.length > 0 ? (
          <Area
            type="monotone"
            dataKey="logs"
            name="日志"
            stroke="var(--chart-2)"
            strokeWidth={1.5}
            fill="url(#fillLogs)"
            dot={false}
          />
        ) : null}
        <Area
          type="monotone"
          dataKey="errors"
          name="错误"
          stroke="var(--chart-4)"
          strokeWidth={2}
          fill="url(#fillErrors)"
          dot={false}
          activeDot={{ r: 3 }}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}

export function IssueTrendChart({ data }: { data: IssueTrendPoint[] }) {
  if (data.length === 0) return <ChartEmpty />
  return (
    <ResponsiveContainer width="100%" height="100%">
      <BarChart data={data} margin={{ top: 8, right: 8, left: -22, bottom: 0 }} barGap={2}>
        <CartesianGrid stroke="var(--border)" strokeDasharray="3 4" vertical={false} />
        <XAxis
          dataKey="t"
          tickFormatter={formatTick}
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
          axisLine={false}
          tickLine={false}
          minTickGap={16}
        />
        <YAxis
          tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
          axisLine={false}
          tickLine={false}
          allowDecimals={false}
        />
        <Tooltip content={<ChartTooltip />} cursor={{ fill: "var(--accent)", opacity: 0.4 }} />
        <Bar dataKey="new" name="新增" fill="var(--chart-4)" radius={[3, 3, 0, 0]} maxBarSize={18} />
        <Bar dataKey="resolved" name="解决" fill="var(--chart-5)" radius={[3, 3, 0, 0]} maxBarSize={18} />
        <Line type="monotone" dataKey="new" stroke="transparent" />
      </BarChart>
    </ResponsiveContainer>
  )
}

const LEVEL_ORDER = ["fatal", "error", "warn", "info", "debug"]
const LEVEL_COLORS: Record<string, string> = {
  fatal: "var(--destructive)",
  error: "var(--chart-4)",
  warn: "var(--chart-3)",
  info: "var(--chart-2)",
  debug: "var(--muted-foreground)",
}

export function LevelDonut({ levels }: { levels: Record<string, number> }) {
  const data = LEVEL_ORDER.map((level) => ({
    name: levelMeta[level]?.label ?? level,
    key: level,
    value: levels[level] ?? 0,
  })).filter((d) => d.value > 0)
  const total = data.reduce((sum, d) => sum + d.value, 0)
  if (total === 0) return <ChartEmpty text="暂无日志" />
  return (
    <div className="flex h-full flex-col gap-3">
      <div className="relative mx-auto h-40 w-full max-w-[220px]">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={data}
              dataKey="value"
              nameKey="name"
              innerRadius={52}
              outerRadius={72}
              paddingAngle={2}
              strokeWidth={0}
            >
              {data.map((entry) => (
                <Cell key={entry.key} fill={LEVEL_COLORS[entry.key]} />
              ))}
            </Pie>
            <Tooltip content={<ChartTooltip />} />
          </PieChart>
        </ResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-xl font-semibold tabular-nums">{formatNumber(total)}</span>
          <span className="text-muted-foreground text-[10px]">日志总量</span>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1.5">
        {data.map((d) => (
          <div key={d.key} className="flex items-center justify-between gap-2 text-xs">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ background: LEVEL_COLORS[d.key] }} />
              {d.name}
            </span>
            <span className="text-muted-foreground font-mono tabular-nums">
              {formatNumber(d.value)}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

function ChartEmpty({ text = "所选范围内暂无数据" }: { text?: string }) {
  return (
    <div className="text-muted-foreground flex h-full min-h-32 items-center justify-center text-sm">
      {text}
    </div>
  )
}
