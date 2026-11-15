package auth

import "testing"

func TestEmailAllowed(t *testing.T) {
	cases := []struct {
		email   string
		domains []string
		want    bool
	}{
		{"alice@acme.test", nil, true},
		{"alice@acme.test", []string{"acme.test"}, true},
		{"alice@sub.acme.test", []string{"@acme.test"}, true},
		{"alice@evil-acme.test", []string{"acme.test"}, false},
		{"alice@other.test", []string{"acme.test", "example.com"}, false},
		{"bad-email", []string{"acme.test"}, false},
		{"ALICE@ACME.TEST", []string{"ACME.TEST"}, true},
	}
	for _, c := range cases {
		if got := emailAllowed(c.email, c.domains); got != c.want {
			t.Errorf("emailAllowed(%q, %v) = %v, want %v", c.email, c.domains, got, c.want)
		}
	}
}

func TestSafeReturnTo(t *testing.T) {
	cases := map[string]bool{
		"/":                  true,
		"/issues?status=all": true,
		"/issues/123":        true,
		"":                   false,
		"//evil.com":         false,
		"https://evil.com":   false,
		"\\\\evil":           false,
	}
	for in, want := range cases {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q) = %v, want %v", in, got, want)
		}
	}
}
