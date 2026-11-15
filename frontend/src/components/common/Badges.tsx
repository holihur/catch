import { cn, levelClass, levelMeta, sourceMeta, statusMeta } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"

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
