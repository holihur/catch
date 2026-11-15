package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/catch/catch/backend/internal/auth"
	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

type publicTenant struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	LogoURL     string `json:"logoUrl,omitempty"`
	OIDCEnabled bool   `json:"oidcEnabled"`
	DevEnabled  bool   `json:"devEnabled"`
}

func (a *API) toPublicTenant(t model.Tenant) publicTenant {
	return publicTenant{
		Slug: t.Slug, Name: t.Name, LogoURL: t.LogoURL,
		OIDCEnabled: auth.Enabled(t), DevEnabled: a.cfg.AllowDev,
	}
}

func (a *API) listPublicTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := a.store.ListTenants(r.Context())
	if err != nil {
		a.logError(r, "list tenants failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载租户列表")
		return
	}
	out := make([]publicTenant, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, a.toPublicTenant(t))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tenants": out})
}

func (a *API) publicTenant(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "slug")))
	t, err := a.store.TenantBySlug(r.Context(), slug)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "tenant_not_found", "租户不存在")
		return
	}
	httpx.JSON(w, http.StatusOK, a.toPublicTenant(t))
}

// The tiny wrapper keeps handlers independent from chi in tests.

func (a *API) oidcStart(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tenant")))
	if slug == "" {
		httpx.Error(w, http.StatusBadRequest, "missing_tenant", "请提供租户标识")
		return
	}
	t, err := a.store.TenantBySlug(r.Context(), slug)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "tenant_not_found", "租户不存在")
		return
	}
	if !auth.Enabled(t) {
		httpx.Error(w, http.StatusBadRequest, "oidc_disabled", "该租户未配置 OIDC 登录")
		return
	}
	returnTo := r.URL.Query().Get("returnTo")
	url, err := a.oidc.Start(r.Context(), t, returnTo)
	if err != nil {
		a.logger.Slog.ErrorContext(r.Context(), "oidc start failed", "tenant", t.Slug, "error", err)
		httpx.Error(w, http.StatusBadGateway, "oidc_start_failed", "无法启动 OIDC 登录，请检查身份提供方配置")
		return
	}
	a.logger.Slog.InfoContext(reqctx.WithTenant(r.Context(), t), "oidc login started", "tenant", t.Slug)
	httpx.JSON(w, http.StatusOK, map[string]string{"url": url})
}

func (a *API) oidcCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		a.logger.Slog.WarnContext(r.Context(), "oidc provider error", "error", e, "description", q.Get("error_description"))
		a.oidcErrorRedirect(w, r, "provider_error")
		return
	}
	tenant, user, returnTo, err := a.oidc.Callback(r.Context(), q.Get("code"), q.Get("state"))
	if err != nil {
		code := "oidc_failed"
		switch {
		case errors.Is(err, auth.ErrBadState):
			code = "invalid_state"
		case errors.Is(err, auth.ErrDomainDenied):
			code = "domain_denied"
		case errors.Is(err, auth.ErrNoEmail):
			code = "no_email"
		}
		a.logger.Slog.ErrorContext(r.Context(), "oidc callback failed", "tenant", tenant.Slug, "code", code, "error", err)
		a.oidcErrorRedirect(w, r, code)
		return
	}
	if err := a.startSession(w, r, tenant, user); err != nil {
		a.logger.Slog.ErrorContext(r.Context(), "create session failed", "error", err)
		a.oidcErrorRedirect(w, r, "session_failed")
		return
	}
	a.logger.Slog.InfoContext(reqctx.WithTenant(r.Context(), tenant), "oidc login succeeded",
		"tenant", tenant.Slug, "user", user.Email, "role", user.Role)
	if returnTo == "" {
		returnTo = "/"
	}
	http.Redirect(w, r, a.cfg.AppURL+returnTo, http.StatusFound)
}

func (a *API) devLogin(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.AllowDev {
		httpx.Error(w, http.StatusNotFound, "dev_login_disabled", "开发登录未启用")
		return
	}
	var body struct {
		Tenant string `json:"tenant"`
		Email  string `json:"email"`
		Name   string `json:"name"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	body.Tenant = strings.ToLower(strings.TrimSpace(body.Tenant))
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	body.Name = strings.TrimSpace(body.Name)
	if body.Tenant == "" || !strings.Contains(body.Email, "@") {
		httpx.Error(w, http.StatusBadRequest, "invalid_input", "请填写有效的租户和邮箱")
		return
	}
	if body.Name == "" {
		body.Name = strings.Split(body.Email, "@")[0]
	}
	t, err := a.store.TenantBySlug(r.Context(), body.Tenant)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "tenant_not_found", "租户不存在")
		return
	}
	u, err := a.store.DevUser(r.Context(), t.ID, body.Email, body.Name)
	if err != nil {
		a.logError(r, "dev login failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "开发登录失败")
		return
	}
	if err := a.startSession(w, r, t, u); err != nil {
		a.logError(r, "create session failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法创建会话")
		return
	}
	a.logger.Slog.InfoContext(reqctx.WithTenant(r.Context(), t), "dev login", "tenant", t.Slug, "user", u.Email, "role", u.Role)
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u, "tenant": t})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	t := currentTenant(r)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user":        u,
		"tenant":      t,
		"permissions": map[string]bool{"canManage": model.AtLeast(u.Role, model.RoleAdmin), "canWrite": model.AtLeast(u.Role, model.RoleMember)},
		"oidcEnabled": auth.Enabled(t),
		"devLogin":    a.cfg.AllowDev,
		"version":     Version,
	})
}
