import * as React from "react"

import type { SourceFrame } from "@/lib/source"
import { SourceViewerDialog } from "@/components/common/SourceViewer"

interface SourceViewerContextValue {
  openSource: (frame: SourceFrame) => void
}

const SourceViewerContext = React.createContext<SourceViewerContextValue | null>(null)

export function SourceViewerProvider({ children }: { children: React.ReactNode }) {
  const [frame, setFrame] = React.useState<SourceFrame | null>(null)
  const openSource = React.useCallback((next: SourceFrame) => setFrame(next), [])
  const value = React.useMemo(() => ({ openSource }), [openSource])
  return (
    <SourceViewerContext.Provider value={value}>
      {children}
      <SourceViewerDialog frame={frame} onClose={() => setFrame(null)} />
    </SourceViewerContext.Provider>
  )
}

export function useSourceViewer() {
  const ctx = React.useContext(SourceViewerContext)
  if (!ctx) throw new Error("useSourceViewer must be used within SourceViewerProvider")
  return ctx
}
