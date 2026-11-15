import * as React from "react"
import { ChevronDown, ChevronRight, ExternalLink } from "lucide-react"

import { cn } from "@/lib/utils"
import type { Project } from "@/lib/types"
import { buildSourceUrl, firstSourceUrl, parseSourceLocation } from "@/lib/source"
import { CopyButton } from "./CopyButton"

export function StackTrace({
  stack,
  project,
  className,
}: {
  stack?: string
  project?: Project | null
  className?: string
}) {
  const [expanded, setExpanded] = React.useState(true)
  if (!stack) {
    return <p className="text-muted-foreground text-sm">没有可用的堆栈信息</p>
  }
  const lines = stack.split("\n")
  const visible = expanded ? lines : lines.slice(0, 3)
  const firstSource = firstSourceUrl(project, [stack])
  return (
    <div className={cn("bg-muted/40 rounded-xl", className)}>
      <div className="flex items-center justify-between gap-2 px-3 py-2">
        <span className="text-muted-foreground text-xs font-medium">Stack trace</span>
        <div className="flex items-center gap-1">
          {firstSource ? (
            <a
              href={firstSource}
              target="_blank"
              rel="noreferrer noopener"
              className="text-primary hover:bg-primary/10 flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs font-medium transition-colors"
            >
              打开源码 <ExternalLink className="size-3" />
            </a>
          ) : null}
          <CopyButton value={stack} label="复制堆栈" />
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs"
            onClick={() => setExpanded((v) => !v)}
          >
            {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
            {expanded ? "收起" : `展开 ${lines.length} 行`}
          </button>
        </div>
      </div>
      <div className="scrollbar-thin max-h-[420px] overflow-auto">
        <pre className="p-3 text-xs leading-relaxed">
          {visible.map((line, i) => {
            const isFrame = /^\s+at\s|^\s+/.test(line)
            const isError = i === 0
            const location = parseSourceLocation(line)
            const href = location
              ? buildSourceUrl(project, location.file, location.line, location.column)
              : null
            return (
              <div
                key={i}
                className={cn(
                  "flex gap-3 rounded px-1",
                  isError && "text-destructive font-medium",
                  isFrame && !isError && "text-muted-foreground",
                )}
              >
                <span className="w-6 shrink-0 text-right opacity-40 select-none">{i + 1}</span>
                <span className="min-w-0 whitespace-pre-wrap break-all">
                  {href && location ? (
                    <>
                      {line.slice(0, location.start)}
                      <a
                        href={href}
                        target="_blank"
                        rel="noreferrer noopener"
                        title="在源码仓库中打开"
                        className="text-primary decoration-primary/40 hover:decoration-primary underline-offset-2 hover:underline"
                      >
                        {line.slice(location.start, location.end)}
                      </a>
                      {line.slice(location.end)}
                    </>
                  ) : (
                    line || " "
                  )}
                </span>
              </div>
            )
          })}
          {!expanded && lines.length > 3 ? (
            <div className="text-muted-foreground px-1 pl-9">… 还有 {lines.length - 3} 行</div>
          ) : null}
        </pre>
      </div>
    </div>
  )
}
