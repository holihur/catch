package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the Catch API.
type Config struct {
	Env          string
	Port         string
	PublicURL    string // URL of the API (used for OIDC redirect URIs)
	AppURL       string // URL of the web app (used after login)
	MongoURI     string
	MongoDB      string
	StaticDir    string
	AllowDev     bool
	SeedDemo     bool
	LogLevel     string
	SessionTTL   time.Duration
	CookieName   string
	CookieSecure bool
	AppSecret    string
	Retention    time.Duration
	RateLimit    int // max ingest events per minute per project
	CORSOrigins  []string

	// Optional bootstrap OIDC config. When set, the tenant is upserted on boot.
	OIDCTenantSlug     string
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCAllowedDomains []string
}

// staticDir auto-detects the built frontend so `go run ./cmd/server` and the
// container image both serve the SPA from Go without extra configuration.
func staticDir() string {
	if v := strings.TrimSpace(os.Getenv("STATIC_DIR")); v != "" {
		return v
	}
	for _, candidate := range []string{"web", "../frontend/dist", "frontend/dist", "/app/web"} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Load builds a Config from environment variables.
func Load() (Config, error) {
	cfg := Config{
		Env:                env("APP_ENV", "development"),
		Port:               env("PORT", "8080"),
		PublicURL:          strings.TrimRight(env("PUBLIC_URL", "http://localhost:8080"), "/"),
		AppURL:             strings.TrimRight(env("APP_URL", "http://localhost:5173"), "/"),
		MongoURI:           env("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDB:            env("MONGODB_DB", "catch"),
		StaticDir:          staticDir(),
		AllowDev:           envBool("ALLOW_DEV_LOGIN", true),
		SeedDemo:           envBool("SEED_DEMO", true),
		LogLevel:           strings.ToLower(env("LOG_LEVEL", "info")),
		SessionTTL:         time.Duration(envInt("SESSION_TTL_HOURS", 168)) * time.Hour,
		CookieName:         env("SESSION_COOKIE_NAME", "catch_session"),
		CookieSecure:       envBool("COOKIE_SECURE", false),
		AppSecret:          env("APP_SECRET", "catch-development-secret-change-me"),
		Retention:          time.Duration(envInt("RETENTION_DAYS", 30)) * 24 * time.Hour,
		RateLimit:          envInt("INGEST_RATE_LIMIT", 1200),
		CORSOrigins:        envList("CORS_ORIGINS"),
		OIDCTenantSlug:     env("OIDC_TENANT_SLUG", ""),
		OIDCIssuer:         env("OIDC_ISSUER", ""),
		OIDCClientID:       env("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:   env("OIDC_CLIENT_SECRET", ""),
		OIDCAllowedDomains: envList("OIDC_ALLOWED_DOMAINS"),
	}

	if cfg.CORSOrigins == nil {
		cfg.CORSOrigins = []string{cfg.AppURL, cfg.PublicURL}
	}
	if cfg.SessionTTL <= 0 {
		return cfg, fmt.Errorf("SESSION_TTL_HOURS must be positive")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		cfg.LogLevel = "info"
	}
	if cfg.Env == "production" && cfg.AllowDev {
		// Dev login is a full auth bypass; never allow it in production.
		cfg.AllowDev = false
	}
	if cfg.Env == "production" && cfg.AppSecret == "catch-development-secret-change-me" {
		return cfg, fmt.Errorf("APP_SECRET must be changed in production")
	}
	return cfg, nil
}

// IsProd reports whether the service runs in production mode.
func (c Config) IsProd() bool { return c.Env == "production" }
