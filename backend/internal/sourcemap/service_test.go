package sourcemap

import (
	"testing"

	"github.com/catch/catch/backend/internal/model"
)

func TestNormalizeGeneratedPath(t *testing.T) {
	cases := map[string]string{
		"http://localhost:5173/assets/index-abc.js": "assets/index-abc.js",
		"file:///app/dist/app.min.js":               "app/dist/app.min.js",
		"./dist/app.min.js?v=1":                     "dist/app.min.js",
		"/static/js/main.js#x":                      "static/js/main.js",
		"app.min.js":                                "app.min.js",
	}
	for in, want := range cases {
		if got := normalizeGeneratedPath(in); got != want {
			t.Errorf("normalizeGeneratedPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanSourcePath(t *testing.T) {
	cases := map[string]string{
		"webpack:///./src/App.tsx":          "src/App.tsx",
		"../src/pages/Orders.tsx":           "src/pages/Orders.tsx",
		"file:///app/backend/internal/x.go": "app/backend/internal/x.go",
		"C:\\repo\\src\\main.ts":            "repo/src/main.ts",
	}
	for in, want := range cases {
		if got := cleanSourcePath(in); got != want {
			t.Errorf("cleanSourcePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchSourceMap(t *testing.T) {
	maps := []model.SourceMap{
		{File: "assets/index-abc.js"},
		{File: "app.min.js"},
	}
	sm, ok := matchSourceMap(maps, "assets/index-abc.js")
	if !ok || sm.File != "assets/index-abc.js" {
		t.Fatalf("exact match failed: %+v %v", sm, ok)
	}
	sm, ok = matchSourceMap(maps, "static/app.min.js")
	if !ok || sm.File != "app.min.js" {
		t.Fatalf("basename match failed: %+v %v", sm, ok)
	}
	if _, ok := matchSourceMap(maps, "other.js"); ok {
		t.Fatal("unexpected match")
	}
}

func TestAToi(t *testing.T) {
	if atoi("42") != 42 || atoi("") != 0 || atoi("1x") != 0 {
		t.Fatal("unexpected atoi")
	}
}

func TestExtractSources(t *testing.T) {
	raw := []byte(`{
	  "version": 3,
	  "sourceRoot": "/repo",
	  "sources": ["webpack:///./src/App.tsx", "../src/original.ts", "src/empty.ts"],
	  "sourcesContent": ["const App = 1", "export const x = 2", ""],
	  "mappings": "AAAA"
	}`)
	sources := extractSources(raw)
	if sources["src/App.tsx"] != "const App = 1" {
		t.Fatalf("webpack source not extracted: %#v", sources)
	}
	if sources["src/original.ts"] != "export const x = 2" {
		t.Fatalf("relative source not extracted: %#v", sources)
	}
	if _, ok := sources["src/empty.ts"]; ok {
		t.Fatal("empty sourcesContent should be skipped")
	}
}
