package api

import (
	"testing"

	"github.com/catch/catch/backend/internal/model"
)

func TestSampleSpansKeepsErrorTraces(t *testing.T) {
	spans := []model.Span{
		{TraceID: "keep-error", SpanID: "a", Status: "ok"},
		{TraceID: "keep-error", SpanID: "b", Status: "error"},
		{TraceID: "drop-me", SpanID: "c", Status: "ok"},
	}
	project := model.Project{TraceSampleRate: 0, TraceErrorAlwaysSample: true}
	got := sampleSpans(spans, project)
	if len(got) != 2 {
		t.Fatalf("expected only error trace spans, got %+v", got)
	}
	for _, span := range got {
		if span.TraceID != "keep-error" {
			t.Fatalf("unexpected span kept: %+v", span)
		}
	}

	project.TraceErrorAlwaysSample = false
	if got := sampleSpans(spans, project); len(got) != 0 {
		t.Fatalf("expected no spans at 0%% without error override, got %+v", got)
	}

	project.TraceSampleRate = 1
	if got := sampleSpans(spans, project); len(got) != len(spans) {
		t.Fatalf("expected all spans at 100%%, got %d", len(got))
	}
}

func TestSampleSpansIsDeterministic(t *testing.T) {
	project := model.Project{TraceSampleRate: 0.5, TraceErrorAlwaysSample: false}
	spans := []model.Span{
		{TraceID: "a", SpanID: "1"}, {TraceID: "b", SpanID: "2"},
		{TraceID: "c", SpanID: "3"}, {TraceID: "d", SpanID: "4"},
	}
	first := sampleSpans(spans, project)
	for i := 0; i < 20; i++ {
		if got := sampleSpans(spans, project); len(got) != len(first) {
			t.Fatalf("sampling is not deterministic: %d vs %d", len(got), len(first))
		}
	}
}
