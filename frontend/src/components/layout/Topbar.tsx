import * as React from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { ChevronDown, LogOut, Menu, Moon, Settings, Sun } from "lucide-react"
import { toast } from "sonner"

import { useAuth } from "@/context/AuthContext"
import { useProjects } from "@/context/ProjectContext"
import { useTheme } from "@/hooks/useTheme"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"

const PAGE_TITLES: Record<string, string> = {
  "/": "概览",
  "/issues": "问题",
  "/logs": "日志",
  "/traces": "Trace",
  "/metrics": "指标",
  "/insights": "持续改进",
  "/settings": "设置",
}

function pageTitle(pathname: string) {
  if (pathname.startsWith("/issues/")) return "问题详情"
  if (pathname.startsWith("/traces/")) return "Trace 详情"
  return PAGE_TITLES[pathname] ?? "Catch"
}

export function Topbar({ onOpenMenu }: { onOpenMenu: () => void }) {
  const { me } = useAuth()
  const { projects, selectedId, setSelectedId, loading } = useProjects()
  const { theme, toggle } = useTheme()
  const location = useLocation()
  const page = pageTitle(location.pathname)

  return (
    <header className="bg-background/80 sticky top-0 z-20 flex h-14 items-center gap-2 border-b border-border/60 px-3 backdrop-blur-md sm:h-16 sm:gap-3 sm:px-6 lg:px-8">
      <Button variant="ghost" size="icon-sm" className="lg:hidden" onClick={onOpenMenu}>
        <Menu className="size-4" />
      </Button>
      <div className="hidden min-w-0 sm:block">
        <div className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
          <span className="truncate">{me?.tenant.name}</span>
          <span>/</span>
          <span>{page}</span>
        </div>
        <h2 className="truncate text-sm font-semibold">{page}</h2>
      </div>

      <div className="ml-auto flex min-w-0 items-center gap-1.5 sm:gap-2">
        <Select
          value={selectedId || "all"}
          onValueChange={(v) => setSelectedId(v === "all" ? "" : v)}
          disabled={loading}
        >
          <SelectTrigger className="h-8 w-[140px] text-xs sm:w-[180px]" aria-label="选择项目">
            <SelectValue placeholder="全部项目" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部项目</SelectItem>
            {projects.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button variant="ghost" size="icon-sm" onClick={toggle} title="切换主题">
          {theme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
        </Button>

        <UserMenu />
      </div>
    </header>
  )
}

const ROLE_LABEL: Record<string, string> = {
  owner: "Owner",
  admin: "Admin",
  member: "Member",
  viewer: "Viewer",
}

function UserMenu() {
  const { me, logout } = useAuth()
  const navigate = useNavigate()
  const [loading, setLoading] = React.useState(false)
  const user = me?.user
  const initials = (user?.name || user?.email || "?").slice(0, 1).toUpperCase()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="hover:bg-accent flex items-center gap-2 rounded-full py-1 pr-2 pl-1 transition-colors">
          <span className="bg-primary/15 text-primary flex size-7 items-center justify-center rounded-full text-xs font-semibold">
            {initials}
          </span>
          <span className="hidden max-w-28 truncate text-xs font-medium sm:block">
            {user?.name || user?.email}
          </span>
          <ChevronDown className="text-muted-foreground size-3" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="font-normal">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium">{user?.name}</span>
            <span className="text-muted-foreground truncate text-xs">{user?.email}</span>
            <Badge variant="outline" className="mt-1 w-fit font-mono text-[10px]">
              {ROLE_LABEL[user?.role ?? ""] ?? user?.role}
            </Badge>
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => navigate("/settings")}>
          <Settings className="size-4" /> 工作区设置
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          disabled={loading}
          onClick={async () => {
            setLoading(true)
            try {
              await logout()
              toast.success("已退出登录")
              navigate("/login")
            } catch {
              toast.error("退出登录失败")
            } finally {
              setLoading(false)
            }
          }}
        >
          <LogOut className="size-4" /> 退出登录
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
