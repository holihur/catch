package api

import (
	"context"
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
