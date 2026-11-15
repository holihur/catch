import { AlertTriangle, RefreshCw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn, copyText } from "@/lib/utils"
import { ApiError } from "@/lib/api"
import { toast } from "sonner"

export function ErrorState({
  error,
  onRetry,
  className,
}: {
  error: unknown
  onRetry?: () => void
  className?: string
}) {
  const message = error instanceof Error ? error.message : String(error)
  const requestId = error instanceof ApiError ? error.requestId : undefined
  return (
    <div
      className={cn(
        "border-destructive/20 bg-destructive/5 flex flex-col items-center gap-3 rounded-xl border px-6 py-12 text-center",
        className,
      )}
    >
      <div className="bg-destructive/10 text-destructive flex size-11 items-center justify-center rounded-full">
        <AlertTriangle className="size-5" />
      </div>
      <div className="space-y-1">
        <p className="font-medium">加载失败</p>
        <p className="text-muted-foreground max-w-lg text-sm">{message}</p>
        {requestId ? (
          <button
            className="text-muted-foreground hover:text-foreground font-mono text-xs underline-offset-2 hover:underline"
            onClick={() => {
              void copyText(requestId)
              toast.success("已复制 Request ID")
            }}
          >
            Request ID: {requestId}
          </button>
        ) : null}
      </div>
      {onRetry ? (
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw className="size-4" /> 重试
        </Button>
      ) : null}
    </div>
  )
}
