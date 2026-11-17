package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/secure"
)

func TestNormalizeRecipients(t *testing.T) {
	got, msg := normalizeRecipients([]string{"A <a@example.com>, b@example.com", "a@example.com"})
	if msg != "" || len(got) != 2 || got[0] != "a@example.com" || got[1] != "b@example.com" {
		t.Fatalf("unexpected recipients: %v %s", got, msg)
	}
	if _, msg := normalizeRecipients([]string{"not-an-email"}); msg == "" {
		t.Fatal("expected invalid email to be rejected")
	}
}

func TestSanitizeMailHeader(t *testing.T) {
	if got := sanitizeMailHeader("hello\r\nBcc: evil@example.com"); got != "hello  Bcc: evil@example.com" {
		t.Fatalf("unexpected header: %q", got)
	}
}

func TestSendWebhookSignature(t *testing.T) {
	box, err := secure.NewBox("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := box.Encrypt("webhook-secret")
	if err != nil {
		t.Fatal(err)
	}
	var gotSignature string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get("X-Catch-Signature")
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"event":"firing"}` {
			t.Errorf("unexpected body: %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	a := &API{box: box}
	channel := model.NotificationChannel{Type: "webhook", WebhookURL: ts.URL, WebhookSecretEnc: enc}
	body := []byte(`{"event":"firing"}`)
	status, err := a.sendWebhook(context.Background(), channel, body, "delivery-1")
	if err != nil || status != http.StatusOK {
		t.Fatalf("send webhook: status=%d err=%v", status, err)
	}
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSignature != want {
		t.Fatalf("signature mismatch: got %q want %q", gotSignature, want)
	}
}

func TestWebhookPrivatePolicy(t *testing.T) {
	blocked := newWebhookPolicy(false, nil)
	if err := blocked.CheckURL("http://127.0.0.1:9999/hook"); err == nil {
		t.Fatal("expected loopback webhook to be rejected")
	}
	if err := blocked.CheckURL("http://localhost:9999/hook"); err == nil {
		t.Fatal("expected localhost webhook to be rejected")
	}
	allowlisted := newWebhookPolicy(false, []string{"127.0.0.1"})
	if err := allowlisted.CheckURL("http://127.0.0.1:9999/hook"); err != nil {
		t.Fatalf("allowlisted IP rejected: %v", err)
	}
	allowAll := newWebhookPolicy(true, nil)
	if err := allowAll.CheckURL("http://localhost:9999/hook"); err != nil {
		t.Fatalf("allow-private rejected: %v", err)
	}
}

func TestWebhookPrivatePolicyDial(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	blockedClient := newNotificationHTTPClient(newWebhookPolicy(false, nil))
	resp, err := blockedClient.Get(ts.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected blocked dial to fail")
	}
	if !strings.Contains(err.Error(), "blocked private") {
		t.Fatalf("unexpected blocked error: %v", err)
	}

	allowedClient := newNotificationHTTPClient(newWebhookPolicy(false, []string{"127.0.0.1"}))
	resp, err = allowedClient.Get(ts.URL)
	if err != nil {
		t.Fatalf("allowlisted dial failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}
