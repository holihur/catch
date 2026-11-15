// Package auth implements per-tenant OIDC login with PKCE and nonce checks.
package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/secure"
	"github.com/catch/catch/backend/internal/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var (
	ErrOIDCDisabled = errors.New("该租户未启用 OIDC 登录")
	ErrDomainDenied = errors.New("该邮箱域名不在租户允许范围内")
	ErrNoEmail      = errors.New("OIDC 身份提供方未返回邮箱")
	ErrBadState     = errors.New("登录状态已失效，请重新登录")
)

// Service performs OIDC discovery, code exchange and identity verification.
type Service struct {
	store     *store.Store
	box       *secure.Box
	publicURL string
	client    *httpClient

	mu    sync.Mutex
	cache map[string]*oidc.Provider
}

// NewService creates an OIDC service. publicURL is the externally reachable
// API base URL used to build the callback redirect URI.
func NewService(st *store.Store, box *secure.Box, publicURL string) *Service {
	return &Service{
		store:     st,
		box:       box,
		publicURL: strings.TrimRight(publicURL, "/"),
		client:    newHTTPClient(),
		cache:     map[string]*oidc.Provider{},
	}
}

// Enabled reports whether a tenant has a usable OIDC configuration.
func Enabled(t model.Tenant) bool {
	return t.OIDC != nil && t.OIDC.Enabled && t.OIDC.Issuer != "" && t.OIDC.ClientID != ""
}

func (s *Service) provider(ctx context.Context, t model.Tenant) (*oidc.Provider, error) {
	if !Enabled(t) {
		return nil, ErrOIDCDisabled
	}
	key := t.OIDC.Issuer + "|" + t.OIDC.ClientID
	s.mu.Lock()
	if p, ok := s.cache[key]; ok {
		s.mu.Unlock()
		return p, nil
	}
	s.mu.Unlock()
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dctx = oidc.ClientContext(dctx, s.client.HTTP)
	p, err := oidc.NewProvider(dctx, t.OIDC.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery 失败: %w", err)
	}
	s.mu.Lock()
	s.cache[key] = p
	s.mu.Unlock()
	return p, nil
}

func (s *Service) oauthConfig(t model.Tenant, p *oidc.Provider) (*oauth2.Config, error) {
	secret, err := s.box.Decrypt(t.OIDC.ClientSecretEnc)
	if err != nil {
		return nil, fmt.Errorf("无法读取 OIDC client secret: %w", err)
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	for _, sc := range t.OIDC.Scopes {
		if sc != "" && !slices.Contains(scopes, sc) {
			scopes = append(scopes, sc)
		}
	}
	return &oauth2.Config{
		ClientID:     t.OIDC.ClientID,
		ClientSecret: secret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.publicURL + "/api/auth/oidc/callback",
		Scopes:       scopes,
	}, nil
}

// Start creates a login transaction and returns the provider authorization URL.
func (s *Service) Start(ctx context.Context, t model.Tenant, returnTo string) (string, error) {
	p, err := s.provider(ctx, t)
	if err != nil {
		return "", err
	}
	cfg, err := s.oauthConfig(t, p)
	if err != nil {
		return "", err
	}
	state := secure.RandomToken(24)
	nonce := secure.RandomToken(24)
	verifier := oauth2.GenerateVerifier()
	if !safeReturnTo(returnTo) {
		returnTo = "/"
	}
	st := model.OIDCState{
		StateHash: secure.HashToken(state),
		TenantID:  t.ID,
		Verifier:  verifier,
		Nonce:     nonce,
		ReturnTo:  returnTo,
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	if err := s.store.CreateOIDCState(ctx, st); err != nil {
		return "", err
	}
	url := cfg.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.AccessTypeOnline,
	)
	return url, nil
}

// Callback completes a login and returns the tenant-scoped user.
func (s *Service) Callback(ctx context.Context, code, state string) (model.Tenant, model.User, string, error) {
	if code == "" || state == "" {
		return model.Tenant{}, model.User{}, "", ErrBadState
	}
	st, err := s.store.ConsumeOIDCState(ctx, secure.HashToken(state))
	if err != nil {
		if store.IsNoDocuments(err) {
			return model.Tenant{}, model.User{}, "", ErrBadState
		}
		return model.Tenant{}, model.User{}, "", err
	}
	tenant, err := s.store.TenantByID(ctx, st.TenantID)
	if err != nil {
		return model.Tenant{}, model.User{}, "", err
	}
	p, err := s.provider(ctx, tenant)
	if err != nil {
		return tenant, model.User{}, "", err
	}
	cfg, err := s.oauthConfig(tenant, p)
	if err != nil {
		return tenant, model.User{}, "", err
	}
	exchangeCtx := oidc.ClientContext(ctx, s.client.HTTP)
	token, err := cfg.Exchange(exchangeCtx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return tenant, model.User{}, "", fmt.Errorf("OIDC code 交换失败: %w", err)
	}
	rawID, _ := token.Extra("id_token").(string)
	if rawID == "" {
		return tenant, model.User{}, "", errors.New("OIDC 响应缺少 id_token")
	}
	verifier := p.Verifier(&oidc.Config{ClientID: tenant.OIDC.ClientID})
	idToken, err := verifier.Verify(ctx, rawID)
	if err != nil {
		return tenant, model.User{}, "", fmt.Errorf("id_token 校验失败: %w", err)
	}
	var claims struct {
		Nonce             string `json:"nonce"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Picture           string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return tenant, model.User{}, "", fmt.Errorf("id_token claims 解析失败: %w", err)
	}
	if claims.Nonce != st.Nonce {
		return tenant, model.User{}, "", ErrBadState
	}
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if email == "" {
		return tenant, model.User{}, "", ErrNoEmail
	}
	if !emailAllowed(email, tenant.AllowedDomains) {
		return tenant, model.User{}, "", ErrDomainDenied
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	user, _, err := s.store.UpsertOIDCUser(ctx, tenant.ID, idToken.Subject, email, name, claims.Picture, tenant.DefaultRole)
	if err != nil {
		return tenant, model.User{}, "", err
	}
	return tenant, user, st.ReturnTo, nil
}

func emailAllowed(email string, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(d, "@")))
		if d == "" {
			continue
		}
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

func safeReturnTo(p string) bool {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.Contains(p, "\\") {
		return false
	}
	return true
}
