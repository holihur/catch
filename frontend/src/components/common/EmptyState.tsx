import * as React from "react"
import { Inbox } from "lucide-react"

import { cn } from "@/lib/utils"

export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: React.ReactNode
  title: string
  description?: string
  action?: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        "bg-muted/30 flex flex-col items-center justify-center gap-3 rounded-2xl px-6 py-14 text-center",
        className,
      )}
    >
      <div className="bg-muted text-muted-foreground flex size-11 items-center justify-center rounded-full">
        {icon ?? <Inbox className="size-5" />}
      </div>
      <div className="space-y-1">
        <p className="font-medium">{title}</p>
        {description ? (
          <p className="text-muted-foreground mx-auto max-w-md text-sm">{description}</p>
        ) : null}
      </div>
      {action}
    </div>
  )
}
