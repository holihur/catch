// Package httpx contains HTTP plumbing shared by the API.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/logging"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/catch/catch/backend/internal/secure"
)

// ErrorBody is the single error envelope returned by the API.
type ErrorBody struct {
	Error ErrorInfo `json:"error"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// JSON writes a JSON response.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes the standard error envelope.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorBody{Error: ErrorInfo{Code: code, Message: message}})
}

// ErrorDetails writes an error envelope with structured details.
func ErrorDetails(w http.ResponseWriter, status int, code, message string, details any) {
	JSON(w, status, ErrorBody{Error: ErrorInfo{Code: code, Message: message, Details: details}})
}

// DecodeJSON decodes a request body with a hard size limit and strict fields.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return fmt.Errorf("request body exceeds %d bytes", maxBytes)
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		default:
			return fmt.Errorf("invalid JSON body: %w", err)
		}
	}
	// Reject trailing JSON documents.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// RequestID assigns a request id used in logs, responses and error events.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if id == "" || len(id) > 128 {
			id = secure.RandomToken(12)
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(reqctx.WithRequestID(r.Context(), id)))
	})
}

// Recover converts panics into 500 responses and backend error logs.
func Recover(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := string(debug.Stack())
					logger.Slog.ErrorContext(r.Context(), "panic recovered",
						"panic", fmt.Sprint(rec),
						"method", r.Method,
						"path", r.URL.Path,
						"stack", logging.Truncate(stack, 16000),
					)
					// If the handler already wrote headers we cannot change the
					// status, but still close the response.
					Error(w, http.StatusInternalServerError, "internal_error", "服务端发生未处理的异常")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog records every request at info level and 5xx at error level.
func AccessLog(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			dur := time.Since(start)
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"durationMs", dur.Milliseconds(),
				"requestId", reqctx.RequestID(r.Context()),
				"ip", ClientIP(r),
				"bytes", ww.bytes,
			}
			if ww.status >= 500 {
				logger.Slog.ErrorContext(r.Context(), "request failed", attrs...)
			} else if ww.status >= 400 {
				logger.Slog.WarnContext(r.Context(), "request rejected", attrs...)
			} else {
				logger.Slog.InfoContext(r.Context(), "request", attrs...)
			}
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *responseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ClientIP returns the best-effort client IP, trusting the first XFF hop only
// when the direct peer is a private/loopback address.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" && isPrivate(host) {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	return host
}

func isPrivate(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate()
}

// CORS handles credentialed API requests. Ingest is allowed from any origin
// because browser SDKs use a project ingest key rather than cookies.
func CORS(origins []string, allowAnyForIngest bool) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if origin != "" {
				isIngest := strings.HasPrefix(r.URL.Path, "/api/ingest")
				isOTLP := r.URL.Path == "/v1/logs" || r.URL.Path == "/v1/traces" || r.URL.Path == "/v1/metrics"
				isSDK := r.URL.Path == "/catch-sdk.js"
				switch {
				case isSDK:
					// The browser SDK is a public static file and must be
					// embeddable by any third-party origin.
					w.Header().Set("Access-Control-Allow-Origin", "*")
					w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
					w.Header().Set("Access-Control-Max-Age", "600")
				case allowed[origin] || (allowAnyForIngest && (isIngest || isOTLP)):
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Ingest-Key, X-Request-Id, X-Project-Id")
					w.Header().Set("Access-Control-Max-Age", "600")
					w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CheckOrigin rejects cross-site cookie-authenticated mutations. Requests with
// no Origin (curl, server-to-server) are allowed.
func CheckOrigin(allowed []string) func(http.Handler) http.Handler {
	allowedSet := map[string]bool{}
	for _, o := range allowed {
		allowedSet[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			// Ingest is authenticated by a project key, not a cookie, and is
			// intentionally callable from any third-party origin.
			if strings.HasPrefix(r.URL.Path, "/api/ingest") ||
				r.URL.Path == "/v1/logs" || r.URL.Path == "/v1/traces" || r.URL.Path == "/v1/metrics" {
				next.ServeHTTP(w, r)
				return
			}
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if origin != "" && !allowedSet[origin] {
				Error(w, http.StatusForbidden, "origin_not_allowed", "请求来源不被允许")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders applies conservative browser hardening.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// NoCache prevents stale API responses.
func NoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
