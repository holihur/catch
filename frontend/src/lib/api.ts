import { captureError } from "./telemetry"
import type {
  EventItem,
  EventsResponse,
  Insights,
  Issue,
  IssueDetail,
  IssuesResponse,
  MeResponse,
  MembersResponse,
  Overview,
  Project,
  ProjectListResponse,
  PublicTenant,
  SourceFileResponse,
  SourceMapMeta,
  Tenant,
  TenantResponse,
  User,
} from "./types"

const API_BASE = (import.meta.env.VITE_API_URL ?? "").replace(/\/$/, "")

export class ApiError extends Error {
  status: number
  code: string
  requestId?: string
  details?: unknown

  constructor(status: number, code: string, message: string, requestId?: string, details?: unknown) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
    this.requestId = requestId
    this.details = details
  }
}

interface RequestOptions {
  method?: string
  body?: unknown
  signal?: AbortSignal
  skipAuthEvent?: boolean
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, signal, skipAuthEvent } = options
  const headers: Record<string, string> = {}
  if (body !== undefined) headers["Content-Type"] = "application/json"
  const started = performance.now()
  let res: Response
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      credentials: "include",
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    })
  } catch (err) {
    if ((err as Error)?.name !== "AbortError") {
      captureError(err, { kind: "network", path, method })
    }
    throw new ApiError(0, "network_error", "网络请求失败，请检查网络连接")
  }
  const requestId = res.headers.get("X-Request-Id") ?? undefined
  if (res.status === 204) return undefined as T
  const text = await res.text()
  let payload: unknown = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = null
    }
  }
  if (!res.ok) {
    const errBody = payload as {
      error?: { code?: string; message?: string; details?: unknown }
    } | null
    const err = new ApiError(
      res.status,
      errBody?.error?.code ?? "request_failed",
      errBody?.error?.message ?? `请求失败 (${res.status})`,
      requestId,
      errBody?.error?.details,
    )
    if (res.status === 401 && !skipAuthEvent) {
      window.dispatchEvent(new CustomEvent("catch:unauthorized"))
    }
    if (res.status >= 500) {
      captureError(err, {
        kind: "api",
        path,
        method,
        status: res.status,
        requestId,
        durationMs: Math.round(performance.now() - started),
      })
    }
    throw err
  }
  return payload as T
}

function qs(params: Record<string, string | number | undefined | null>): string {
  const search = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== "") search.set(k, String(v))
  }
  const s = search.toString()
  return s ? `?${s}` : ""
}

export const api = {
  // auth
  publicTenants: () =>
    request<{ tenants: PublicTenant[] }>("/api/auth/tenants", { skipAuthEvent: true }),
  publicTenant: (slug: string) =>
    request<PublicTenant>(`/api/auth/tenants/${encodeURIComponent(slug)}`, { skipAuthEvent: true }),
  oidcStart: (tenant: string, returnTo = "/") =>
    request<{ url: string }>(
      `/api/auth/oidc/start${qs({ tenant, returnTo })}`,
      { skipAuthEvent: true },
    ),
  devLogin: (tenant: string, email: string, name?: string) =>
    request<{ user: User; tenant: Tenant }>("/api/auth/dev", {
      method: "POST",
      body: { tenant, email, name },
      skipAuthEvent: true,
    }),
  me: () => request<MeResponse>("/api/auth/me", { skipAuthEvent: true }),
  logout: () => request<void>("/api/auth/logout", { method: "POST", skipAuthEvent: true }),

  // projects
  projects: () => request<ProjectListResponse>("/api/projects"),
  createProject: (body: { name: string; slug?: string; platform?: string }) =>
    request<Project>("/api/projects", { method: "POST", body }),
  updateProject: (
    id: string,
    body: {
      name?: string
      platform?: string
      repoProvider?: string
      repoUrl?: string
      repoBranch?: string
      repoRoot?: string
      repoLinkTemplate?: string
    },
  ) => request<Project>(`/api/projects/${id}`, { method: "PATCH", body }),
  rotateKey: (id: string) =>
    request<Project>(`/api/projects/${id}/rotate-key`, { method: "POST" }),
  deleteProject: (id: string) => request<void>(`/api/projects/${id}`, { method: "DELETE" }),

  // issues
  issues: (params: {
    projectId?: string
    status?: string
    level?: string
    source?: string
    q?: string
    sort?: string
    page?: number
    pageSize?: number
  }) => request<IssuesResponse>(`/api/issues${qs(params)}`),
  issue: (id: string) => request<IssueDetail>(`/api/issues/${id}`),
  updateIssue: (id: string, body: { status?: string; assigneeId?: string }) =>
    request<Issue>(`/api/issues/${id}`, { method: "PATCH", body }),
  addComment: (id: string, body: string) =>
    request(`/api/issues/${id}/comments`, { method: "POST", body: { body } }),

  // events
  events: (params: {
    projectId?: string
    kind?: string
    source?: string
    level?: string
    q?: string
    range?: string
    page?: number
    pageSize?: number
  }) => request<EventsResponse>(`/api/events${qs(params)}`),
  event: (id: string) => request<EventItem>(`/api/events/${id}`),

  // analytics
  overview: (params: { projectId?: string; range?: string }) =>
    request<Overview>(`/api/overview${qs(params)}`),
  insights: (params: { projectId?: string; range?: string }) =>
    request<Insights>(`/api/insights${qs(params)}`),

  // source maps
  sourceFile: (projectId: string, release: string, path: string, line?: number) =>
    request<SourceFileResponse>(
      `/api/projects/${projectId}/source${qs({ release, path, line })}`,
    ),
  sourceMaps: (projectId: string, release?: string) =>
    request<{ sourceMaps: SourceMapMeta[] }>(
      `/api/projects/${projectId}/sourcemaps${qs({ release })}`,
    ),
  uploadSourceMap: async (
    projectId: string,
    input: { release: string; file: string; map: File },
  ): Promise<SourceMapMeta> => {
    const form = new FormData()
    form.append("release", input.release)
    if (input.file) form.append("file", input.file)
    form.append("map", input.map, input.map.name)
    const res = await fetch(`${API_BASE}/api/projects/${projectId}/sourcemaps`, {
      method: "POST",
      credentials: "include",
      body: form,
    })
    const text = await res.text()
    const payload = text ? JSON.parse(text) : null
    if (!res.ok) {
      const err = new ApiError(
        res.status,
        payload?.error?.code ?? "upload_failed",
        payload?.error?.message ?? `上传失败 (${res.status})`,
        res.headers.get("X-Request-Id") ?? undefined,
      )
      if (res.status === 401) window.dispatchEvent(new CustomEvent("catch:unauthorized"))
      if (res.status >= 500) captureError(err, { kind: "api", path: `/api/projects/${projectId}/sourcemaps`, method: "POST" })
      throw err
    }
    return payload.sourceMap as SourceMapMeta
  },
  deleteSourceMap: (projectId: string, mapId: string) =>
    request<void>(`/api/projects/${projectId}/sourcemaps/${mapId}`, { method: "DELETE" }),

  // members & tenant
  members: () => request<MembersResponse>("/api/members"),
  updateMember: (id: string, role: string) =>
    request<User>(`/api/members/${id}`, { method: "PATCH", body: { role } }),
  removeMember: (id: string) => request<void>(`/api/members/${id}`, { method: "DELETE" }),
  tenant: () => request<TenantResponse>("/api/tenant"),
  updateTenant: (name: string) =>
    request<Tenant>("/api/tenant", { method: "PATCH", body: { name } }),
  updateOIDC: (body: {
    issuer: string
    clientId: string
    clientSecret: string
    scopes: string[]
    enabled: boolean
    allowedDomains: string[]
  }) => request<{ tenant: Tenant; hasSecret: boolean }>("/api/tenant/oidc", { method: "PUT", body }),
}

export { API_BASE }
