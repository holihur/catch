package ingest

import "testing"

func TestNormalizeMessage(t *testing.T) {
	cases := map[string]string{
		"User 123 not found": "user <n> not found",
		"Order 550e8400-e29b-41d4-a716-446655440000 failed": "order <uuid> failed",
		"Could not reach 10.0.0.12":                         "could not reach <ip>",
		"Request 'abc-123' failed":                          "request <str> failed",
		"email bob@example.com invalid":                     "email <email> invalid",
	}
	for in, want := range cases {
		if got := NormalizeMessage(in); got != want {
			t.Errorf("NormalizeMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorType(t *testing.T) {
	cases := map[string]string{
		"TypeError: x is undefined":                "TypeError",
		"panic: runtime error: index out of range": "panic",
		"plain message":                            "plain",
		"":                                         "",
	}
	for in, want := range cases {
		if got := ErrorType(in); got != want {
			t.Errorf("ErrorType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCulprit(t *testing.T) {
	stack := `TypeError: x is undefined
    at loadUser (http://localhost:5173/src/pages/User.tsx:42:11)
    at node_modules/react-dom/index.js:10:2`
	want := "loadUser (User.tsx:42)"
	if got := Culprit(stack); got != want {
		t.Fatalf("Culprit() = %q, want %q", got, want)
	}
}

func TestFingerprintStable(t *testing.T) {
	a := Fingerprint("frontend", "TypeError", "user 123 missing", "loadUser (User.tsx:42)")
	b := Fingerprint("frontend", "TypeError", "user 999 missing", "loadUser (User.tsx:42)")
	if a != b {
		t.Fatalf("fingerprints differ: %s vs %s", a, b)
	}
	c := Fingerprint("frontend", "TypeError", "order 123 missing", "loadUser (User.tsx:42)")
	if a == c {
		t.Fatal("different messages should have different fingerprints")
	}
}

func TestTitle(t *testing.T) {
	if got := Title("TypeError", "TypeError: x is undefined"); got != "TypeError: x is undefined" {
		t.Fatalf("unexpected title %q", got)
	}
	if got := Title("TypeError", "x is undefined"); got != "TypeError: x is undefined" {
		t.Fatalf("unexpected title %q", got)
	}
}
