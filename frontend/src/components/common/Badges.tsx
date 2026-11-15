import { cn, levelMeta, sourceMeta, statusMeta } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"

/**
 * Compact severity marker. Color is the primary signal in dense lists; the
 * accessible name and native tooltip keep the level available to screen
 * readers and on hover.
 */
export function LevelMark({ level, className }: { level: string; className?: string }) {
  const meta = levelMeta[level] ?? levelMeta.info
  return (
    <span
      role="img"
      aria-label={`${meta.label} 级别`}
      title={`${meta.label} 级别`}
      className={cn("inline-flex size-3 items-center justify-center", className)}
    >
      <span
        className={cn(
          "size-2.5 shrink-0 rounded-full",
          meta.dot,
          level === "fatal" && "ring-2 ring-destructive/25",
        )}
      />
    </span>
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
