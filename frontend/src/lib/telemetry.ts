/**
 * Catch browser SDK.
 *
 * Design goals:
 * - never break the host app (all failures swallowed)
 * - batch + persist events so a navigation or reload does not lose them
 * - keep breadcrumbs in memory and attach them to errors
 * - use the project ingest key, which is write-only and safe to expose in the bundle
 */
import type { Breadcrumb, IngestResponse } from "./types"

const API_BASE = (import.meta.env.VITE_API_URL ?? "").replace(/\/$/, "")
const CONFIGURED_ENDPOINT = import.meta.env.VITE_INGEST_ENDPOINT as string | undefined
const INGEST_KEY = (import.meta.env.VITE_INGEST_KEY as string | undefined) ?? ""
const ENDPOINT = CONFIGURED_ENDPOINT || `${API_BASE}/api/ingest`
const ENVIRONMENT = (import.meta.env.VITE_ENV as string | undefined) || import.meta.env.MODE
const RELEASE = (import.meta.env.VITE_RELEASE as string | undefined) || "web@dev"
const SERVICE = (import.meta.env.VITE_APP_NAME as string | undefined) || "catch-web"

const QUEUE_KEY = "catch:telemetry:queue:v1"
const MAX_QUEUE = 200
const MAX_BREADCRUMBS = 30
const MAX_ATTEMPTS = 8
const BATCH_SIZE = 20

export type TelemetryLevel = "debug" | "info" | "warn" | "error" | "fatal"

interface TelemetryEvent {
  id: string
  kind: "log" | "error"
  source: "frontend"
  level: TelemetryLevel
  message: string
  timestamp: string
  environment: string
  release: string
  service: string
  url: string
  errorType?: string
  stack?: string
  context?: Record<string, unknown>
  tags?: Record<string, string>
  breadcrumbs?: Breadcrumb[]
}

interface QueuedEvent {
  event: TelemetryEvent
  attempts: number
}

let initialized = false
let flushing = false
let flushTimer: number | undefined
let queue: QueuedEvent[] = []
let breadcrumbs: Breadcrumb[] = []

function safeParseQueue(): QueuedEvent[] {
  try {
    const raw = localStorage.getItem(QUEUE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as QueuedEvent[]
    return Array.isArray(parsed) ? parsed.slice(-MAX_QUEUE) : []
  } catch {
    return []
  }
}

function persistQueue() {
  try {
    localStorage.setItem(QUEUE_KEY, JSON.stringify(queue.slice(-MAX_QUEUE)))
  } catch {
    // Storage may be unavailable (private mode / quota).
  }
}

function newId(): string {
  try {
    return crypto.randomUUID()
  } catch {
    return `${Date.now()}-${Math.random().toString(16).slice(2)}`
  }
}

function breadcrumb(category: string, message: string, data?: Record<string, unknown>) {
  breadcrumbs.push({
    timestamp: new Date().toISOString(),
    category,
    message,
    level: "info",
    data,
  })
  if (breadcrumbs.length > MAX_BREADCRUMBS) {
    breadcrumbs = breadcrumbs.slice(-MAX_BREADCRUMBS)
  }
}

function enqueue(event: TelemetryEvent) {
  if (!INGEST_KEY) return
  event.breadcrumbs = breadcrumbs.slice(-MAX_BREADCRUMBS)
  queue.push({ event, attempts: 0 })
  if (queue.length > MAX_QUEUE) queue = queue.slice(-MAX_QUEUE)
  persistQueue()
  scheduleFlush(250)
}

function scheduleFlush(delay: number) {
  if (!INGEST_KEY) return
  if (flushTimer !== undefined) return
  flushTimer = window.setTimeout(() => {
    flushTimer = undefined
    void flush()
  }, delay)
}

async function flush() {
  if (flushing || queue.length === 0 || !INGEST_KEY) return
  flushing = true
  const batch = queue.slice(0, BATCH_SIZE)
  const ids = new Set(batch.map((q) => q.event.id))
  try {
    const res = await fetch(ENDPOINT, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Ingest-Key": INGEST_KEY },
      body: JSON.stringify({ events: batch.map((q) => q.event) }),
      keepalive: true,
      credentials: "omit",
    })
    if (res.ok) {
      queue = queue.filter((q) => !ids.has(q.event.id))
      persistQueue()
    } else {
      markAttempts(ids)
    }
  } catch {
    markAttempts(ids)
  } finally {
    flushing = false
    if (queue.length > 0) scheduleFlush(4000)
  }
}

function markAttempts(ids: Set<string>) {
  queue = queue
    .map((q) => (ids.has(q.event.id) ? { ...q, attempts: q.attempts + 1 } : q))
    .filter((q) => q.attempts < MAX_ATTEMPTS)
  persistQueue()
}

function beacon() {
  if (!INGEST_KEY || queue.length === 0) return
  try {
    const batch = queue.slice(0, 50)
    const body = JSON.stringify({ events: batch.map((q) => q.event) })
    const url = new URL(ENDPOINT, window.location.origin)
    const sent = navigator.sendBeacon(
      `${url.toString()}?key=${encodeURIComponent(INGEST_KEY)}`,
      new Blob([body], { type: "text/plain;charset=UTF-8" }),
    )
    if (sent) {
      const ids = new Set(batch.map((q) => q.event.id))
      queue = queue.filter((q) => !ids.has(q.event.id))
      persistQueue()
    }
  } catch {
    // ignore
  }
}

function baseEvent(kind: "log" | "error", level: TelemetryLevel, message: string): TelemetryEvent {
  return {
    id: newId(),
    kind,
    source: "frontend",
    level,
    message,
    timestamp: new Date().toISOString(),
    environment: ENVIRONMENT,
    release: RELEASE,
    service: SERVICE,
    url: window.location.href,
  }
}

export function initTelemetry() {
  if (initialized || typeof window === "undefined") return
  initialized = true
  queue = safeParseQueue()
  window.addEventListener("error", (event) => {
    const error = event.error instanceof Error ? event.error : undefined
    captureError(error ?? event.message, {
      mechanism: "window.onerror",
      filename: event.filename,
      lineno: event.lineno,
      colno: event.colno,
    })
  })
  window.addEventListener("unhandledrejection", (event) => {
    captureError(event.reason, { mechanism: "unhandledrejection" })
  })
  window.addEventListener("pagehide", beacon)
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") {
      beacon()
    }
  })
  scheduleFlush(1000)
}

export function telemetryEnabled(): boolean {
  return Boolean(INGEST_KEY)
}

export function telemetryInfo(): { enabled: boolean; endpoint: string; environment: string; release: string } {
  return { enabled: Boolean(INGEST_KEY), endpoint: ENDPOINT, environment: ENVIRONMENT, release: RELEASE }
}

export function log(
  level: TelemetryLevel,
  message: string,
  context?: Record<string, unknown>,
  tags?: Record<string, string>,
) {
  const event = baseEvent("log", level, message)
  if (context) event.context = context
  if (tags) event.tags = tags
  enqueue(event)
}

export function captureError(error: unknown, context?: Record<string, unknown>) {
  let message = "Unknown error"
  let errorType = "Error"
  let stack: string | undefined
  if (error instanceof Error) {
    message = error.message || error.name
    errorType = error.name
    stack = error.stack
  } else if (typeof error === "string") {
    message = error
  } else if (error && typeof error === "object") {
    const maybe = error as { message?: unknown; name?: unknown; stack?: unknown }
    if (typeof maybe.message === "string") message = maybe.message
    if (typeof maybe.name === "string") errorType = maybe.name
    if (typeof maybe.stack === "string") stack = maybe.stack
    if (message === "Unknown error") message = safeStringify(error)
  }
  const event = baseEvent("error", "error", message)
  event.errorType = errorType
  event.stack = stack
  if (context) event.context = context
  enqueue(event)
}

export function trackBreadcrumb(category: string, message: string, data?: Record<string, unknown>) {
  breadcrumb(category, message, data)
}

/** Send a test event synchronously with an explicit project key (admin settings). */
export async function sendTestEvent(
  ingestKey: string,
  kind: "log" | "error",
): Promise<IngestResponse> {
  const event = baseEvent(
    kind,
    kind === "error" ? "error" : "info",
    kind === "error"
      ? "TestError: 来自 Catch 控制台的测试错误"
      : "Catch 控制台测试日志：前后端采集链路正常",
  )
  if (kind === "error") {
    event.errorType = "TestError"
    event.stack = "TestError: 来自 Catch 控制台的测试错误\n    at Settings.handleTest (Settings.tsx:1:1)"
  }
  event.context = { trigger: "settings-test", source: "console" }
  event.breadcrumbs = [{ timestamp: new Date().toISOString(), category: "ui", message: "clicked test button" }]
  const res = await fetch(ENDPOINT, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Ingest-Key": ingestKey },
    body: JSON.stringify({ events: [event] }),
  })
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: { message?: string } } | null
    throw new Error(body?.error?.message || `发送失败 (${res.status})`)
  }
  return (await res.json()) as IngestResponse
}

function safeStringify(value: unknown): string {
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}
