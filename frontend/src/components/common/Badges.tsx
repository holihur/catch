import { cn, levelClass, levelMeta, sourceMeta, statusMeta } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"

/**
 * Compact severity marker: a colored dot plus (optionally) a short label.
 * Color is never the only signal — the label, title and aria-label remain
 * available for accessibility and color-blind users.
 */
export function LevelMark({
  level,
  label = true,
  className,
}: {
  level: string
  label?: boolean
  className?: string
}) {
  const meta = levelMeta[level] ?? levelMeta.info
  return (
    <span
      role="img"
      aria-label={`${meta.label} 级别`}
      title={`${meta.label} 级别`}
      className={cn("inline-flex items-center gap-1.5", className)}
    >
      <span className={cn("size-2 shrink-0 rounded-full", meta.dot)} />
      {label ? (
        <span className="text-muted-foreground text-[10px] font-medium tracking-wide uppercase">
          {meta.label}
        </span>
      ) : null}
    </span>
  )
}

export function LevelBadge({ level, className }: { level: string; className?: string }) {
  const meta = levelMeta[level] ?? levelMeta.info
  return (
    <Badge variant="outline" className={cn("gap-1.5 font-mono text-[10px] uppercase", levelClass(level), className)}>
      <span className={cn("size-1.5 rounded-full", meta.dot)} />
      {meta.label}
    </Badge>
  )
}

export function StatusBadge({ status, className }: { status: string; className?: string }) {
  const meta = statusMeta[status] ?? statusMeta.open
  return (
    <Badge variant="outline" className={cn("font-medium", meta.className, className)}>
      {meta.label}
    </Badge>
  )
}

export function SourceBadge({ source, className }: { source: string; className?: string }) {
  const meta = sourceMeta[source] ?? sourceMeta.frontend
  return (
    <Badge variant="outline" className={cn("font-medium", meta.className, className)}>
      {meta.label}
    </Badge>
  )
}
