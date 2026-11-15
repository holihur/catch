package api

import (
	"encoding/json"
	"testing"
)

func TestSplitPayload(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"single", `{"kind":"log","message":"hi"}`, 1},
		{"array", `[{"message":"a"},{"message":"b"}]`, 2},
		{"batch", `{"events":[{"message":"a"},{"message":"b"},{"message":"c"}]}`, 3},
		{"empty batch", `{"events":[]}`, 0},
	}
	for _, c := range cases {
		got, err := splitPayload([]byte(c.in))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if len(got) != c.want {
			t.Errorf("%s: got %d events, want %d", c.name, len(got), c.want)
		}
	}
	if _, err := splitPayload([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

func TestFlexTime(t *testing.T) {
	for _, raw := range []string{`"2026-01-02T03:04:05Z"`, `1767323045`, `1767323045000`} {
		var ft flexTime
		if err := json.Unmarshal([]byte(raw), &ft); err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if ft.Time.IsZero() {
			t.Fatalf("parse %s produced zero time", raw)
		}
	}
}

func TestCleanTags(t *testing.T) {
	tags := cleanTags(map[string]string{" env ": " prod ", "": "x", "region": "ap-east-1"})
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", tags)
	}
	if tags["env"] != "prod" {
		t.Fatalf("unexpected tag value: %v", tags)
	}
}
