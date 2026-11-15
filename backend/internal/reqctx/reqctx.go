// Package reqctx carries request-scoped identity and tracing values.
package reqctx

import (
	"context"
	"sync"

	"github.com/catch/catch/backend/internal/model"
)

type key int

const (
	tenantKey key = iota
	userKey
	requestIDKey
	sessionKey
	projectKey
	holderKey
)

// Holder is a mutable identity box shared by outer middlewares (access logs)
// and the authentication middleware. The outer request context cannot be
// replaced by inner middleware, so auth writes into the holder instead.
type Holder struct {
	mu        sync.RWMutex
	tenant    model.Tenant
	user      model.User
	hasTenant bool
	hasUser   bool
}

func (h *Holder) SetTenant(t model.Tenant) {
	h.mu.Lock()
	h.tenant, h.hasTenant = t, true
	h.mu.Unlock()
}

func (h *Holder) Tenant() (model.Tenant, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.tenant, h.hasTenant
}

func (h *Holder) SetUser(u model.User) {
	h.mu.Lock()
	h.user, h.hasUser = u, true
	h.mu.Unlock()
}

func (h *Holder) User() (model.User, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.user, h.hasUser
}

func WithHolder(ctx context.Context, h *Holder) context.Context {
	return context.WithValue(ctx, holderKey, h)
}

func HolderFrom(ctx context.Context) *Holder {
	h, _ := ctx.Value(holderKey).(*Holder)
	return h
}

func WithTenant(ctx context.Context, t model.Tenant) context.Context {
	return context.WithValue(ctx, tenantKey, t)
}

func Tenant(ctx context.Context) (model.Tenant, bool) {
	if t, ok := ctx.Value(tenantKey).(model.Tenant); ok {
		return t, true
	}
	if h := HolderFrom(ctx); h != nil {
		return h.Tenant()
	}
	return model.Tenant{}, false
}

func WithUser(ctx context.Context, u model.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func User(ctx context.Context) (model.User, bool) {
	if u, ok := ctx.Value(userKey).(model.User); ok {
		return u, true
	}
	if h := HolderFrom(ctx); h != nil {
		return h.User()
	}
	return model.User{}, false
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func WithSessionHash(ctx context.Context, hash string) context.Context {
	return context.WithValue(ctx, sessionKey, hash)
}

func SessionHash(ctx context.Context) string {
	h, _ := ctx.Value(sessionKey).(string)
	return h
}

func WithProject(ctx context.Context, p model.Project) context.Context {
	return context.WithValue(ctx, projectKey, p)
}

func Project(ctx context.Context) (model.Project, bool) {
	p, ok := ctx.Value(projectKey).(model.Project)
	return p, ok
}
