package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckOriginAllowsIngestFromAnyOrigin(t *testing.T) {
	called := false
	h := CheckOrigin([]string{"https://app.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", strings.NewReader(`{"events":[]}`))
	req.Header.Set("Origin", "https://partner.example.net")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusAccepted {
		t.Fatalf("ingest should bypass origin check, called=%v status=%d", called, rec.Code)
	}
}

func TestCheckOriginBlocksUnknownOriginForAdminAPI(t *testing.T) {
	h := CheckOrigin([]string{"https://app.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example.net")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCORSSDKIsEmbeddableEverywhere(t *testing.T) {
	h := CORS([]string{"https://app.example.com"}, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/catch-sdk.js", nil)
	req.Header.Set("Origin", "https://partner.example.net")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard CORS for SDK, got %q", got)
	}
}
