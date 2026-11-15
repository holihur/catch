package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/catch/catch/backend/internal/store"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Options controls the logger.
type Options struct {
	Level     string
	Service   string
	Persist   bool
	QueueSize int
}

// Logger bundles the process logger and its persistence pipeline.
type Logger struct {
	Slog    *slog.Logger
	mongo   *mongoHandler
	dropped atomic.Int64
	failed  atomic.Int64
}

// New creates a JSON logger that mirrors info+ records into MongoDB as backend
// log events. Persistence is best-effort and never blocks request handling.
func New(st *store.Store, opts Options) *Logger {
	level := parseLevel(opts.Level)
	stdout := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	l := &Logger{}
	handler := slog.Handler(stdout)
	if opts.Persist && st != nil {
		l.mongo = newMongoHandler(st, level, opts.QueueSize, &l.dropped, &l.failed)
		handler = &fanout{handlers: []slog.Handler{stdout, l.mongo}}
	}
	base := slog.New(handler)
	if opts.Service != "" {
		base = base.With("service", opts.Service)
	}
	l.Slog = base
	return l
}

// NewText is useful for CLI/seed output.
func NewText(level string) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(level)}))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Close drains queued backend logs.
func (l *Logger) Close() {
	if l != nil && l.mongo != nil {
		l.mongo.Close()
	}
}

// Dropped returns the number of log records dropped due to backpressure.
func (l *Logger) Dropped() int64 { return l.dropped.Load() }

// Failed returns the number of log records that could not be persisted.
func (l *Logger) Failed() int64 { return l.failed.Load() }

// SyncLogger is the subset used by API handlers.
type SyncLogger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	DebugContext(ctx context.Context, msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

var _ SyncLogger = (*slog.Logger)(nil)

// ---- fanout handler ----

type fanout struct {
	handlers []slog.Handler
}

func (f *fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f *fanout) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range f.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (f *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		out[i] = h.WithAttrs(attrs)
	}
	return &fanout{handlers: out}
}

func (f *fanout) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		out[i] = h.WithGroup(name)
	}
	return &fanout{handlers: out}
}

// ---- mongo handler ----

type mongoCore struct {
	st        *store.Store
	minLevel  slog.Level
	ch        chan model.Event
	done      chan struct{}
	closeOnce sync.Once
	dropped   *atomic.Int64
	failed    *atomic.Int64
}

type mongoHandler struct {
	core   *mongoCore
	attrs  []slog.Attr
	groups []string
}

func newMongoHandler(st *store.Store, minLevel slog.Level, queue int, dropped, failed *atomic.Int64) *mongoHandler {
	if queue <= 0 {
		queue = 2048
	}
	core := &mongoCore{
		st: st, minLevel: minLevel,
		ch: make(chan model.Event, queue), done: make(chan struct{}),
		dropped: dropped, failed: failed,
	}
	go core.worker()
	return &mongoHandler{core: core}
}

func (c *mongoCore) worker() {
	defer close(c.done)
	for e := range c.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := c.st.InsertEvent(ctx, e)
		cancel()
		if err != nil {
			c.failed.Add(1)
		}
	}
}

func (h *mongoHandler) Close() {
	c := h.core
	c.closeOnce.Do(func() {
		close(c.ch)
		<-c.done
	})
}

func (h *mongoHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.core.minLevel
}

func (h *mongoHandler) Handle(ctx context.Context, r slog.Record) error {
	tenant, ok := reqctx.Tenant(ctx)
	if !ok {
		return nil
	}
	fields := map[string]any{}
	for _, a := range h.attrs {
		addAttr(fields, h.groups, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		addAttr(fields, h.groups, a)
		return true
	})
	reqID := reqctx.RequestID(ctx)
	projectID, _ := reqctx.Project(ctx)
	e := model.Event{
		TenantID:   tenant.ID,
		Source:     model.SourceBackend,
		Kind:       model.KindLog,
		Level:      strings.ToLower(r.Level.String()),
		Message:    r.Message,
		Timestamp:  r.Time.UTC(),
		ReceivedAt: time.Now().UTC(),
		Context:    fields,
		Request:    &model.EventRequest{RequestID: reqID},
	}
	if projectID.ID != primitive.NilObjectID {
		e.ProjectID = projectID.ID
	}
	if r.Level >= slog.LevelError {
		e.Level = "error"
	}
	select {
	case h.core.ch <- e:
	default:
		h.core.dropped.Add(1)
	}
	return nil
}

func (h *mongoHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &mongoHandler{
		core:   h.core,
		attrs:  append(append([]slog.Attr{}, h.attrs...), attrs...),
		groups: append([]string{}, h.groups...),
	}
}

func (h *mongoHandler) WithGroup(name string) slog.Handler {
	return &mongoHandler{
		core:   h.core,
		attrs:  append([]slog.Attr{}, h.attrs...),
		groups: append(append([]string{}, h.groups...), name),
	}
}

func addAttr(fields map[string]any, groups []string, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	key := a.Key
	if len(groups) > 0 {
		key = strings.Join(groups, ".") + "." + key
	}
	fields[key] = valueToAny(a.Value)
}

func valueToAny(v slog.Value) any {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindGroup:
		m := map[string]any{}
		for _, a := range v.Group() {
			m[a.Key] = valueToAny(a.Value)
		}
		return m
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	default:
		return v.Any()
	}
}

// Truncate caps a string to n bytes on a UTF-8 boundary.
func Truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n] + "…"
}
