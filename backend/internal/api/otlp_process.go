package api

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/otlp"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

var errOTLPRateLimited = errors.New("OTLP rate limited")

func (a *API) processOTLPLogs(ctx context.Context, tenant model.Tenant, project model.Project, req *collectorlogsv1.ExportLogsServiceRequest) (int64, error) {
	events := otlp.ConvertLogs(req, project, time.Now().UTC())
	rejected := int64(0)
	if len(events) > maxOTLPEvents {
		rejected = int64(len(events) - maxOTLPEvents)
		events = events[:maxOTLPEvents]
	}
	if len(events) == 0 {
		return rejected, nil
	}
	if !a.limiter.allow(project.ID.Hex(), len(events)) {
		return rejected, errOTLPRateLimited
	}
	if _, _, err := a.persistEvents(ctx, tenant, project, events); err != nil {
		return rejected, err
	}
	return rejected, nil
}

func (a *API) processOTLPTraces(ctx context.Context, tenant model.Tenant, project model.Project, req *collectortracev1.ExportTraceServiceRequest) (int64, error) {
	now := time.Now().UTC()
	spans := otlp.ConvertSpans(req, project, now)
	spans = sampleSpans(spans, project)
	events := otlp.ConvertTraces(req, project, now)

	rejected := int64(0)
	if len(spans) > maxOTLPEvents {
		rejected = int64(len(spans) - maxOTLPEvents)
		spans = spans[:maxOTLPEvents]
	}
	accepted := make(map[string]bool, len(spans))
	for i := range spans {
		accepted[spans[i].SpanID] = true
	}
	events = filterEventsForSpans(events, accepted)
	if len(spans) == 0 && len(events) == 0 {
		return rejected, nil
	}
	count := len(spans)
	if count == 0 {
		count = len(events)
	}
	if !a.limiter.allow(project.ID.Hex(), count) {
		return rejected, errOTLPRateLimited
	}
	if err := a.store.InsertSpans(ctx, spans); err != nil {
		return rejected, err
	}
	if len(events) > 0 {
		if _, _, err := a.persistEvents(ctx, tenant, project, events); err != nil {
			return rejected, err
		}
	}
	return rejected, nil
}

func (a *API) processOTLPMetrics(ctx context.Context, project model.Project, req *collectormetricsv1.ExportMetricsServiceRequest) (int64, error) {
	points := otlp.ConvertMetrics(req, project, time.Now().UTC())
	rejected := int64(0)
	if len(points) > maxOTLPEvents {
		rejected = int64(len(points) - maxOTLPEvents)
		points = points[:maxOTLPEvents]
	}
	if len(points) == 0 {
		return rejected, nil
	}
	if !a.limiter.allow(project.ID.Hex(), len(points)) {
		return rejected, errOTLPRateLimited
	}
	if err := a.store.InsertMetrics(ctx, points); err != nil {
		return rejected, err
	}
	return rejected, nil
}

// sampleSpans keeps all spans for error traces (when configured) and applies
// deterministic trace-id sampling to the rest. Sampling is stable across
// batches, so a trace is never partially stored because of sampling.
func sampleSpans(spans []model.Span, project model.Project) []model.Span {
	rate := project.TraceSampleRate
	if rate >= 1 {
		return spans
	}
	errorTraces := make(map[string]bool)
	for i := range spans {
		if spans[i].Status == "error" || hasException(spans[i]) {
			errorTraces[spans[i].TraceID] = true
		}
	}
	out := make([]model.Span, 0, len(spans))
	for _, span := range spans {
		if project.TraceErrorAlwaysSample && errorTraces[span.TraceID] {
			out = append(out, span)
			continue
		}
		if rate > 0 && traceHash(span.TraceID) < rate {
			out = append(out, span)
		}
	}
	return out
}

func hasException(span model.Span) bool {
	for _, event := range span.Events {
		if event.Name == "exception" {
			return true
		}
	}
	return false
}

func traceHash(traceID string) float64 {
	sum := sha256.Sum256([]byte(traceID))
	return float64(binary.BigEndian.Uint64(sum[:8])) / float64(^uint64(0))
}

func filterEventsForSpans(events []model.Event, accepted map[string]bool) []model.Event {
	out := make([]model.Event, 0, len(events))
	for _, e := range events {
		spanID := ""
		if e.Tags != nil {
			spanID = e.Tags["span_id"]
		}
		if spanID == "" || accepted[spanID] {
			out = append(out, e)
		}
	}
	return out
}
