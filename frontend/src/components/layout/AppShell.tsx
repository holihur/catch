import * as React from "react"
import { NavLink, Outlet, useLocation } from "react-router-dom"
import { Activity, Bug, ChevronRight, LayoutDashboard, ScrollText, Settings, Sparkles, X } from "lucide-react"

import { cn, relativeTime } from "@/lib/utils"
import { useAuth } from "@/context/AuthContext"
import { useProjects } from "@/context/ProjectContext"
import { log, telemetryEnabled, trackBreadcrumb } from "@/lib/telemetry"
import { Topbar } from "./Topbar"
import { Button } from "@/components/ui/button"

const NAV = [
  {
    section: "监控",
    items: [
      { to: "/", label: "概览", icon: LayoutDashboard, end: true },
      { to: "/issues", label: "问题", icon: Bug },
      { to: "/logs", label: "日志", icon: ScrollText },
    ],
  },
  {
    section: "改进",
    items: [{ to: "/insights", label: "持续改进", icon: Sparkles }],
  },
  {
    section: "管理",
    items: [{ to: "/settings", label: "设置", icon: Settings }],
  },
]

export function AppShell() {
  const [mobileOpen, setMobileOpen] = React.useState(false)
  const location = useLocation()

  React.useEffect(() => {
    trackBreadcrumb("navigation", location.pathname)
    log("info", "page_view", { path: location.pathname, referrer: document.referrer || undefined })
  }, [location.pathname])

  React.useEffect(() => {
    setMobileOpen(false)
  }, [location.pathname])

  return (
    <div className="bg-background min-h-screen">
      <aside className="bg-sidebar text-sidebar-foreground border-sidebar-border fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r lg:flex">
        <SidebarContent />
      </aside>

      {mobileOpen ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <button
            aria-label="关闭菜单"
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setMobileOpen(false)}
          />
          <aside className="bg-sidebar text-sidebar-foreground border-sidebar-border animate-in slide-in-from-left relative flex h-full w-72 flex-col border-r duration-200">
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-sidebar-foreground hover:bg-sidebar-accent absolute top-4 right-3"
              onClick={() => setMobileOpen(false)}
            >
              <X className="size-4" />
            </Button>
            <SidebarContent />
          </aside>
        </div>
      ) : null}

      <div className="lg:pl-64">
        <Topbar onOpenMenu={() => setMobileOpen(true)} />
        <main className="mx-auto w-full max-w-[1440px] px-4 py-6 sm:px-6 lg:px-8">
          <Outlet />
        </main>
      </div>
    </div>
  )
}

function SidebarContent() {
  const { me } = useAuth()
  const { selected } = useProjects()
  return (
    <>
      <div className="flex h-16 items-center gap-3 px-5">
        <div className="from-primary to-chart-1 shadow-primary/30 flex size-9 items-center justify-center rounded-xl bg-gradient-to-br shadow-lg">
          <Activity className="text-primary-foreground size-5" />
        </div>
        <div className="flex flex-col">
          <span className="text-sidebar-accent-foreground text-[17px] font-semibold tracking-tight">
            Catch
          </span>
          <span className="text-sidebar-foreground/50 text-[10px] tracking-wider uppercase">
            Observability
          </span>
        </div>
        <span className="border-sidebar-border text-sidebar-foreground/60 ml-auto rounded-md border px-1.5 py-0.5 font-mono text-[10px]">
          v1.0
        </span>
      </div>

      <nav className="scrollbar-thin flex-1 space-y-6 overflow-y-auto px-3 py-4">
        {NAV.map((group) => (
          <div key={group.section} className="space-y-1">
            <p className="text-sidebar-foreground/40 px-2 pb-1 text-[10px] font-medium tracking-wider uppercase">
              {group.section}
            </p>
            {group.items.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  cn(
                    "group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                    isActive
                      ? "bg-sidebar-accent text-sidebar-accent-foreground"
                      : "text-sidebar-foreground/70 hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
                  )
                }
              >
                {({ isActive }) => (
                  <>
                    {isActive ? (
                      <span className="bg-primary absolute top-1/2 left-0 h-4 w-0.5 -translate-y-1/2 rounded-full" />
                    ) : null}
                    <item.icon className="size-4 opacity-80" />
                    <span>{item.label}</span>
                    <ChevronRight className="ml-auto size-3.5 opacity-0 transition-opacity group-hover:opacity-40" />
                  </>
                )}
              </NavLink>
            ))}
          </div>
        ))}
      </nav>

      <div className="border-sidebar-border space-y-3 border-t p-4">
        <div className="bg-sidebar-accent/50 flex items-center gap-3 rounded-lg p-2.5">
          <div className="bg-primary/15 text-primary flex size-8 items-center justify-center rounded-lg text-sm font-semibold">
            {me?.tenant.name?.slice(0, 1).toUpperCase() ?? "T"}
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-sidebar-accent-foreground truncate text-sm font-medium">
              {me?.tenant.name}
            </p>
            <p className="text-sidebar-foreground/50 truncate text-[11px]">
              当前项目 · {selected?.name ?? "全部项目"}
            </p>
          </div>
        </div>
        <div className="text-sidebar-foreground/45 flex items-center justify-between px-1 text-[10px]">
          <span className="flex items-center gap-1.5">
            <span
              className={cn(
                "size-1.5 rounded-full",
                telemetryEnabled() ? "bg-success animate-pulse" : "bg-warning",
              )}
            />
            {telemetryEnabled() ? "前端采集已启用" : "前端采集未配置"}
          </span>
          <span>{relativeTime(new Date())}</span>
        </div>
      </div>
    </>
  )
}
