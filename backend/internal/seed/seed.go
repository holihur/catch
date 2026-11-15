// Package seed provisions a demo tenant and optional OIDC bootstrap config.
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/auth"
	"github.com/catch/catch/backend/internal/config"
	"github.com/catch/catch/backend/internal/ingest"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/secure"
	"github.com/catch/catch/backend/internal/store"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Run applies the optional OIDC bootstrap tenant and the demo dataset.
func Run(ctx context.Context, st *store.Store, cfg config.Config, logger *slog.Logger) error {
	box, err := secure.NewBox(cfg.AppSecret)
	if err != nil {
		return err
	}
	if cfg.OIDCTenantSlug != "" && cfg.OIDCIssuer != "" && cfg.OIDCClientID != "" {
		enc, err := box.Encrypt(cfg.OIDCClientSecret)
		if err != nil {
			return fmt.Errorf("encrypt oidc secret: %w", err)
		}
		slug := strings.ToLower(cfg.OIDCTenantSlug)
		t, err := st.UpsertTenant(ctx, model.Tenant{
			Slug: slug,
			Name: titleCase(slug),
			OIDC: &model.OIDCConfig{
				Issuer: cfg.OIDCIssuer, ClientID: cfg.OIDCClientID,
				ClientSecretEnc: enc, Enabled: true,
				Scopes: []string{"openid", "profile", "email"},
			},
			AllowedDomains: cfg.OIDCAllowedDomains,
			DefaultRole:    model.RoleMember,
		})
		if err != nil {
			return fmt.Errorf("bootstrap oidc tenant: %w", err)
		}
		logger.Info("OIDC tenant configured", "tenant", t.Slug, "issuer", cfg.OIDCIssuer)
	}
	if !cfg.SeedDemo {
		return nil
	}
	return seedDemo(ctx, st, logger)
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func seedDemo(ctx context.Context, st *store.Store, logger *slog.Logger) error {
	t, err := st.TenantBySlug(ctx, "acme")
	if store.IsNoDocuments(err) {
		t, err = st.UpsertTenant(ctx, model.Tenant{
			Slug: "acme", Name: "Acme Inc.",
			AllowedDomains: nil, DefaultRole: model.RoleMember,
		})
	}
	if err != nil {
		return err
	}
	projects, err := st.ListProjects(ctx, t.ID)
	if err != nil {
		return err
	}
	var web, api model.Project
	for _, p := range projects {
		switch p.Slug {
		case "web":
			web = p
		case "api":
			api = p
		}
	}
	if web.ID.IsZero() {
		web, err = st.CreateProject(ctx, t.ID, "Web App", "web", "javascript-react")
		if err != nil {
			return err
		}
		// Deterministic keys make the bundled frontend SDK work out of the
		// box. Only set them on creation so key rotation is never overwritten.
		web, err = st.UpdateProject(ctx, t.ID, web.ID, bson.M{
			"ingestKey":    "ck_demo_web_0000000000000000000000",
			"repoProvider": "github",
			"repoUrl":      "https://github.com/holihur/catch",
			"repoBranch":   "master",
			"repoRoot":     "frontend",
		})
		if err != nil {
			return fmt.Errorf("set demo web key: %w", err)
		}
	}
	if api.ID.IsZero() {
		api, err = st.CreateProject(ctx, t.ID, "Orders API", "api", "go")
		if err != nil {
			return err
		}
		api, err = st.UpdateProject(ctx, t.ID, api.ID, bson.M{
			"ingestKey":    "ck_demo_api_0000000000000000000000",
			"repoProvider": "github",
			"repoUrl":      "https://github.com/holihur/catch",
			"repoBranch":   "master",
			"repoRoot":     "backend",
		})
		if err != nil {
			return fmt.Errorf("set demo api key: %w", err)
		}
	}

	// Ensure a stable dev owner exists.
	if _, err := st.DevUser(ctx, t.ID, "demo@acme.dev", "Demo Owner"); err != nil {
		return err
	}
	if _, err := st.DevUser(ctx, t.ID, "dev@acme.dev", "Lin Chen"); err != nil {
		return err
	}

	count, err := st.CountEvents(ctx, bson.M{"tenantId": t.ID})
	if err != nil {
		return err
	}
	if count > 0 {
		logger.Info("demo tenant already seeded", "tenant", t.Slug, "projects", 2)
		return nil
	}

	now := time.Now().UTC()
	type tpl struct {
		title    string
		message  string
		errType  string
		level    string
		source   string
		project  model.Project
		culprit  string
		stack    string
		status   string
		resolved bool
		ignored  bool
		regress  int
		base     int // base events per day
	}
	templates := []tpl{
		{
			title:   "TypeError: Cannot read properties of undefined (reading 'map')",
			message: "TypeError: Cannot read properties of undefined (reading 'map')\n    at renderOrderList (http://localhost:5173/src/pages/Orders.tsx:88:24)\n    at renderWithHooks (node_modules/react-dom/cjs/react-dom.development.js:14985:18)",
			errType: "TypeError", level: "error", source: model.SourceFrontend, project: web,
			culprit: "renderOrderList (Orders.tsx:88)", status: model.IssueOpen, base: 14,
			stack: "TypeError: Cannot read properties of undefined (reading 'map')\n    at renderOrderList (http://localhost:5173/src/pages/Orders.tsx:88:24)\n    at renderWithHooks (node_modules/react-dom/cjs/react-dom.development.js:14985:18)",
		},
		{
			title:   "APIError: 502 Bad Gateway from checkout-service",
			message: "APIError: upstream checkout-service returned 502 for POST /v1/checkout",
			errType: "APIError", level: "error", source: model.SourceBackend, project: api,
			culprit: "checkout.Client.Do (client.go:112)", status: model.IssueOpen, base: 9,
			stack: "APIError: upstream checkout-service returned 502\n    at checkout.Client.Do (internal/checkout/client.go:112)\n    at http.HandlerFunc.ServeHTTP (internal/api/checkout.go:48)",
		},
		{
			title:   "MongoServerError: E11000 duplicate key error collection: shop.orders",
			message: "MongoServerError: E11000 duplicate key error collection: shop.orders index: uniq_order_no",
			errType: "MongoServerError", level: "fatal", source: model.SourceBackend, project: api,
			culprit: "orders.Repository.Insert (repository.go:78)", status: model.IssueOpen, base: 4,
			stack: "MongoServerError: E11000 duplicate key error collection: shop.orders\n    at orders.Repository.Insert (internal/orders/repository.go:78)\n    at orders.Service.Create (internal/orders/service.go:41)",
		},
		{
			title:   "RangeError: Maximum update depth exceeded",
			message: "RangeError: Maximum update depth exceeded. This can happen when a component repeatedly calls setState inside componentWillUpdate.",
			errType: "RangeError", level: "error", source: model.SourceFrontend, project: web,
			culprit: "Checkout.useEffect (Checkout.tsx:57)", status: model.IssueResolved, resolved: true, base: 6,
			stack: "RangeError: Maximum update depth exceeded\n    at Checkout.useEffect (http://localhost:5173/src/pages/Checkout.tsx:57:5)\n    at commitHookEffectListMount (node_modules/react-dom/cjs/react-dom.development.js:23150:26)",
		},
		{
			title:   "TimeoutError: request to /api/orders timed out after 10000ms",
			message: "TimeoutError: request to /api/orders timed out after 10000ms",
			errType: "TimeoutError", level: "warn", source: model.SourceFrontend, project: web,
			culprit: "fetchJSON (client.ts:33)", status: model.IssueOpen, base: 11,
			stack: "TimeoutError: request to /api/orders timed out after 10000ms\n    at fetchJSON (http://localhost:5173/src/lib/client.ts:33:9)\n    at Orders.load (http://localhost:5173/src/pages/Orders.tsx:31:20)",
		},
		{
			title:   "NotFoundError: order <n> not found",
			message: "NotFoundError: order 88213 not found",
			errType: "NotFoundError", level: "error", source: model.SourceBackend, project: api,
			culprit: "orders.Service.Get (service.go:66)", status: model.IssueResolved, resolved: true, base: 3,
			stack: "NotFoundError: order not found\n    at orders.Service.Get (internal/orders/service.go:66)\n    at http.HandlerFunc.ServeHTTP (internal/api/orders.go:91)",
		},
		{
			title:   "AuthError: token expired while refreshing session",
			message: "AuthError: token expired while refreshing session",
			errType: "AuthError", level: "warn", source: model.SourceFrontend, project: web,
			culprit: "refreshSession (auth.ts:72)", status: model.IssueOpen, regress: 2, base: 5,
			stack: "AuthError: token expired while refreshing session\n    at refreshSession (http://localhost:5173/src/lib/auth.ts:72:11)\n    at AuthProvider.useEffect (http://localhost:5173/src/context/Auth.tsx:44:7)",
		},
		{
			title:   "HydrationError: text content does not match server-rendered HTML",
			message: "HydrationError: text content does not match server-rendered HTML",
			errType: "HydrationError", level: "warn", source: model.SourceFrontend, project: web,
			culprit: "hydrateRoot (client.tsx:19)", status: model.IssueIgnored, ignored: true, base: 2,
			stack: "HydrationError: text content does not match server-rendered HTML\n    at hydrateRoot (http://localhost:5173/src/client.tsx:19:3)",
		},
	}

	rng := rand.New(rand.NewSource(42))
	issues := make([]model.Issue, 0, len(templates))
	issueIDs := make([]primitive.ObjectID, len(templates))
	events := make([]model.Event, 0, 1200)
	issueCounts := make([]int64, len(templates))
	lastSeen := make([]time.Time, len(templates))
	firstSeen := make([]time.Time, len(templates))

	for i := range templates {
		firstSeen[i] = now.Add(-time.Duration(6+i%3) * 24 * time.Hour)
		lastSeen[i] = now
	}
	// Event fan-out over the last 7 days, with a visible spike on the last day.
	for d := 6; d >= 0; d-- {
		day := now.AddDate(0, 0, -d)
		for i, tpl := range templates {
			if tpl.ignored {
				// Ignored issues stop occurring three days ago.
				if d < 3 {
					continue
				}
			}
			perDay := tpl.base
			if d <= 1 {
				perDay = int(float64(perDay) * 1.8)
			}
			if d == 3 && tpl.resolved {
				continue
			}
			for j := 0; j < perDay; j++ {
				ts := day.Add(time.Duration(rng.Intn(22)+1) * time.Hour).Add(time.Duration(rng.Intn(60)) * time.Minute)
				if ts.After(now) {
					ts = now.Add(-time.Duration(rng.Intn(120)) * time.Minute)
				}
				uid := fmt.Sprintf("u_%d", rng.Intn(40)+1)
				ev := model.Event{
					TenantID: t.ID, ProjectID: tpl.project.ID,
					Kind: model.KindError, Source: tpl.source, Level: tpl.level,
					Message: tpl.message, Timestamp: ts, ReceivedAt: ts.Add(time.Second),
					Environment: "production", Release: "web@1.8." + fmt.Sprint(3+d%2),
					Service: tpl.project.Slug, Platform: tpl.project.Platform,
					ErrorType: tpl.errType, Culprit: tpl.culprit,
					Stack: tpl.stack, URL: "https://app.acme.dev" + []string{"/orders", "/checkout", "/dashboard"}[i%3],
					Tags: map[string]string{"region": []string{"ap-east-1", "us-west-2"}[i%2], "tenant": "acme"},
					User: &model.EventUser{ID: uid, Email: uid + "@customer.example", Username: "customer-" + uid[2:]},
					Breadcrumbs: []model.Breadcrumb{
						{Timestamp: ts.Add(-4 * time.Second), Category: "navigation", Message: "navigated to /orders", Level: "info"},
						{Timestamp: ts.Add(-2 * time.Second), Category: "http", Message: "GET /api/orders 200", Level: "info",
							Data: map[string]any{"status": 200, "durationMs": 184}},
						{Timestamp: ts.Add(-time.Second), Category: "ui", Message: "button#checkout clicked", Level: "info"},
					},
				}
				if tpl.source == model.SourceBackend {
					ev.Request = &model.EventRequest{
						Method: "POST", URL: "/v1/" + tpl.project.Slug,
						Status: 502, DurationMS: int64(400 + rng.Intn(2000)),
						RequestID: fmt.Sprintf("req_%d_%d", d, j),
					}
				}
				ev.Fingerprint = ingest.Fingerprint(ev.Source, tpl.errType, tpl.message, tpl.culprit)
				events = append(events, ev)
				issueCounts[i]++
				if ts.After(lastSeen[i]) {
					lastSeen[i] = ts
				}
				if ts.Before(firstSeen[i]) {
					firstSeen[i] = ts
				}
			}
		}
	}
	// Logs: 60-120 per day across both sources.
	logMessages := []struct {
		level, source, msg string
		project            model.Project
	}{
		{"info", model.SourceBackend, "GET /api/orders 200 in 42ms", api},
		{"info", model.SourceBackend, "GET /api/products 200 in 18ms", api},
		{"warn", model.SourceBackend, "slow query: orders.find took 830ms", api},
		{"error", model.SourceBackend, "upstream checkout-service returned 502", api},
		{"debug", model.SourceFrontend, "router: navigated to /dashboard", web},
		{"info", model.SourceFrontend, "fetch /api/orders completed", web},
		{"warn", model.SourceFrontend, "cart state restored from localStorage (2 items)", web},
		{"error", model.SourceFrontend, "unhandled promise rejection: network timeout", web},
	}
	for d := 6; d >= 0; d-- {
		day := now.AddDate(0, 0, -d)
		n := 55 + rng.Intn(45)
		if d <= 1 {
			n = int(float64(n) * 1.5)
		}
		for j := 0; j < n; j++ {
			tpl := logMessages[rng.Intn(len(logMessages))]
			ts := day.Add(time.Duration(rng.Intn(23)) * time.Hour).Add(time.Duration(rng.Intn(60)) * time.Minute)
			if ts.After(now) {
				ts = now.Add(-time.Duration(rng.Intn(300)) * time.Minute)
			}
			ev := model.Event{
				TenantID: t.ID, ProjectID: tpl.project.ID,
				Kind: model.KindLog, Source: tpl.source, Level: tpl.level,
				Message: tpl.msg, Timestamp: ts, ReceivedAt: ts.Add(time.Millisecond),
				Environment: "production", Release: "web@1.8.4",
				Service: tpl.project.Slug, Platform: tpl.project.Platform,
				Tags:    map[string]string{"region": []string{"ap-east-1", "us-west-2"}[rng.Intn(2)]},
				Context: map[string]any{"durationMs": rng.Intn(900), "method": "GET", "path": []string{"/api/orders", "/api/products", "/api/cart"}[rng.Intn(3)]},
				User:    &model.EventUser{ID: fmt.Sprintf("u_%d", rng.Intn(40)+1)},
			}
			if tpl.source == model.SourceBackend {
				ev.Request = &model.EventRequest{RequestID: fmt.Sprintf("req_log_%d_%d", d, j), Method: "GET", URL: "/api/orders", Status: 200, DurationMS: int64(rng.Intn(900))}
			}
			events = append(events, ev)
		}
	}

	// Build issue documents now that counts are known.
	for i, tpl := range templates {
		fp := ingest.Fingerprint(tpl.source, tpl.errType, tpl.message, tpl.culprit)
		issue := model.Issue{
			TenantID: t.ID, ProjectID: tpl.project.ID, Fingerprint: fp,
			Title: tpl.title, Culprit: tpl.culprit, Level: tpl.level, Status: tpl.status,
			Source: tpl.source, Count: issueCounts[i], FirstSeen: firstSeen[i], LastSeen: lastSeen[i],
			RegressionCount: tpl.regress, CreatedAt: firstSeen[i], UpdatedAt: lastSeen[i],
			AssigneeName: "",
		}
		if issue.Count == 0 {
			issue.Count = 1
		}
		if tpl.ignored {
			issue.Status = model.IssueIgnored
		}
		if tpl.resolved {
			rt := now.Add(-48 * time.Hour)
			issue.ResolvedAt = &rt
			issue.ResolvedBy = "Demo Owner"
		}
		issueIDs[i] = primitive.NewObjectID()
		issue.ID = issueIDs[i]
		issues = append(issues, issue)
	}
	// Link events to issues by fingerprint.
	fpToIssue := map[string]primitive.ObjectID{}
	for i, tpl := range templates {
		fpToIssue[ingest.Fingerprint(tpl.source, tpl.errType, tpl.message, tpl.culprit)] = issueIDs[i]
	}
	for i := range events {
		if events[i].Kind == model.KindError {
			events[i].IssueID = fpToIssue[events[i].Fingerprint]
		}
	}
	if _, err := st.Issues.InsertMany(ctx, toAny(issues)); err != nil {
		return fmt.Errorf("seed issues: %w", err)
	}
	// Chunk to stay well under BSON limits.
	for start := 0; start < len(events); start += 200 {
		end := start + 200
		if end > len(events) {
			end = len(events)
		}
		if err := st.InsertEvents(ctx, events[start:end]); err != nil {
			return fmt.Errorf("seed events: %w", err)
		}
	}
	logger.Info("demo data seeded",
		"tenant", t.Slug,
		"webIngestKey", web.IngestKey,
		"apiIngestKey", api.IngestKey,
		"issues", len(issues),
		"events", len(events),
		"oidc", auth.Enabled(t),
	)
	return nil
}

func toAny[T any](in []T) []any {
	out := make([]any, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}
