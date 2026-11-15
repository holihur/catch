import * as React from "react"
import { Navigate, useLocation } from "react-router-dom"

import { api, ApiError } from "@/lib/api"
import type { MeResponse } from "@/lib/types"
import { Skeleton } from "@/components/ui/skeleton"

interface AuthContextValue {
  me: MeResponse | null
  loading: boolean
  refresh: () => Promise<MeResponse | null>
  devLogin: (tenant: string, email: string, name?: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = React.createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [me, setMe] = React.useState<MeResponse | null>(null)
  const [loading, setLoading] = React.useState(true)

  const refresh = React.useCallback(async () => {
    try {
      const next = await api.me()
      setMe(next)
      return next
    } catch (err) {
      if (!(err instanceof ApiError) || err.status !== 401) {
        console.warn("auth check failed", err)
      }
      setMe(null)
      return null
    } finally {
      setLoading(false)
    }
  }, [])

  React.useEffect(() => {
    void refresh()
  }, [refresh])

  React.useEffect(() => {
    const onUnauthorized = () => setMe(null)
    window.addEventListener("catch:unauthorized", onUnauthorized)
    return () => window.removeEventListener("catch:unauthorized", onUnauthorized)
  }, [])

  const devLogin = React.useCallback(async (tenant: string, email: string, name?: string) => {
    await api.devLogin(tenant, email, name)
    await refresh()
  }, [refresh])

  const logout = React.useCallback(async () => {
    try {
      await api.logout()
    } finally {
      setMe(null)
    }
  }, [])

  const value = React.useMemo(
    () => ({ me, loading, refresh, devLogin, logout }),
    [me, loading, refresh, devLogin, logout],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = React.useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { me, loading } = useAuth()
  const location = useLocation()
  if (loading) return <FullScreenLoader />
  if (!me) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <>{children}</>
}

export function GuestOnly({ children }: { children: React.ReactNode }) {
  const { me, loading } = useAuth()
  if (loading) return <FullScreenLoader />
  if (me) return <Navigate to="/" replace />
  return <>{children}</>
}

function FullScreenLoader() {
  return (
    <div className="flex min-h-screen flex-col gap-4 p-8">
      <div className="flex items-center gap-3">
        <Skeleton className="size-10 rounded-xl" />
        <Skeleton className="h-5 w-28" />
      </div>
      <div className="grid gap-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-28 rounded-xl" />
        ))}
      </div>
      <Skeleton className="h-80 rounded-xl" />
    </div>
  )
}
