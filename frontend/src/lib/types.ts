export type Role = "owner" | "admin" | "member" | "viewer"

export interface Tenant {
  id: string
  slug: string
  name: string
  logoUrl?: string
  oidc?: {
    issuer: string
    clientId: string
    scopes?: string[]
    enabled: boolean
  }
  allowedDomains?: string[]
  defaultRole: Role | string
  createdAt: string
  updatedAt: string
}

export interface User {
  id: string
  tenantId: string
  subject: string
  email: string
  name: string
  avatarUrl?: string
  role: Role | string
  lastLoginAt: string
  createdAt: string
}

export interface Project {
  id: string
  tenantId: string
  name: string
  slug: string
  platform?: string
  ingestKey: string
  repoProvider?: "github" | "gitlab" | "bitbucket" | "gitea" | "custom" | string
  repoUrl?: string
  repoBranch?: string
  repoRoot?: string
  repoLinkTemplate?: string
  createdAt: string
  updatedAt: string
}

export interface EventUser {
  id?: string
  email?: string
  username?: string
}

export interface EventRequest {
  method?: string
  url?: string
  status?: number
  durationMs?: number
  requestId?: string
}

export interface Breadcrumb {
  timestamp: string
  category?: string
  message?: string
  level?: string
  data?: Record<string, unknown>
}

export interface EventItem {
  id: string
  tenantId: string
  projectId?: string
  issueId?: string
  kind: "log" | "error" | string
  source: "frontend" | "backend" | string
  level: "debug" | "info" | "warn" | "error" | "fatal" | string
  message: string
  timestamp: string
  receivedAt: string
  environment?: string
  release?: string
  service?: string
  platform?: string
  fingerprint?: string
  errorType?: string
  culprit?: string
  url?: string
  stack?: string
  sourceMap?: boolean
  tags?: Record<string, string>
  context?: Record<string, unknown>
  user?: EventUser
  request?: EventRequest
  breadcrumbs?: Breadcrumb[]
}

export interface SourceFileResponse {
  path: string
  content: string
  line: number
  language: string
  release: string
  truncated: boolean
}

export interface SourceMapMeta {
  id: string
  tenantId: string
  projectId: string
  release: string
  file: string
  size: number
  sourceCount: number
  createdAt: string
  updatedAt: string
}

export interface Issue {
  id: string
  tenantId: string
  projectId?: string
  fingerprint: string
  title: string
  culprit?: string
  level: string
  status: "open" | "resolved" | "ignored" | string
  source: "frontend" | "backend" | string
  count: number
  firstSeen: string
  lastSeen: string
  resolvedAt?: string
  resolvedBy?: string
  regressionCount: number
  assigneeId?: string
  assigneeName?: string
  createdAt: string
  updatedAt: string
}

export interface Comment {
  id: string
  tenantId: string
  issueId: string
  userId: string
  authorName: string
  body: string
  createdAt: string
}

export interface TrendPoint {
  t: string
  count: number
}

export interface Overview {
  from: string
  to: string
  stats: {
    errors: number
    logs: number
    prevErrors: number
    errorChange: number
    openIssues: number
    resolvedIssues: number
    ignoredIssues: number
    affectedUsers: number
    newIssues: number
    errorRate: number
  }
  timeseries: {
    errors: TrendPoint[]
    logs: TrendPoint[]
  }
  levels: Record<string, number>
  topIssues: Issue[]
  recentErrors: EventItem[]
}

export interface IssueDetail {
  issue: Issue
  events: EventItem[]
  comments: Comment[]
  trend: TrendPoint[]
  userCount: number
  projectName?: string
}

export interface IssueTrendPoint {
  t: string
  new: number
  resolved: number
}

export interface IssueSpike {
  issueId: string
  current: number
  previous: number
  title: string
  level: string
  projectId: string
}

export interface Suggestion {
  id: string
  severity: "critical" | "warning" | "info" | "success"
  title: string
  detail: string
  action: string
  issueId?: string
}

export interface Insights {
  from: string
  to: string
  rangeDays: number
  health: { score: number; grade: string }
  metrics: {
    openIssues: number
    resolvedIssues: number
    ignoredIssues: number
    newIssues: number
    resolvedInRange: number
    mttrHours: number
    regressions: number
    staleIssues: number
    errors: number
    prevErrors: number
    errorChange: number
    logs: number
    logErrorRate: number
    topIssueShare: number
    openFatal: number
    openErrors: number
  }
  trend: IssueTrendPoint[]
  spikes: IssueSpike[]
  topIssues: Issue[]
  suggestions: Suggestion[]
}

export interface MeResponse {
  user: User
  tenant: Tenant
  permissions: { canManage: boolean; canWrite: boolean }
  oidcEnabled: boolean
  devLogin: boolean
  version: string
}

export interface PublicTenant {
  slug: string
  name: string
  logoUrl?: string
  oidcEnabled: boolean
  devEnabled: boolean
}

export interface ProjectListResponse {
  projects: Project[]
}

export interface MembersResponse {
  members: User[]
}

export interface EventsResponse {
  events: EventItem[]
  total: number
  page: number
  pageSize: number
  from: string
  to: string
}

export interface IssuesResponse {
  issues: Issue[]
  total: number
  page: number
  pageSize: number
}

export interface TenantResponse {
  tenant: Tenant
  hasSecret: boolean
  permissions: { canManage: boolean }
}

export interface IngestResponse {
  accepted: number
  newIssues: number
  reopenedIssues: number
}
