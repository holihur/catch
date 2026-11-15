package api

import "testing"

func TestNormalizeRepoURL(t *testing.T) {
	cases := []struct {
		raw      string
		provider string
		wantURL  string
		wantProv string
		wantErr  bool
	}{
		{"https://github.com/acme/app.git/", "", "https://github.com/acme/app", "github", false},
		{"https://gitlab.com/acme/app", "", "https://gitlab.com/acme/app", "gitlab", false},
		{"https://bitbucket.org/acme/app", "", "https://bitbucket.org/acme/app", "bitbucket", false},
		{"https://git.example.com/acme/app", "", "https://git.example.com/acme/app", "custom", false},
		{"https://git.example.com/acme/app", "gitea", "https://git.example.com/acme/app", "gitea", false},
		{"ftp://example.com/repo", "", "", "", true},
		{"not-a-url", "", "", "", true},
	}
	for _, c := range cases {
		gotURL, gotProv, err := normalizeRepoURL(c.raw, c.provider)
		if (err != nil) != c.wantErr {
			t.Errorf("normalizeRepoURL(%q, %q) error = %v, wantErr %v", c.raw, c.provider, err, c.wantErr)
			continue
		}
		if err == nil && (gotURL != c.wantURL || gotProv != c.wantProv) {
			t.Errorf("normalizeRepoURL(%q, %q) = (%q, %q), want (%q, %q)", c.raw, c.provider, gotURL, gotProv, c.wantURL, c.wantProv)
		}
	}
}
