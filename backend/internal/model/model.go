package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Roles, ordered from least to most privileged.
const (
	RoleViewer = "viewer"
	RoleMember = "member"
	RoleAdmin  = "admin"
	RoleOwner  = "owner"
)

// Issue statuses.
const (
	IssueOpen     = "open"
	IssueResolved = "resolved"
	IssueIgnored  = "ignored"
)

// Event kinds and sources.
const (
	KindLog   = "log"
	KindError = "error"

	SourceFrontend = "frontend"
	SourceBackend  = "backend"
)

// Tenant is an isolated organization. Users, projects and events never cross
// tenant boundaries.
type Tenant struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Slug           string             `bson:"slug" json:"slug"`
	Name           string             `bson:"name" json:"name"`
	LogoURL        string             `bson:"logoUrl,omitempty" json:"logoUrl,omitempty"`
	OIDC           *OIDCConfig        `bson:"oidc,omitempty" json:"oidc,omitempty"`
	AllowedDomains []string           `bson:"allowedDomains,omitempty" json:"allowedDomains,omitempty"`
	DefaultRole    string             `bson:"defaultRole" json:"defaultRole"`
	CreatedAt      time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// OIDCConfig holds per-tenant OpenID Connect settings. ClientSecretEnc is an
// AES-GCM encrypted value and is never serialized to API responses.
type OIDCConfig struct {
	Issuer          string   `bson:"issuer" json:"issuer"`
	ClientID        string   `bson:"clientId" json:"clientId"`
	ClientSecretEnc string   `bson:"clientSecretEnc,omitempty" json:"-"`
	Scopes          []string `bson:"scopes,omitempty" json:"scopes,omitempty"`
	Enabled         bool     `bson:"enabled" json:"enabled"`
}

// User is a tenant-scoped membership. The same person logging into two tenants
// produces two user documents, linked only by OIDC subject/email.
type User struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID    primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	Subject     string             `bson:"subject" json:"subject"`
	Email       string             `bson:"email" json:"email"`
	Name        string             `bson:"name" json:"name"`
	AvatarURL   string             `bson:"avatarUrl,omitempty" json:"avatarUrl,omitempty"`
	Role        string             `bson:"role" json:"role"`
	LastLoginAt time.Time          `bson:"lastLoginAt" json:"lastLoginAt"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}

// Session is a server-side session. Only the SHA-256 hash of the token is stored.
type Session struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TokenHash string             `bson:"tokenHash" json:"-"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	TenantID  primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ExpiresAt time.Time          `bson:"expiresAt" json:"expiresAt"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// OIDCState is a short-lived login transaction (state + PKCE + nonce).
type OIDCState struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	StateHash string             `bson:"stateHash"`
	TenantID  primitive.ObjectID `bson:"tenantId"`
	Verifier  string             `bson:"verifier"`
	Nonce     string             `bson:"nonce"`
	ReturnTo  string             `bson:"returnTo"`
	ExpiresAt time.Time          `bson:"expiresAt"`
}

// Project is an ingest target (one app or service) inside a tenant.
type Project struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID  primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	Name      string             `bson:"name" json:"name"`
	Slug      string             `bson:"slug" json:"slug"`
	Platform  string             `bson:"platform,omitempty" json:"platform,omitempty"`
	IngestKey string             `bson:"ingestKey" json:"ingestKey"`

	// Source mapping lets stack frames deep-link into the project repository.
	RepoProvider     string `bson:"repoProvider,omitempty" json:"repoProvider,omitempty"`
	RepoURL          string `bson:"repoUrl,omitempty" json:"repoUrl,omitempty"`
	RepoBranch       string `bson:"repoBranch,omitempty" json:"repoBranch,omitempty"`
	RepoRoot         string `bson:"repoRoot,omitempty" json:"repoRoot,omitempty"`
	RepoLinkTemplate string `bson:"repoLinkTemplate,omitempty" json:"repoLinkTemplate,omitempty"`

	TraceSampleRate        float64 `bson:"traceSampleRate" json:"traceSampleRate"`
	TraceErrorAlwaysSample bool    `bson:"traceErrorAlwaysSample" json:"traceErrorAlwaysSample"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// Event is a single log line or error occurrence. Logs and errors share one
// collection so that the UI can filter and correlate them uniformly.
type Event struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID    primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID   primitive.ObjectID `bson:"projectId,omitempty" json:"projectId,omitempty"`
	IssueID     primitive.ObjectID `bson:"issueId,omitempty" json:"issueId,omitempty"`
	Kind        string             `bson:"kind" json:"kind"`
	Source      string             `bson:"source" json:"source"`
	Level       string             `bson:"level" json:"level"`
	Message     string             `bson:"message" json:"message"`
	Timestamp   time.Time          `bson:"timestamp" json:"timestamp"`
	ReceivedAt  time.Time          `bson:"receivedAt" json:"receivedAt"`
	Environment string             `bson:"environment,omitempty" json:"environment,omitempty"`
	Release     string             `bson:"release,omitempty" json:"release,omitempty"`
	Service     string             `bson:"service,omitempty" json:"service,omitempty"`
	Platform    string             `bson:"platform,omitempty" json:"platform,omitempty"`
	Fingerprint string             `bson:"fingerprint,omitempty" json:"fingerprint,omitempty"`
	ErrorType   string             `bson:"errorType,omitempty" json:"errorType,omitempty"`
	Culprit     string             `bson:"culprit,omitempty" json:"culprit,omitempty"`
	URL         string             `bson:"url,omitempty" json:"url,omitempty"`
	Stack       string             `bson:"stack,omitempty" json:"stack,omitempty"`
	SourceMap   bool               `bson:"sourceMap,omitempty" json:"sourceMap,omitempty"`
	Tags        map[string]string  `bson:"tags,omitempty" json:"tags,omitempty"`
	Context     map[string]any     `bson:"context,omitempty" json:"context,omitempty"`
	User        *EventUser         `bson:"user,omitempty" json:"user,omitempty"`
	Request     *EventRequest      `bson:"request,omitempty" json:"request,omitempty"`
	Breadcrumbs []Breadcrumb       `bson:"breadcrumbs,omitempty" json:"breadcrumbs,omitempty"`
}

type EventUser struct {
	ID       string `bson:"id,omitempty" json:"id,omitempty"`
	Email    string `bson:"email,omitempty" json:"email,omitempty"`
	Username string `bson:"username,omitempty" json:"username,omitempty"`
}

type EventRequest struct {
	Method     string `bson:"method,omitempty" json:"method,omitempty"`
	URL        string `bson:"url,omitempty" json:"url,omitempty"`
	Status     int    `bson:"status,omitempty" json:"status,omitempty"`
	DurationMS int64  `bson:"durationMs,omitempty" json:"durationMs,omitempty"`
	RequestID  string `bson:"requestId,omitempty" json:"requestId,omitempty"`
}

type Breadcrumb struct {
	Timestamp time.Time      `bson:"timestamp" json:"timestamp"`
	Category  string         `bson:"category,omitempty" json:"category,omitempty"`
	Message   string         `bson:"message,omitempty" json:"message,omitempty"`
	Level     string         `bson:"level,omitempty" json:"level,omitempty"`
	Data      map[string]any `bson:"data,omitempty" json:"data,omitempty"`
}

// Issue is a group of error events sharing a fingerprint.
type Issue struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID        primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID       primitive.ObjectID `bson:"projectId,omitempty" json:"projectId,omitempty"`
	Fingerprint     string             `bson:"fingerprint" json:"fingerprint"`
	Title           string             `bson:"title" json:"title"`
	Culprit         string             `bson:"culprit,omitempty" json:"culprit,omitempty"`
	Level           string             `bson:"level" json:"level"`
	Status          string             `bson:"status" json:"status"`
	Source          string             `bson:"source" json:"source"`
	Count           int64              `bson:"count" json:"count"`
	FirstSeen       time.Time          `bson:"firstSeen" json:"firstSeen"`
	LastSeen        time.Time          `bson:"lastSeen" json:"lastSeen"`
	ResolvedAt      *time.Time         `bson:"resolvedAt,omitempty" json:"resolvedAt,omitempty"`
	ResolvedBy      string             `bson:"resolvedBy,omitempty" json:"resolvedBy,omitempty"`
	RegressionCount int                `bson:"regressionCount" json:"regressionCount"`
	AssigneeID      string             `bson:"assigneeId,omitempty" json:"assigneeId,omitempty"`
	AssigneeName    string             `bson:"assigneeName,omitempty" json:"assigneeName,omitempty"`
	CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Comment is a discussion entry on an issue.
type Comment struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID   primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	IssueID    primitive.ObjectID `bson:"issueId" json:"issueId"`
	UserID     primitive.ObjectID `bson:"userId" json:"userId"`
	AuthorName string             `bson:"authorName" json:"authorName"`
	Body       string             `bson:"body" json:"body"`
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
}

// Span is one OpenTelemetry trace span. All spans are stored for the trace UI.
type Span struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID      primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID     primitive.ObjectID `bson:"projectId" json:"projectId"`
	TraceID       string             `bson:"traceId" json:"traceId"`
	SpanID        string             `bson:"spanId" json:"spanId"`
	ParentSpanID  string             `bson:"parentSpanId,omitempty" json:"parentSpanId,omitempty"`
	Name          string             `bson:"name" json:"name"`
	Kind          string             `bson:"kind,omitempty" json:"kind,omitempty"`
	Service       string             `bson:"service,omitempty" json:"service,omitempty"`
	Environment   string             `bson:"environment,omitempty" json:"environment,omitempty"`
	Release       string             `bson:"release,omitempty" json:"release,omitempty"`
	Platform      string             `bson:"platform,omitempty" json:"platform,omitempty"`
	StartTime     time.Time          `bson:"startTime" json:"startTime"`
	EndTime       time.Time          `bson:"endTime" json:"endTime"`
	DurationMS    int64              `bson:"durationMs" json:"durationMs"`
	Status        string             `bson:"status" json:"status"`
	StatusMessage string             `bson:"statusMessage,omitempty" json:"statusMessage,omitempty"`
	ScopeName     string             `bson:"scopeName,omitempty" json:"scopeName,omitempty"`
	ScopeVersion  string             `bson:"scopeVersion,omitempty" json:"scopeVersion,omitempty"`
	Attributes    map[string]any     `bson:"attributes,omitempty" json:"attributes,omitempty"`
	Resource      map[string]any     `bson:"resource,omitempty" json:"resource,omitempty"`
	Events        []SpanEvent        `bson:"events,omitempty" json:"events,omitempty"`
	ReceivedAt    time.Time          `bson:"receivedAt" json:"receivedAt"`
}

// SpanEvent is an annotated event inside a span (usually an exception).
type SpanEvent struct {
	Time       time.Time      `bson:"time" json:"time"`
	Name       string         `bson:"name" json:"name"`
	Attributes map[string]any `bson:"attributes,omitempty" json:"attributes,omitempty"`
}

// MetricPoint is one OTLP metric data point. Histograms/summaries keep count
// and sum; Value is the average for charting.
type MetricQuantile struct {
	Quantile float64 `bson:"quantile" json:"quantile"`
	Value    float64 `bson:"value" json:"value"`
}

// MetricDistribution preserves histogram bucket layout. Explicit bounds are
// used by classic histograms; exponential histograms keep their scale and
// positive/negative bucket counts.
type MetricDistribution struct {
	BucketCounts    []uint64  `bson:"bucketCounts,omitempty" json:"bucketCounts,omitempty"`
	ExplicitBounds  []float64 `bson:"explicitBounds,omitempty" json:"explicitBounds,omitempty"`
	ZeroCount       uint64    `bson:"zeroCount,omitempty" json:"zeroCount,omitempty"`
	Scale           int32     `bson:"scale,omitempty" json:"scale,omitempty"`
	PositiveOffset  int32     `bson:"positiveOffset,omitempty" json:"positiveOffset,omitempty"`
	PositiveBuckets []uint64  `bson:"positiveBuckets,omitempty" json:"positiveBuckets,omitempty"`
	NegativeOffset  int32     `bson:"negativeOffset,omitempty" json:"negativeOffset,omitempty"`
	NegativeBuckets []uint64  `bson:"negativeBuckets,omitempty" json:"negativeBuckets,omitempty"`
}

type MetricPoint struct {
	ID           primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	TenantID     primitive.ObjectID  `bson:"tenantId" json:"tenantId"`
	ProjectID    primitive.ObjectID  `bson:"projectId" json:"projectId"`
	Name         string              `bson:"name" json:"name"`
	Description  string              `bson:"description,omitempty" json:"description,omitempty"`
	Unit         string              `bson:"unit,omitempty" json:"unit,omitempty"`
	Type         string              `bson:"type" json:"type"`
	Service      string              `bson:"service,omitempty" json:"service,omitempty"`
	Environment  string              `bson:"environment,omitempty" json:"environment,omitempty"`
	Release      string              `bson:"release,omitempty" json:"release,omitempty"`
	Platform     string              `bson:"platform,omitempty" json:"platform,omitempty"`
	Timestamp    time.Time           `bson:"timestamp" json:"timestamp"`
	StartTime    time.Time           `bson:"startTime,omitempty" json:"startTime,omitempty"`
	Value        float64             `bson:"value" json:"value"`
	Count        uint64              `bson:"count,omitempty" json:"count,omitempty"`
	Sum          float64             `bson:"sum,omitempty" json:"sum,omitempty"`
	Min          *float64            `bson:"min,omitempty" json:"min,omitempty"`
	Max          *float64            `bson:"max,omitempty" json:"max,omitempty"`
	Temporality  string              `bson:"temporality,omitempty" json:"temporality,omitempty"`
	IsMonotonic  bool                `bson:"isMonotonic,omitempty" json:"isMonotonic,omitempty"`
	Distribution *MetricDistribution `bson:"distribution,omitempty" json:"distribution,omitempty"`
	Quantiles    []MetricQuantile    `bson:"quantiles,omitempty" json:"quantiles,omitempty"`
	Attributes   map[string]string   `bson:"attributes,omitempty" json:"attributes,omitempty"`
	Resource     map[string]any      `bson:"resource,omitempty" json:"resource,omitempty"`
	ReceivedAt   time.Time           `bson:"receivedAt" json:"receivedAt"`
}

// SourceMap stores a gzipped source map for a generated bundle. Content is
// never serialized to API responses.
type SourceMap struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID    primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID   primitive.ObjectID `bson:"projectId" json:"projectId"`
	Release     string             `bson:"release" json:"release"`
	File        string             `bson:"file" json:"file"`
	Size        int64              `bson:"size" json:"size"`
	SourceCount int                `bson:"sourceCount" json:"sourceCount"`
	Content     []byte             `bson:"content" json:"-"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// RoleRank returns a numeric rank for role comparisons.
func RoleRank(role string) int {
	switch role {
	case RoleOwner:
		return 4
	case RoleAdmin:
		return 3
	case RoleMember:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// AtLeast reports whether role is at least min.
func AtLeast(role, min string) bool {
	return RoleRank(role) >= RoleRank(min)
}

// AlertRule defines a metric threshold that is evaluated periodically.
type AlertRule struct {
	ID            primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	TenantID      primitive.ObjectID   `bson:"tenantId" json:"tenantId"`
	ProjectID     primitive.ObjectID   `bson:"projectId" json:"projectId"`
	Name          string               `bson:"name" json:"name"`
	Metric        string               `bson:"metric" json:"metric"`
	Service       string               `bson:"service,omitempty" json:"service,omitempty"`
	Aggregation   string               `bson:"aggregation" json:"aggregation"`
	Operator      string               `bson:"operator" json:"operator"`
	Threshold     float64              `bson:"threshold" json:"threshold"`
	WindowSeconds int                  `bson:"windowSeconds" json:"windowSeconds"`
	Severity      string               `bson:"severity" json:"severity"`
	ChannelIDs    []primitive.ObjectID `bson:"channelIds,omitempty" json:"channelIds,omitempty"`
	Enabled       bool                 `bson:"enabled" json:"enabled"`
	State         string               `bson:"state" json:"state"`
	Value         float64              `bson:"value" json:"value"`
	HasData       bool                 `bson:"hasData" json:"hasData"`
	LastEvaluated time.Time            `bson:"lastEvaluated,omitempty" json:"lastEvaluated,omitempty"`
	FiredAt       *time.Time           `bson:"firedAt,omitempty" json:"firedAt,omitempty"`
	CreatedAt     time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time            `bson:"updatedAt" json:"updatedAt"`
}

// Alert is one firing episode of an alert rule.
type Alert struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID   primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID  primitive.ObjectID `bson:"projectId" json:"projectId"`
	RuleID     primitive.ObjectID `bson:"ruleId" json:"ruleId"`
	RuleName   string             `bson:"ruleName" json:"ruleName"`
	Metric     string             `bson:"metric" json:"metric"`
	Service    string             `bson:"service,omitempty" json:"service,omitempty"`
	Severity   string             `bson:"severity" json:"severity"`
	State      string             `bson:"state" json:"state"`
	Operator   string             `bson:"operator" json:"operator"`
	Threshold  float64            `bson:"threshold" json:"threshold"`
	Value      float64            `bson:"value" json:"value"`
	Message    string             `bson:"message" json:"message"`
	StartedAt  time.Time          `bson:"startedAt" json:"startedAt"`
	ResolvedAt *time.Time         `bson:"resolvedAt,omitempty" json:"resolvedAt,omitempty"`
	UpdatedAt  time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// NotificationChannel is a tenant-level destination for alert notifications.
type NotificationChannel struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID         primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	Name             string             `bson:"name" json:"name"`
	Type             string             `bson:"type" json:"type"`
	Enabled          bool               `bson:"enabled" json:"enabled"`
	WebhookURL       string             `bson:"webhookUrl,omitempty" json:"webhookUrl,omitempty"`
	WebhookSecretEnc string             `bson:"webhookSecretEnc,omitempty" json:"-"`
	WebhookSecretSet bool               `bson:"-" json:"secretSet"`
	Recipients       []string           `bson:"recipients,omitempty" json:"recipients,omitempty"`
	CreatedAt        time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// NotificationDelivery is a queued alert notification with retry metadata.
type NotificationDelivery struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID      primitive.ObjectID `bson:"tenantId" json:"tenantId"`
	ProjectID     primitive.ObjectID `bson:"projectId" json:"projectId"`
	AlertID       primitive.ObjectID `bson:"alertId" json:"alertId"`
	RuleID        primitive.ObjectID `bson:"ruleId" json:"ruleId"`
	ChannelID     primitive.ObjectID `bson:"channelId" json:"channelId"`
	ChannelName   string             `bson:"channelName" json:"channelName"`
	ChannelType   string             `bson:"channelType" json:"channelType"`
	Event         string             `bson:"event" json:"event"`
	Status        string             `bson:"status" json:"status"`
	Attempts      int                `bson:"attempts" json:"attempts"`
	StatusCode    int                `bson:"statusCode,omitempty" json:"statusCode,omitempty"`
	Error         string             `bson:"error,omitempty" json:"error,omitempty"`
	Payload       string             `bson:"payload" json:"-"`
	NextAttemptAt time.Time          `bson:"nextAttemptAt" json:"nextAttemptAt"`
	SentAt        *time.Time         `bson:"sentAt,omitempty" json:"sentAt,omitempty"`
	CreatedAt     time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt" json:"updatedAt"`
}
