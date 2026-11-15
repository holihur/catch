import * as React from "react"
import { ArrowDownRight, ArrowUpRight, Minus } from "lucide-react"

import { cn, formatNumber } from "@/lib/utils"
import { Card, CardContent } from "@/components/ui/card"

export function StatCard({
  label,
  value,
  icon,
  hint,
  delta,
  deltaLabel,
  invertDelta = false,
  accent = "primary",
}: {
  label: string
  value: number | string
  icon: React.ReactNode
  hint?: React.ReactNode
  delta?: number
  deltaLabel?: string
  invertDelta?: boolean
  accent?: "primary" | "danger" | "success" | "warning" | "info"
}) {
  const accentClass: Record<string, string> = {
    primary: "bg-primary/10 text-primary",
    danger: "bg-destructive/10 text-destructive",
    success: "bg-success/10 text-success",
    warning: "bg-warning/15 text-warning-foreground dark:text-warning",
    info: "bg-chart-2/10 text-chart-2",
  }
  const positive = delta !== undefined ? (invertDelta ? delta < 0 : delta > 0) : null
  return (
    <Card className="card-hover gap-0 py-0">
      <CardContent className="flex flex-col gap-4 p-5">
        <div className="flex items-start justify-between gap-3">
          <span className="text-muted-foreground text-sm font-medium">{label}</span>
          <span className={cn("flex size-8 items-center justify-center rounded-lg", accentClass[accent])}>
            {icon}
          </span>
        </div>
        <div className="flex items-end justify-between gap-2">
          <span className="text-2xl font-semibold tracking-tight tabular-nums">
            {typeof value === "number" ? formatNumber(value) : value}
          </span>
          {delta !== undefined ? (
            <span
              className={cn(
                "flex items-center gap-0.5 text-xs font-medium tabular-nums",
                positive === null
                  ? "text-muted-foreground"
                  : positive
                    ? "text-success"
                    : "text-destructive",
              )}
            >
              {delta === 0 ? (
                <Minus className="size-3" />
              ) : delta > 0 ? (
                <ArrowUpRight className="size-3" />
              ) : (
                <ArrowDownRight className="size-3" />
              )}
              {Math.abs(delta).toFixed(1)}%
              {deltaLabel ? <span className="text-muted-foreground ml-1">{deltaLabel}</span> : null}
            </span>
          ) : null}
        </div>
        {hint ? <div className="text-muted-foreground text-xs">{hint}</div> : null}
      </CardContent>
    </Card>
  )
}
