import * as React from "react"
import { useLocation, useNavigate } from "react-router-dom"
import {
  Activity,
  ArrowRight,
  Bug,
  Building2,
  Loader2,
  LockKeyhole,
  ScrollText,
  ShieldCheck,
  Sparkles,
} from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { PublicTenant } from "@/lib/types"
import { useAuth } from "@/context/AuthContext"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Separator } from "@/components/ui/separator"

const OIDC_ERRORS: Record<string, string> = {
  invalid_state: "登录状态已失效，请重新发起登录",
  domain_denied: "该邮箱域名不在租户允许范围内",
  no_email: "身份提供方未返回邮箱，请联系管理员",
  session_failed: "无法创建登录会话，请稍后重试",
  provider_error: "身份提供方返回错误，请查看服务端日志",
  oidc_failed: "OIDC 登录失败，请检查身份提供方配置",
}

export function LoginPage() {
  const { devLogin, me } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from ?? "/"

  const [tenants, setTenants] = React.useState<PublicTenant[]>([])
  const [loadingTenants, setLoadingTenants] = React.useState(true)
  const [slug, setSlug] = React.useState(
    () => localStorage.getItem("catch:tenant") ?? "acme",
  )
  const [email, setEmail] = React.useState("demo@acme.dev")
  const [name, setName] = React.useState("Demo Owner")
  const [busy, setBusy] = React.useState<"sso" | "dev" | null>(null)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    let active = true
    api
      .publicTenants()
      .then((res) => {
        if (!active) return
        setTenants(res.tenants)
        if (res.tenants.length > 0 && !res.tenants.some((t) => t.slug === slug)) {
          setSlug(res.tenants[0].slug)
        }
      })
      .catch((err) => {
        if (active) setError(err instanceof Error ? err.message : "无法加载租户列表")
      })
      .finally(() => {
        if (active) setLoadingTenants(false)
      })
    return () => {
      active = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  React.useEffect(() => {
    const params = new URLSearchParams(location.search)
    const code = params.get("error")
    if (code) {
      setError(OIDC_ERRORS[code] ?? "登录失败，请重试")
      const url = new URL(window.location.href)
      url.searchParams.delete("error")
      window.history.replaceState({}, "", url.toString())
    }
  }, [location.search])

  React.useEffect(() => {
    if (me) navigate(from, { replace: true })
  }, [me, from, navigate])

  const selected = tenants.find((t) => t.slug === slug) ?? null

  const selectTenant = (next: string) => {
    setSlug(next)
    localStorage.setItem("catch:tenant", next)
    setError(null)
    if (next === "acme") {
      setEmail("demo@acme.dev")
      setName("Demo Owner")
    } else {
      setEmail("")
      setName("")
    }
  }

  const startSSO = async () => {
    if (!selected) return
    setBusy("sso")
    setError(null)
    try {
      const res = await api.oidcStart(selected.slug, from)
      window.location.assign(res.url)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "无法启动 SSO 登录")
      setBusy(null)
    }
  }

  const submitDev = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selected) return
    if (!email.includes("@")) {
      setError("请输入有效的邮箱地址")
      return
    }
    setBusy("dev")
    setError(null)
    try {
      await devLogin(selected.slug, email.trim(), name.trim() || undefined)
      toast.success("登录成功")
      navigate(from, { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : "开发登录失败")
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="grid min-h-screen lg:grid-cols-[1.05fr_1fr]">
      <section className="bg-sidebar text-sidebar-foreground relative hidden overflow-hidden lg:flex lg:flex-col lg:justify-between">
        <div className="bg-login-glow absolute inset-0 opacity-80" />
        <div className="bg-dot-grid absolute inset-0 opacity-[0.08]" />
        <div className="relative z-10 flex items-center gap-3 p-10">
          <div className="from-primary to-chart-1 shadow-primary/30 flex size-10 items-center justify-center rounded-xl bg-gradient-to-br shadow-xl">
            <Activity className="text-primary-foreground size-5" />
          </div>
          <div>
            <p className="text-lg font-semibold tracking-tight">Catch</p>
            <p className="text-sidebar-foreground/50 text-[10px] tracking-[0.2em] uppercase">
              Observability Platform
            </p>
          </div>
        </div>

        <div className="relative z-10 max-w-xl px-10">
          <div className="border-sidebar-border text-sidebar-foreground/70 mb-6 inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs">
            <span className="bg-success size-1.5 animate-pulse rounded-full" />
            多租户 · OIDC SSO · 实时采集
          </div>
          <h1 className="text-4xl leading-tight font-semibold tracking-tight text-balance">
            捕获每一次异常，
            <br />
            持续改进产品质量
          </h1>
          <p className="text-sidebar-foreground/60 mt-4 max-w-md text-sm leading-relaxed">
            统一收集前端与后端日志、错误和上下文，自动归并问题、量化质量趋势，让每一次修复都有据可依。
          </p>

          <div className="mt-10 grid grid-cols-2 gap-4">
            {[
              { icon: Bug, title: "错误聚合", desc: "指纹归并 · 回归检测" },
              { icon: ScrollText, title: "统一日志", desc: "前端 / 后端 / 上下文" },
              { icon: ShieldCheck, title: "多租户隔离", desc: "每个租户独立 OIDC" },
              { icon: Sparkles, title: "持续改进", desc: "健康分 · 改进建议" },
            ].map((f) => (
              <div
                key={f.title}
                className="border-sidebar-border bg-sidebar-accent/30 rounded-xl border p-4 backdrop-blur-sm"
              >
                <f.icon className="text-primary mb-2 size-4" />
                <p className="text-sm font-medium">{f.title}</p>
                <p className="text-sidebar-foreground/50 mt-0.5 text-xs">{f.desc}</p>
              </div>
            ))}
          </div>
        </div>

        <div className="text-sidebar-foreground/35 relative z-10 flex items-center justify-between p-10 text-xs">
          <span>© {new Date().getFullYear()} Catch</span>
          <span>v1.0 · Built with Go & React</span>
        </div>
      </section>

      <section className="bg-background relative flex min-h-screen items-center justify-center px-4 py-10 sm:px-8">
        <div className="bg-dot-grid pointer-events-none absolute inset-0 opacity-[0.35] lg:hidden" />
        <div className="relative w-full max-w-md">
          <div className="mb-8 flex items-center gap-3 lg:hidden">
            <div className="from-primary to-chart-1 flex size-10 items-center justify-center rounded-xl bg-gradient-to-br">
              <Activity className="text-primary-foreground size-5" />
            </div>
            <div>
              <p className="text-lg font-semibold tracking-tight">Catch</p>
              <p className="text-muted-foreground text-xs">日志与错误可观测平台</p>
            </div>
          </div>

          <Card className="gap-5 shadow-lg">
            <CardHeader>
              <CardTitle className="text-xl">登录工作区</CardTitle>
              <CardDescription>
                选择租户并使用 OIDC 单点登录，或使用本地开发身份进入。
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              {error ? (
                <div className="border-destructive/30 bg-destructive/10 text-destructive rounded-lg border px-3 py-2 text-sm">
                  {error}
                </div>
              ) : null}

              {loadingTenants ? (
                <div className="space-y-3">
                  <Skeleton className="h-9 w-full" />
                  <Skeleton className="h-24 w-full" />
                </div>
              ) : tenants.length === 0 ? (
                <div className="rounded-lg border border-dashed p-4 text-sm">
                  <p className="font-medium">尚未配置租户</p>
                  <p className="text-muted-foreground mt-1 text-xs leading-relaxed">
                    在服务端设置 <code className="bg-muted rounded px-1">OIDC_TENANT_SLUG</code>、
                    <code className="bg-muted rounded px-1">OIDC_ISSUER</code>、
                    <code className="bg-muted rounded px-1">OIDC_CLIENT_ID</code> 后重启，或使用种子数据。
                  </p>
                </div>
              ) : (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="tenant">租户</Label>
                    <Select value={slug} onValueChange={selectTenant}>
                      <SelectTrigger id="tenant" className="w-full">
                        <SelectValue placeholder="选择租户" />
                      </SelectTrigger>
                      <SelectContent>
                        {tenants.map((t) => (
                          <SelectItem key={t.slug} value={t.slug}>
                            {t.name} · {t.slug}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  {selected?.oidcEnabled ? (
                    <div className="space-y-3">
                      <Button className="w-full" onClick={() => void startSSO()} disabled={busy !== null}>
                        {busy === "sso" ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : (
                          <Building2 className="size-4" />
                        )}
                        使用 OIDC 单点登录
                        <ArrowRight className="ml-auto size-4" />
                      </Button>
                      {selected.devEnabled ? (
                        <div className="relative py-1">
                          <Separator />
                          <span className="bg-card text-muted-foreground absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 px-2 text-[10px] tracking-wider uppercase">
                            或开发登录
                          </span>
                        </div>
                      ) : null}
                    </div>
                  ) : null}

                  {selected?.devEnabled ? (
                    <form onSubmit={submitDev} className="space-y-3">
                      {!selected.oidcEnabled ? (
                        <div className="border-warning/30 bg-warning/10 text-warning-foreground dark:text-warning flex items-start gap-2 rounded-lg border p-3 text-xs">
                          <LockKeyhole className="mt-0.5 size-3.5 shrink-0" />
                          开发登录仅用于本地验证，生产环境请配置 OIDC。
                        </div>
                      ) : null}
                      <div className="space-y-2">
                        <Label htmlFor="email">邮箱</Label>
                        <Input
                          id="email"
                          type="email"
                          autoComplete="email"
                          placeholder="you@company.com"
                          value={email}
                          onChange={(e) => setEmail(e.target.value)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="name">显示名称（可选）</Label>
                        <Input
                          id="name"
                          placeholder="你的名字"
                          value={name}
                          onChange={(e) => setName(e.target.value)}
                        />
                      </div>
                      <Button type="submit" variant={selected.oidcEnabled ? "outline" : "default"} className="w-full" disabled={busy !== null}>
                        {busy === "dev" ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : (
                          <ArrowRight className="size-4" />
                        )}
                        进入工作区
                      </Button>
                      {selected.slug === "acme" ? (
                        <p className="text-muted-foreground text-center text-[11px]">
                          演示租户已预置 <span className="font-mono">demo@acme.dev</span>
                        </p>
                      ) : null}
                    </form>
                  ) : null}

                  {selected && !selected.oidcEnabled && !selected.devEnabled ? (
                    <div className="border-destructive/30 bg-destructive/10 text-destructive rounded-lg border p-3 text-sm">
                      该租户未配置任何登录方式，请联系管理员启用 OIDC。
                    </div>
                  ) : null}
                </>
              )}

              <div className="text-muted-foreground flex items-center gap-1.5 border-t pt-4 text-[11px]">
                <ShieldCheck className="size-3.5" />
                租户数据完全隔离，会话使用 HttpOnly Cookie 存储
              </div>
            </CardContent>
          </Card>
        </div>
      </section>
    </div>
  )
}
