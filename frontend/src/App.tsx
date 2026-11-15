import * as React from "react"
import { Link, Route, Routes, useLocation } from "react-router-dom"
import { ArrowLeft, ShieldAlert } from "lucide-react"

import { RequireAuth, GuestOnly } from "@/context/AuthContext"
import { AppShell } from "@/components/layout/AppShell"
import { Button } from "@/components/ui/button"

const LoginPage = React.lazy(() =>
  import("@/pages/LoginPage").then((m) => ({ default: m.LoginPage })),
)
const OverviewPage = React.lazy(() =>
  import("@/pages/OverviewPage").then((m) => ({ default: m.OverviewPage })),
)
const IssuesPage = React.lazy(() =>
  import("@/pages/IssuesPage").then((m) => ({ default: m.IssuesPage })),
)
const IssueDetailPage = React.lazy(() =>
  import("@/pages/IssueDetailPage").then((m) => ({ default: m.IssueDetailPage })),
)
const LogsPage = React.lazy(() =>
  import("@/pages/LogsPage").then((m) => ({ default: m.LogsPage })),
)
const TracesPage = React.lazy(() =>
  import("@/pages/TracesPage").then((m) => ({ default: m.TracesPage })),
)
const TraceDetailPage = React.lazy(() =>
  import("@/pages/TraceDetailPage").then((m) => ({ default: m.TraceDetailPage })),
)
const MetricsPage = React.lazy(() =>
  import("@/pages/MetricsPage").then((m) => ({ default: m.MetricsPage })),
)
const InsightsPage = React.lazy(() =>
  import("@/pages/InsightsPage").then((m) => ({ default: m.InsightsPage })),
)
const SettingsPage = React.lazy(() =>
  import("@/pages/SettingsPage").then((m) => ({ default: m.SettingsPage })),
)

export function App() {
  return (
    <React.Suspense fallback={<RouteFallback />}>
      <Routes>
      <Route
        path="/login"
        element={
          <GuestOnly>
            <LoginPage />
          </GuestOnly>
        }
      />
      <Route
        element={
          <RequireAuth>
            <AppShell />
          </RequireAuth>
        }
      >
        <Route index element={<OverviewPage />} />
        <Route path="issues" element={<IssuesPage />} />
        <Route path="issues/:id" element={<IssueDetailPage />} />
        <Route path="logs" element={<LogsPage />} />
        <Route path="traces" element={<TracesPage />} />
        <Route path="traces/:id" element={<TraceDetailPage />} />
        <Route path="metrics" element={<MetricsPage />} />
        <Route path="insights" element={<InsightsPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
      </Routes>
    </React.Suspense>
  )
}

function RouteFallback() {
  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <div className="text-muted-foreground flex items-center gap-2 text-sm">
        <span className="border-primary size-4 animate-spin rounded-full border-2 border-t-transparent" />
        加载中…
      </div>
    </div>
  )
}

function NotFoundPage() {
  const location = useLocation()
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 text-center">
      <div className="bg-muted text-muted-foreground flex size-14 items-center justify-center rounded-2xl">
        <ShieldAlert className="size-6" />
      </div>
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">页面不存在</h1>
        <p className="text-muted-foreground text-sm">
          <code className="bg-muted rounded px-1.5 py-0.5 font-mono text-xs">{location.pathname}</code>{" "}
          没有对应的页面。
        </p>
      </div>
      <Button asChild variant="outline">
        <Link to="/">
          <ArrowLeft className="size-4" /> 返回概览
        </Link>
      </Button>
    </div>
  )
}
