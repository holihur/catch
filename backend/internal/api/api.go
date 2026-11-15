package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/auth"
	"github.com/catch/catch/backend/internal/config"
	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/logging"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/catch/catch/backend/internal/secure"
	"github.com/catch/catch/backend/internal/sourcemap"
	"github.com/catch/catch/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Version is shown in the UI and health endpoint.
const Version = "1.0.0"

// API wires HTTP handlers to storage, auth and logging.
type API struct {
	cfg        config.Config
	store      *store.Store
	logger     *logging.Logger
	oidc       *auth.Service
	box        *secure.Box
	sourcemaps *sourcemap.Service
	limiter    *rateLimiter
	startedAt  time.Time
}

// New creates the API.
func New(cfg config.Config, st *store.Store, logger *logging.Logger) (*API, error) {
	box, err := secure.NewBox(cfg.AppSecret)
	if err != nil {
		return nil, err
	}
	return &API{
		cfg:        cfg,
		store:      st,
		logger:     logger,
		oidc:       auth.NewService(st, box, cfg.PublicURL),
		box:        box,
		sourcemaps: sourcemap.New(st),
		limiter:    newRateLimiter(cfg.RateLimit),
		startedAt:  time.Now().UTC(),
	}, nil
}

// Routes builds the HTTP router.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(holderMiddleware)
	r.Use(httpx.Recover(a.logger))
	r.Use(httpx.AccessLog(a.logger))
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.CORS(a.cfg.CORSOrigins, true))
	r.Use(httpx.CheckOrigin(a.cfg.CORSOrigins))
	r.Use(httpx.NoCache)

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", a.health)
		api.Get("/version", a.version)

		api.Route("/auth", func(ar chi.Router) {
			ar.Get("/tenants", a.listPublicTenants)
			ar.Get("/tenants/{slug}", a.publicTenant)
			ar.Get("/oidc/start", a.oidcStart)
			ar.Get("/oidc/callback", a.oidcCallback)
			ar.Post("/dev", a.devLogin)
			ar.Post("/logout", a.logout)
		})

		// Ingestion is authenticated by project ingest key, not a session.
		api.Post("/ingest", a.ingest)
		api.Post("/ingest/sourcemaps", a.uploadSourceMapWithKey)
		api.Post("/ingest/otlp/v1/logs", a.otlpLogs)
		api.Post("/ingest/otlp/v1/traces", a.otlpTraces)

		api.Group(func(pr chi.Router) {
			pr.Use(a.authenticate)
			pr.Get("/auth/me", a.me)
			pr.Get("/overview", a.overview)
			pr.Get("/insights", a.insights)
			pr.Get("/events", a.listEvents)
			pr.Get("/events/{id}", a.getEvent)
			pr.Get("/issues", a.listIssues)
			pr.Get("/issues/{id}", a.getIssue)
			pr.Get("/members", a.listMembers)
			pr.Get("/tenant", a.getTenant)
			pr.Get("/projects", a.listProjects)
			pr.Get("/projects/{id}/source", a.getSourceFile)

			pr.Group(func(wr chi.Router) {
				wr.Use(requireRole(model.RoleMember))
				wr.Patch("/issues/{id}", a.updateIssue)
				wr.Post("/issues/{id}/comments", a.addComment)
			})

			pr.Group(func(ar chi.Router) {
				ar.Use(requireRole(model.RoleAdmin))
				ar.Post("/projects", a.createProject)
				ar.Patch("/projects/{id}", a.updateProject)
				ar.Post("/projects/{id}/rotate-key", a.rotateProjectKey)
				ar.Delete("/projects/{id}", a.deleteProject)
				ar.Patch("/members/{id}", a.updateMember)
				ar.Delete("/members/{id}", a.deleteMember)
				ar.Get("/projects/{id}/sourcemaps", a.listSourceMaps)
				ar.Post("/projects/{id}/sourcemaps", a.uploadSourceMap)
				ar.Delete("/projects/{id}/sourcemaps/{mapId}", a.deleteSourceMap)
				ar.Patch("/tenant", a.updateTenant)
				ar.Put("/tenant/oidc", a.updateOIDC)
			})
		})
	})

	// OTLP default paths (otlphttp exporter appends /v1/logs and /v1/traces
	// to its configured endpoint). These are key-authenticated like ingest.
	r.Post("/v1/logs", a.otlpLogs)
	r.Post("/v1/traces", a.otlpTraces)

	r.NotFound(a.notFound)
	return r
}

func holderMiddleware(next http.Handler) http.Handler {
	h := &reqctx.Holder{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(reqctx.WithHolder(r.Context(), h)))
	})
}

// authenticate loads the session, user and tenant into the request context.
func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(a.cfg.CookieName)
		if err != nil || cookie.Value == "" {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "请先登录")
			return
		}
		hash := secure.HashToken(cookie.Value)
		sess, err := a.store.SessionByTokenHash(r.Context(), hash)
		if err != nil {
			if store.IsNoDocuments(err) {
				httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "登录已过期，请重新登录")
				return
			}
			a.logger.Slog.ErrorContext(r.Context(), "load session failed", "error", err)
			httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载会话")
			return
		}
		user, err := a.store.UserByID(r.Context(), sess.TenantID, sess.UserID)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "用户已不存在")
			return
		}
		tenant, err := a.store.TenantByID(r.Context(), sess.TenantID)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "租户不存在")
			return
		}
		ctx := reqctx.WithSessionHash(r.Context(), hash)
		ctx = reqctx.WithUser(ctx, user)
		ctx = reqctx.WithTenant(ctx, tenant)
		if h := reqctx.HolderFrom(ctx); h != nil {
			h.SetTenant(tenant)
			h.SetUser(user)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireRole(min string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := reqctx.User(r.Context())
			if !ok || !model.AtLeast(u.Role, min) {
				httpx.Error(w, http.StatusForbidden, "forbidden", "当前角色没有执行该操作的权限")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	status := "ok"
	code := http.StatusOK
	if err := a.store.Client.Ping(ctx, nil); err != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	httpx.JSON(w, code, map[string]any{
		"status":           status,
		"version":          Version,
		"uptimeSec":        int64(time.Since(a.startedAt).Seconds()),
		"now":              time.Now().UTC(),
		"logDropped":       a.logger.Dropped(),
		"logPersistFailed": a.logger.Failed(),
	})
}

func (a *API) version(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"version": Version, "env": a.cfg.Env,
		"devLogin": a.cfg.AllowDev, "time": time.Now().UTC(),
	})
}

func (a *API) notFound(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		httpx.Error(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	a.serveSPA(w, r)
}

// ---- helpers ----

func currentTenant(r *http.Request) model.Tenant {
	t, _ := reqctx.Tenant(r.Context())
	return t
}

func currentUser(r *http.Request) model.User {
	u, _ := reqctx.User(r.Context())
	return u
}

func parseID(v string) (primitive.ObjectID, error) {
	id, err := primitive.ObjectIDFromHex(strings.TrimSpace(v))
	if err != nil {
		return primitive.NilObjectID, errors.New("无效的 ID")
	}
	return id, nil
}

func pathID(r *http.Request, key string) (primitive.ObjectID, error) {
	return parseID(chi.URLParam(r, key))
}

// projectFilter resolves the optional ?projectId= query against the tenant.
func (a *API) projectFilter(r *http.Request) (*primitive.ObjectID, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("projectId"))
	if raw == "" || raw == "all" {
		return nil, nil
	}
	id, err := parseID(raw)
	if err != nil {
		return nil, err
	}
	t := currentTenant(r)
	if _, err := a.store.ProjectByID(r.Context(), t.ID, id); err != nil {
		if store.IsNoDocuments(err) {
			return nil, errors.New("项目不存在或不属于当前租户")
		}
		return nil, err
	}
	return &id, nil
}

// timeRange parses ?range=24h|7d|30d or explicit ?from=&to= RFC3339 values.
func timeRange(r *http.Request, defaultRange string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	q := r.URL.Query()
	if f := q.Get("from"); f != "" {
		from, err := time.Parse(time.RFC3339, f)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("from 必须是 RFC3339 时间")
		}
		to := now
		if t := q.Get("to"); t != "" {
			to, err = time.Parse(time.RFC3339, t)
			if err != nil {
				return time.Time{}, time.Time{}, errors.New("to 必须是 RFC3339 时间")
			}
		}
		if to.Before(from) {
			return time.Time{}, time.Time{}, errors.New("to 必须晚于 from")
		}
		if to.Sub(from) > 90*24*time.Hour {
			return time.Time{}, time.Time{}, errors.New("时间范围不能超过 90 天")
		}
		return from.UTC(), to.UTC(), nil
	}
	rangeStr := q.Get("range")
	if rangeStr == "" {
		rangeStr = defaultRange
	}
	var d time.Duration
	switch rangeStr {
	case "1h":
		d = time.Hour
	case "6h":
		d = 6 * time.Hour
	case "24h":
		d = 24 * time.Hour
	case "7d":
		d = 7 * 24 * time.Hour
	case "30d":
		d = 30 * 24 * time.Hour
	default:
		return time.Time{}, time.Time{}, errors.New("range 必须是 1h、6h、24h、7d 或 30d")
	}
	return now.Add(-d), now, nil
}

func pageParams(r *http.Request, defaultSize int) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if size < 1 {
		size = defaultSize
	}
	if size > 200 {
		size = 200
	}
	return page, size
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "project"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

// oidcErrorRedirect sends the browser back to the login page with a short code.
func (a *API) oidcErrorRedirect(w http.ResponseWriter, r *http.Request, code string) {
	u := a.cfg.AppURL + "/login?error=" + url.QueryEscape(code)
	http.Redirect(w, r, u, http.StatusFound)
}

func (a *API) startSession(w http.ResponseWriter, r *http.Request, tenant model.Tenant, user model.User) error {
	token := secure.RandomToken(32)
	now := time.Now().UTC()
	sess := model.Session{
		TokenHash: secure.HashToken(token),
		UserID:    user.ID,
		TenantID:  tenant.ID,
		ExpiresAt: now.Add(a.cfg.SessionTTL),
		CreatedAt: now,
	}
	if err := a.store.CreateSession(r.Context(), sess); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
		MaxAge:   int(a.cfg.SessionTTL.Seconds()),
	})
	return nil
}

func (a *API) clearSession(w http.ResponseWriter, r *http.Request) {
	if hash := reqctx.SessionHash(r.Context()); hash != "" {
		_ = a.store.DeleteSession(r.Context(), hash)
	}
	if cookie, err := r.Cookie(a.cfg.CookieName); err == nil && cookie.Value != "" {
		_ = a.store.DeleteSession(r.Context(), secure.HashToken(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name: a.cfg.CookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func (a *API) logError(r *http.Request, msg string, err error) {
	a.logger.Slog.ErrorContext(r.Context(), msg, "error", fmt.Sprint(err))
}
