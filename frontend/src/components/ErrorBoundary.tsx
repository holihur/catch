import * as React from "react"
import { AlertOctagon, RefreshCw } from "lucide-react"

import { captureError } from "@/lib/telemetry"
import { Button } from "@/components/ui/button"

interface State {
  error: Error | null
}

export class ErrorBoundary extends React.Component<{ children: React.ReactNode }, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    captureError(error, {
      mechanism: "react-error-boundary",
      componentStack: info.componentStack,
    })
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center gap-5 px-6 text-center">
          <div className="bg-destructive/10 text-destructive flex size-14 items-center justify-center rounded-2xl">
            <AlertOctagon className="size-6" />
          </div>
          <div className="space-y-1.5">
            <h1 className="text-xl font-semibold">页面渲染出错</h1>
            <p className="text-muted-foreground max-w-lg text-sm">
              错误已被捕获并上报。你可以刷新页面重试，或返回概览。
            </p>
            <p className="text-muted-foreground bg-muted mx-auto mt-3 max-w-lg truncate rounded-md px-3 py-2 font-mono text-xs">
              {this.state.error.message}
            </p>
          </div>
          <div className="flex gap-2">
            <Button onClick={() => window.location.reload()}>
              <RefreshCw className="size-4" /> 刷新页面
            </Button>
            <Button variant="outline" onClick={() => (window.location.href = "/")}>
              返回概览
            </Button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
