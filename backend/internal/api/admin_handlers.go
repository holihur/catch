package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"go.mongodb.org/mongo-driver/bson"
)

func (a *API) listMembers(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	users, err := a.store.ListUsers(r.Context(), t.ID)
	if err != nil {
		a.logError(r, "list members failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法加载成员")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"members": users})
}

func (a *API) updateMember(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	actor := currentUser(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	role := strings.ToLower(strings.TrimSpace(body.Role))
	if model.RoleRank(role) < 1 || role == model.RoleOwner {
		httpx.Error(w, http.StatusBadRequest, "invalid_role", "角色必须是 viewer、member 或 admin；owner 只能通过转移流程设置")
		return
	}
	target, err := a.store.UserByID(r.Context(), t.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "member_not_found", "成员不存在")
		return
	}
	if target.Role == model.RoleOwner {
		httpx.Error(w, http.StatusForbidden, "owner_protected", "不能修改 owner 的角色")
		return
	}
	updated, err := a.store.UpdateUserRole(r.Context(), t.ID, id, role)
	if err != nil {
		a.logError(r, "update member role failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新角色")
		return
	}
	a.logger.Slog.InfoContext(r.Context(), "member role updated",
		"target", target.Email, "role", role, "actor", actor.Email)
	httpx.JSON(w, http.StatusOK, updated)
}

func (a *API) deleteMember(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	actor := currentUser(r)
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if actor.ID == id {
		httpx.Error(w, http.StatusBadRequest, "self_delete", "不能删除自己，请使用退出登录")
		return
	}
	target, err := a.store.UserByID(r.Context(), t.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "member_not_found", "成员不存在")
		return
	}
	if target.Role == model.RoleOwner {
		httpx.Error(w, http.StatusForbidden, "owner_protected", "不能删除 owner")
		return
	}
	if err := a.store.DeleteUserSessions(r.Context(), t.ID, id); err != nil {
		a.logError(r, "delete member sessions failed", err)
	}
	if err := a.store.DeleteUser(r.Context(), t.ID, id); err != nil {
		a.logError(r, "delete member failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法删除成员")
		return
	}
	a.logger.Slog.WarnContext(r.Context(), "member removed", "target", target.Email, "actor", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getTenant(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	hasSecret := t.OIDC != nil && t.OIDC.ClientSecretEnc != ""
	httpx.JSON(w, http.StatusOK, map[string]any{
		"tenant":    t,
		"hasSecret": hasSecret,
		"permissions": map[string]bool{
			"canManage": model.AtLeast(currentUser(r).Role, model.RoleAdmin),
		},
	})
}

func (a *API) updateTenant(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 100 {
		httpx.Error(w, http.StatusBadRequest, "invalid_name", "租户名称长度需在 1-100 之间")
		return
	}
	updated, err := a.store.UpdateTenant(r.Context(), t.ID, bson.M{"name": name})
	if err != nil {
		a.logError(r, "update tenant failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新租户")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (a *API) updateOIDC(w http.ResponseWriter, r *http.Request) {
	t := currentTenant(r)
	var body struct {
		Issuer         string   `json:"issuer"`
		ClientID       string   `json:"clientId"`
		ClientSecret   string   `json:"clientSecret"`
		Scopes         []string `json:"scopes"`
		Enabled        bool     `json:"enabled"`
		AllowedDomains []string `json:"allowedDomains"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	issuer := strings.TrimRight(strings.TrimSpace(body.Issuer), "/")
	clientID := strings.TrimSpace(body.ClientID)
	if body.Enabled {
		if issuer == "" || clientID == "" {
			httpx.Error(w, http.StatusBadRequest, "invalid_oidc", "启用 OIDC 时必须填写 issuer 和 clientId")
			return
		}
		u, err := url.Parse(issuer)
		if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && isLocalHost(u.Hostname()))) {
			httpx.Error(w, http.StatusBadRequest, "invalid_issuer", "issuer 必须是 https URL（本地开发允许 http://localhost）")
			return
		}
	}
	secretEnc := ""
	if t.OIDC != nil {
		secretEnc = t.OIDC.ClientSecretEnc
	}
	if s := strings.TrimSpace(body.ClientSecret); s != "" {
		enc, err := a.box.Encrypt(s)
		if err != nil {
			a.logError(r, "encrypt oidc secret failed", err)
			httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存密钥")
			return
		}
		secretEnc = enc
	}
	if body.Enabled && secretEnc == "" {
		httpx.Error(w, http.StatusBadRequest, "missing_secret", "启用 OIDC 时必须提供 clientSecret")
		return
	}
	scopes := cleanScopes(body.Scopes)
	domains := cleanDomains(body.AllowedDomains)
	oidc := &model.OIDCConfig{
		Issuer: issuer, ClientID: clientID, ClientSecretEnc: secretEnc,
		Scopes: scopes, Enabled: body.Enabled,
	}
	updated, err := a.store.UpdateTenant(r.Context(), t.ID, bson.M{
		"oidc": oidc, "allowedDomains": domains,
	})
	if err != nil {
		a.logError(r, "update oidc failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法更新 OIDC 配置")
		return
	}
	a.logger.Slog.InfoContext(r.Context(), "tenant oidc updated",
		"enabled", body.Enabled, "issuer", issuer, "actor", currentUser(r).Email)
	httpx.JSON(w, http.StatusOK, map[string]any{"tenant": updated, "hasSecret": secretEnc != ""})
}

func cleanScopes(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{"openid": true, "profile": true, "email": true}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || len(s) > 80 || seen[s] || len(out) >= 10 {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func cleanDomains(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(d, "@")))
		if d == "" || len(d) > 100 || seen[d] || len(out) >= 20 {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func isLocalHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
