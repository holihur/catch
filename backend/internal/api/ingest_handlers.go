package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/ingest"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/catch/catch/backend/internal/store"
)

const (
	maxIngestBytes = 1 << 20 // 1 MiB
	maxBatchSize   = 100
)

var clientFingerprintRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,128}$`)

type flexTime struct{ time.Time }

func (t *flexTime) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		str = strings.TrimSpace(str)
		if str == "" {
			return nil
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999", "2006-01-02 15:04:05"} {
			if v, err := time.Parse(layout, str); err == nil {
				t.Time = v
				return nil
			}
		}
		// Numeric timestamps are sometimes sent as strings.
		if n, err := strconv.ParseFloat(str, 64); err == nil {
			t.Time = fromUnix(n)
			return nil
		}
		return fmt.Errorf("无法解析时间 %q", str)
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("无法解析时间: %w", err)
	}
	t.Time = fromUnix(n)
	return nil
}

func fromUnix(n float64) time.Time {
	if n > 1e12 { // milliseconds
		return time.UnixMilli(int64(n))
	}
	return time.Unix(int64(n), 0)
}

type ingestError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

type ingestEvent struct {
	Kind        string              `json:"kind"`
	Type        string              `json:"type"`
	Source      string              `json:"source"`
	Level       string              `json:"level"`
	Message     string              `json:"message"`
	Timestamp   flexTime            `json:"timestamp"`
	Environment string              `json:"environment"`
	Release     string              `json:"release"`
	Service     string              `json:"service"`
	Platform    string              `json:"platform"`
	Fingerprint string              `json:"fingerprint"`
	ErrorType   string              `json:"errorType"`
	Error       *ingestError        `json:"error"`
	Culprit     string              `json:"culprit"`
	URL         string              `json:"url"`
	Stack       string              `json:"stack"`
	Tags        map[string]string   `json:"tags"`
	Context     map[string]any      `json:"context"`
	User        *model.EventUser    `json:"user"`
	Request     *model.EventRequest `json:"request"`
	Breadcrumbs []model.Breadcrumb  `json:"breadcrumbs"`
}

// ingest accepts JSON arrays, a single event, or {"events":[...]}.
func (a *API) ingest(w http.ResponseWriter, r *http.Request) {
	key := extractIngestKey(r)
	if key == "" {
		httpx.Error(w, http.StatusUnauthorized, "missing_ingest_key", "缺少项目采集密钥")
		return
	}
	project, err := a.store.ProjectByKey(r.Context(), key)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusUnauthorized, "invalid_ingest_key", "采集密钥无效")
			return
		}
		a.logError(r, "load ingest project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法校验采集密钥")
		return
	}
	tenant, err := a.store.TenantByID(r.Context(), project.TenantID)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_ingest_key", "采集密钥无效")
		return
	}
	ctx := reqctx.WithTenant(r.Context(), tenant)
	ctx = reqctx.WithProject(ctx, project)
	r = r.WithContext(ctx)

	body, err := readLimited(w, r, maxIngestBytes)
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error())
		return
	}
	rawEvents, err := splitPayload(body)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if len(rawEvents) == 0 {
		httpx.Error(w, http.StatusBadRequest, "empty_batch", "事件列表不能为空")
		return
	}
	if len(rawEvents) > maxBatchSize {
		httpx.Error(w, http.StatusBadRequest, "batch_too_large", fmt.Sprintf("单次最多提交 %d 条事件", maxBatchSize))
		return
	}
	if !a.limiter.allow(project.ID.Hex(), len(rawEvents)) {
		w.Header().Set("Retry-After", "60")
		a.logger.Slog.WarnContext(r.Context(), "ingest rate limited",
			"projectId", project.ID.Hex(), "batch", len(rawEvents), "limit", a.cfg.RateLimit)
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "采集速率超限，请稍后重试")
		return
	}

	now := time.Now().UTC()
	events := make([]model.Event, 0, len(rawEvents))
	for i, raw := range rawEvents {
		var in ingestEvent
		if err := json.Unmarshal(raw, &in); err != nil {
			httpx.ErrorDetails(w, http.StatusBadRequest, "invalid_event",
				"事件格式错误", map[string]any{"index": i, "error": err.Error()})
			return
		}
		e, err := a.normalizeEvent(tenant, project, in, now)
		if err != nil {
			httpx.ErrorDetails(w, http.StatusBadRequest, "invalid_event",
				err.Error(), map[string]any{"index": i})
			return
		}
		events = append(events, e)
	}
	created, reopened, err := a.persistEvents(r.Context(), tenant, project, events)
	if err != nil {
		a.logger.Slog.ErrorContext(r.Context(), "persist events failed", "error", err, "projectId", project.ID.Hex())
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存事件")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"accepted": len(events), "newIssues": created, "reopenedIssues": reopened,
	})
}

// persistEvents resolves source maps, groups errors into issues and inserts the
// event batch. It is shared by the native ingest API and OTLP receivers.
func (a *API) persistEvents(ctx context.Context, tenant model.Tenant, project model.Project, events []model.Event) (created, reopened int, err error) {
	for i := range events {
		e := &events[i]
		if e.Stack != "" {
			resolved, changed, smErr := a.sourcemaps.ResolveStack(ctx, tenant.ID, project.ID, e.Release, e.Stack)
			if smErr != nil {
				a.logger.Slog.WarnContext(ctx, "source map resolve failed",
					"projectId", project.ID.Hex(), "release", e.Release, "error", smErr)
			} else if changed {
				e.Stack = resolved
				e.SourceMap = true
				if c := ingest.Culprit(resolved); c != "" {
					e.Culprit = ingest.Truncate(c, 300)
				}
				if e.Kind == model.KindError {
					e.Fingerprint = ingest.Fingerprint(e.Source, e.ErrorType, e.Message, e.Culprit)
				}
			}
		}
		if e.Kind == model.KindError {
			issue, isNew, isReopened, ierr := a.store.UpsertIssue(ctx, model.Issue{
				TenantID: tenant.ID, ProjectID: project.ID,
				Fingerprint: e.Fingerprint, Title: ingest.Title(e.ErrorType, e.Message),
				Culprit: e.Culprit, Level: e.Level, Source: e.Source,
				FirstSeen: e.Timestamp, LastSeen: e.Timestamp,
			})
			if ierr != nil {
				return created, reopened, ierr
			}
			e.IssueID = issue.ID
			if isNew {
				created++
			}
			if isReopened {
				reopened++
			}
		}
	}
	if err := a.store.InsertEvents(ctx, events); err != nil {
		return created, reopened, err
	}
	return created, reopened, nil
}

func (a *API) normalizeEvent(tenant model.Tenant, project model.Project, in ingestEvent, now time.Time) (model.Event, error) {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind == "" {
		if strings.EqualFold(in.Type, "error") || in.Error != nil {
			kind = model.KindError
		} else {
			kind = model.KindLog
		}
	}
	if kind != model.KindLog && kind != model.KindError {
		return model.Event{}, errors.New("kind 必须是 log 或 error")
	}
	if strings.EqualFold(in.Type, "error") {
		kind = model.KindError
	}
	source := strings.ToLower(strings.TrimSpace(in.Source))
	if source == "" {
		source = model.SourceFrontend
	}
	if source != model.SourceFrontend && source != model.SourceBackend {
		return model.Event{}, errors.New("source 必须是 frontend 或 backend")
	}
	message := strings.TrimSpace(in.Message)
	stack := in.Stack
	errType := strings.TrimSpace(in.ErrorType)
	if in.Error != nil {
		if errType == "" {
			errType = strings.TrimSpace(in.Error.Name)
		}
		if stack == "" {
			stack = in.Error.Stack
		}
		if message == "" {
			message = strings.TrimSpace(in.Error.Message)
		}
	}
	if kind == model.KindError && message == "" {
		message = errType
	}
	if message == "" {
		return model.Event{}, errors.New("message 不能为空")
	}
	if errType == "" {
		errType = ingest.ErrorType(message)
	}
	culprit := strings.TrimSpace(in.Culprit)
	if culprit == "" {
		culprit = ingest.Culprit(stack)
	}
	level := ingest.DefaultLevel(in.Level, kind)
	ts := in.Timestamp.Time
	if ts.IsZero() || ts.After(now.Add(5*time.Minute)) {
		ts = now
	}
	if ts.Before(now.AddDate(-1, 0, 0)) {
		ts = now
	}
	tags := cleanTags(in.Tags)
	ctx := in.Context
	if len(ctx) > 0 {
		if b, err := json.Marshal(ctx); err != nil || len(b) > 16<<10 {
			ctx = map[string]any{"_truncated": true, "size": len(b)}
		}
	}
	breadcrumbs := in.Breadcrumbs
	if len(breadcrumbs) > 50 {
		breadcrumbs = breadcrumbs[len(breadcrumbs)-50:]
	}
	for i := range breadcrumbs {
		breadcrumbs[i].Message = ingest.Truncate(breadcrumbs[i].Message, 500)
	}

	e := model.Event{
		TenantID: tenant.ID, ProjectID: project.ID,
		Kind: kind, Source: source, Level: level,
		Message: ingest.Truncate(message, 8000), Timestamp: ts, ReceivedAt: now,
		Environment: ingest.Truncate(in.Environment, 100),
		Release:     ingest.Truncate(in.Release, 200),
		Service:     ingest.Truncate(in.Service, 200),
		Platform:    ingest.Truncate(in.Platform, 100),
		ErrorType:   ingest.Truncate(errType, 200),
		Culprit:     ingest.Truncate(culprit, 300),
		URL:         ingest.Truncate(in.URL, 2048),
		Stack:       ingest.Truncate(stack, 32000),
		Tags:        tags,
		Context:     ctx,
		User:        in.User,
		Request:     in.Request,
		Breadcrumbs: breadcrumbs,
	}
	if e.User != nil {
		e.User.ID = ingest.Truncate(e.User.ID, 200)
		e.User.Email = ingest.Truncate(e.User.Email, 320)
		e.User.Username = ingest.Truncate(e.User.Username, 200)
	}
	if e.Request != nil {
		e.Request.URL = ingest.Truncate(e.Request.URL, 2048)
		e.Request.RequestID = ingest.Truncate(e.Request.RequestID, 200)
	}
	if kind == model.KindLog {
		errType, culprit, stack = "", "", ""
	}
	if kind == model.KindError {
		fp := strings.TrimSpace(in.Fingerprint)
		if !clientFingerprintRe.MatchString(fp) {
			fp = ingest.Fingerprint(source, errType, message, culprit)
		}
		e.Fingerprint = fp
	}
	return e, nil
}

func cleanTags(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	i := 0
	for k, v := range in {
		if i >= 30 {
			break
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[ingest.Truncate(k, 64)] = ingest.Truncate(strings.TrimSpace(v), 500)
		i++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractIngestKey(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Ingest-Key")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("key")); v != "" {
		return v
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func readLimited(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r.Body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, fmt.Errorf("请求体超过 %d 字节限制", limit)
		}
		return nil, errors.New("无法读取请求体")
	}
	return buf.Bytes(), nil
}

func splitPayload(body []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, errors.New("请求体不能为空")
	}
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, errors.New("事件数组不是合法 JSON")
		}
		return arr, nil
	}
	if trimmed[0] != '{' {
		return nil, errors.New("请求体必须是 JSON 对象或数组")
	}
	var batch struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(trimmed, &batch); err != nil {
		return nil, errors.New("请求体不是合法 JSON")
	}
	if batch.Events != nil {
		return batch.Events, nil
	}
	return []json.RawMessage{trimmed}, nil
}
