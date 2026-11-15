import * as React from "react"
import ReactDOM from "react-dom/client"
import { BrowserRouter } from "react-router-dom"
import { Toaster } from "sonner"

import { App } from "./App"
import { AuthProvider } from "./context/AuthContext"
import { ProjectProvider } from "./context/ProjectContext"
import { SourceViewerProvider } from "./context/SourceViewerContext"
import { ErrorBoundary } from "./components/ErrorBoundary"
import { initTelemetry } from "./lib/telemetry"
import "./index.css"

initTelemetry()

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <BrowserRouter>
        <AuthProvider>
          <ProjectProvider>
            <SourceViewerProvider>
              <App />
              <Toaster position="bottom-right" richColors closeButton />
            </SourceViewerProvider>
          </ProjectProvider>
        </AuthProvider>
      </BrowserRouter>
    </ErrorBoundary>
  </React.StrictMode>,
)
