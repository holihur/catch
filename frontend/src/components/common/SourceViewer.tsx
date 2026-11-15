import * as React from "react"
import { AlertCircle, ExternalLink, FileCode2 } from "lucide-react"

import { api } from "@/lib/api"
import type { SourceFrame } from "@/lib/source"
import { cn, copyText } from "@/lib/utils"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"

export function SourceViewerDialog({
  frame,
  onClose,
}: {
  frame: SourceFrame | null
  onClose: () => void
}) {
  const [content, setContent] = React.useState("")
  const [resolvedPath, setResolvedPath] = React.useState("")
  const [language, setLanguage] = React.useState("")
  const [truncated, setTruncated] = React.useState(false)
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const scrollRef = React.useRef<HTMLDivElement>(null)
  const targetRef = React.useRef<HTMLDivElement>(null)

  React.useEffect(() => {
    if (!frame) {
      setContent("")
      setError(null)
      return
    }
    let active = true
    setContent("")
    setError(null)
    setTruncated(false)
    setLanguage("")
    setResolvedPath("")
    if (!frame.release) {
      setError("该事件没有 release，无法从 Source Map 读取内嵌源码。")
      return
    }
    setLoading(true)
    api
      .sourceFile(frame.project.id, frame.release, frame.path, frame.line)
      .then((res) => {
        if (!active) return
        setContent(res.content)
        setResolvedPath(res.path)
        setLanguage(res.language)
        setTruncated(res.truncated)
      })
      .catch((err) => {
        if (!active) return
        setError(err instanceof Error ? err.message : "无法读取源码")
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [frame])

  const lines = React.useMemo(() => content.split("\n"), [content])
  const targetLine = frame?.line ?? 1

  React.useEffect(() => {
    if (!content || !scrollRef.current || !targetRef.current) return
    scrollRef.current.scrollTop = Math.max(0, targetRef.current.offsetTop - 120)
  }, [content, targetLine])

  return (
    <Dialog open={Boolean(frame)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] gap-3 overflow-hidden sm:max-w-4xl">
        {frame ? (
          <>
            <DialogHeader className="text-left">
              <DialogTitle className="flex items-center gap-2 pr-10 text-base">
                <FileCode2 className="size-4 shrink-0" /> 源码
                <Badge variant="outline" className="ml-auto font-mono text-[10px]">
                  {frame.release || "unknown release"}
                </Badge>
              </DialogTitle>
              <DialogDescription className="break-all font-mono text-[11px]">
                {resolvedPath || frame.path}:{frame.line}
                {frame.column ? `:${frame.column}` : ""}
                {language ? ` · ${language}` : ""}
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-wrap items-center justify-end gap-2">
              {content ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void copyText(content)}
                >
                  复制代码
                </Button>
              ) : null}
              {frame.href ? (
                <Button asChild variant="outline" size="sm">
                  <a href={frame.href} target="_blank" rel="noreferrer noopener">
                    <ExternalLink className="size-3.5" /> 在仓库打开
                  </a>
                </Button>
              ) : null}
            </div>

            {loading ? (
              <div className="space-y-2 py-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-4 w-full" />
                ))}
              </div>
            ) : error ? (
              <div className="bg-muted/30 flex flex-col items-center gap-3 rounded-xl px-6 py-10 text-center">
                <div className="bg-warning/10 text-warning flex size-10 items-center justify-center rounded-full">
                  <AlertCircle className="size-5" />
                </div>
                <div className="space-y-1">
                  <p className="text-sm font-medium">无法直接查看源码</p>
                  <p className="text-muted-foreground max-w-md text-xs">{error}</p>
                </div>
                {frame.href ? (
                  <Button asChild size="sm" variant="outline">
                    <a href={frame.href} target="_blank" rel="noreferrer noopener">
                      在仓库打开 <ExternalLink className="size-3.5" />
                    </a>
                  </Button>
                ) : null}
              </div>
            ) : content ? (
              <div className="bg-muted/40 scrollbar-thin max-h-[62vh] overflow-auto rounded-xl">
                <div ref={scrollRef} className="max-h-[62vh] overflow-auto">
                  <div className="py-2">
                    {lines.map((line, i) => {
                      const lineNo = i + 1
                      const active = lineNo === targetLine
                      return (
                        <div
                          key={lineNo}
                          ref={active ? targetRef : undefined}
                          className={cn(
                            "flex min-w-max px-3 font-mono text-[11px] leading-relaxed sm:text-xs",
                            active && "bg-primary/10 text-foreground",
                          )}
                        >
                          <span
                            className={cn(
                              "text-muted-foreground mr-3 inline-block w-8 shrink-0 text-right select-none sm:mr-4 sm:w-10",
                              active && "text-primary font-semibold",
                            )}
                          >
                            {lineNo}
                          </span>
                          <span className="whitespace-pre pr-6">{line || " "}</span>
                        </div>
                      )
                    })}
                  </div>
                </div>
              </div>
            ) : (
              <p className="text-muted-foreground py-8 text-center text-sm">源码内容为空</p>
            )}

            {truncated ? (
              <p className="text-muted-foreground text-[10px]">
                文件较大，已截断显示；请前往仓库查看完整内容。
              </p>
            ) : null}
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
