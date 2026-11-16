package otlp

import (
	"testing"
	"time"

	"github.com/catch/catch/backend/internal/model"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func kv(k string, v *commonv1.AnyValue) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: k, Value: v}
}

func str(k, v string) *commonv1.KeyValue {
	return kv(k, &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v}})
}
func num(k string, v int64) *commonv1.KeyValue {
	return kv(k, &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: v}})
}

func TestConvertLogs(t *testing.T) {
	project := model.Project{Name: "orders", Platform: "go"}
	req := &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
				str("service.name", "orders-api"),
				str("service.version", "orders-api@2.3.1"),
				str("deployment.environment", "production"),
				str("telemetry.sdk.language", "go"),
			}},
			ScopeLogs: []*logsv1.ScopeLogs{{
				LogRecords: []*logsv1.LogRecord{{
					TimeUnixNano:   uint64(time.Now().Add(-time.Minute).UnixNano()),
					SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR,
					SeverityText:   "ERROR",
					Body:           &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "checkout failed"}},
					TraceId:        []byte{1, 2, 3, 4},
					Attributes: []*commonv1.KeyValue{
						str("exception.type", "MongoServerError"),
						str("exception.message", "duplicate key"),
						str("exception.stacktrace", "MongoServerError: duplicate key\n    at orders.Repository.Insert (internal/orders/repository.go:78)"),
						str("http.request.method", "POST"),
						num("http.response.status_code", 500),
						str("enduser.id", "u_123"),
					},
				}},
			}},
		}},
	}
	events := ConvertLogs(req, project, time.Now())
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Kind != model.KindError || e.Level != "error" {
		t.Fatalf("unexpected kind/level: %s/%s", e.Kind, e.Level)
	}
	if e.Service != "orders-api" || e.Environment != "production" || e.Release != "orders-api@2.3.1" {
		t.Fatalf("unexpected resource mapping: %+v", e)
	}
	if e.ErrorType != "MongoServerError" || e.Culprit == "" || e.Fingerprint == "" {
		t.Fatalf("unexpected error mapping: %+v", e)
	}
	if e.Request == nil || e.Request.Method != "POST" || e.Request.Status != 500 {
		t.Fatalf("unexpected request: %+v", e.Request)
	}
	if e.User == nil || e.User.ID != "u_123" {
		t.Fatalf("unexpected user: %+v", e.User)
	}
	if e.Tags["trace_id"] != "01020304" {
		t.Fatalf("unexpected tags: %+v", e.Tags)
	}
	if len(e.Context) == 0 {
		t.Fatal("missing context")
	}

	// JSON decoding must accept the OTLP/HTTP JSON encoding.
	raw, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeLogs(raw, "application/json")
	if err != nil || len(decoded.GetResourceLogs()) != 1 {
		t.Fatalf("JSON decode failed: %v %+v", err, decoded)
	}
	rawProto, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeLogs(rawProto, "application/x-protobuf")
	if err != nil || len(decoded.GetResourceLogs()) != 1 {
		t.Fatalf("protobuf decode failed: %v %+v", err, decoded)
	}
}

func TestConvertTracesOnlyErrors(t *testing.T) {
	project := model.Project{Name: "orders"}
	exc := &tracev1.Span_Event{
		TimeUnixNano: uint64(time.Now().UnixNano()),
		Name:         "exception",
		Attributes: []*commonv1.KeyValue{
			str("exception.type", "TimeoutError"),
			str("exception.message", "upstream timeout"),
			str("exception.stacktrace", "TimeoutError\n    at client.Do (internal/checkout/client.go:112)"),
		},
	}
	req := &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{str("service.name", "checkout")}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Spans: []*tracev1.Span{
					{
						TraceId: []byte{9, 9}, SpanId: []byte{1, 1}, Name: "POST /checkout",
						Kind: tracev1.Span_SPAN_KIND_SERVER, StartTimeUnixNano: uint64(time.Now().UnixNano()),
						Events: []*tracev1.Span_Event{exc},
						Status: &tracev1.Status{Code: tracev1.Status_STATUS_CODE_ERROR, Message: "boom"},
					},
					{
						TraceId: []byte{8, 8}, SpanId: []byte{2, 2}, Name: "GET /health",
						Status: &tracev1.Status{Code: tracev1.Status_STATUS_CODE_OK},
					},
				},
			}},
		}},
	}
	events := ConvertTraces(req, project, time.Now())
	if len(events) != 1 {
		t.Fatalf("expected 1 error span event, got %d", len(events))
	}
	e := events[0]
	if e.Kind != model.KindError || e.ErrorType != "TimeoutError" || e.Service != "checkout" {
		t.Fatalf("unexpected span mapping: %+v", e)
	}
	if e.Tags["trace_id"] != "0909" || e.Tags["span_name"] != "POST /checkout" {
		t.Fatalf("unexpected tags: %+v", e.Tags)
	}
}

func TestSeverityLevel(t *testing.T) {
	cases := []struct {
		num  logsv1.SeverityNumber
		text string
		want string
	}{
		{logsv1.SeverityNumber_SEVERITY_NUMBER_TRACE, "", "debug"},
		{logsv1.SeverityNumber_SEVERITY_NUMBER_DEBUG, "", "debug"},
		{logsv1.SeverityNumber_SEVERITY_NUMBER_INFO, "", "info"},
		{logsv1.SeverityNumber_SEVERITY_NUMBER_WARN, "", "warn"},
		{logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR, "", "error"},
		{logsv1.SeverityNumber_SEVERITY_NUMBER_FATAL, "", "fatal"},
		{logsv1.SeverityNumber_SEVERITY_NUMBER_INFO, "WARNING", "info"},
		{logsv1.SeverityNumber_SEVERITY_NUMBER_UNSPECIFIED, "WARNING", "warn"},
	}
	for _, c := range cases {
		if got := severityLevel(c.num, c.text); got != c.want {
			t.Errorf("severityLevel(%v, %q) = %q, want %q", c.num, c.text, got, c.want)
		}
	}
}

func TestConvertTraceSpanAttributeException(t *testing.T) {
	project := model.Project{Name: "orders"}
	req := &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{str("service.name", "span-service")}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Spans: []*tracev1.Span{{
					TraceId: []byte{1}, SpanId: []byte{2}, Name: "checkout",
					StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
					EndTimeUnixNano:   uint64(time.Now().UnixNano()),
					Attributes: []*commonv1.KeyValue{
						str("exception.type", "AttributeError"),
						str("exception.message", "attached to span"),
						str("exception.stacktrace", "AttributeError\n    at checkout (src/checkout.ts:42:7)"),
					},
					Status: &tracev1.Status{Code: tracev1.Status_STATUS_CODE_OK},
				}},
			}},
		}},
	}
	events := ConvertTraces(req, project, time.Now())
	if len(events) != 1 {
		t.Fatalf("expected span attribute exception to be ingested, got %d", len(events))
	}
	if events[0].ErrorType != "AttributeError" || events[0].Request == nil || events[0].Request.DurationMS != 1000 {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestConvertSpans(t *testing.T) {
	project := model.Project{Name: "orders"}
	req := &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{str("service.name", "orders-api")}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Scope: &commonv1.InstrumentationScope{Name: "http", Version: "1.0"},
				Spans: []*tracev1.Span{{
					TraceId: []byte{1, 2}, SpanId: []byte{3, 4}, ParentSpanId: []byte{5, 6},
					Name: "root", Kind: tracev1.Span_SPAN_KIND_SERVER,
					StartTimeUnixNano: uint64(time.Now().Add(-time.Second).UnixNano()),
					EndTimeUnixNano:   uint64(time.Now().UnixNano()),
					Status:            &tracev1.Status{Code: tracev1.Status_STATUS_CODE_ERROR, Message: "boom"},
					Attributes:        []*commonv1.KeyValue{str("http.route", "/orders")},
					Events: []*tracev1.Span_Event{{
						TimeUnixNano: uint64(time.Now().UnixNano()), Name: "exception",
						Attributes: []*commonv1.KeyValue{str("exception.type", "Error")},
					}},
				}},
			}},
		}},
	}
	spans := ConvertSpans(req, project, time.Now())
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.TraceID != "0102" || span.SpanID != "0304" || span.ParentSpanID != "0506" {
		t.Fatalf("unexpected span IDs: %+v", span)
	}
	if span.Service != "orders-api" || span.Status != "error" || span.ScopeName != "http" || span.DurationMS < 900 {
		t.Fatalf("unexpected span mapping: %+v", span)
	}
	if len(span.Events) != 1 || span.Events[0].Name != "exception" {
		t.Fatalf("unexpected span events: %+v", span.Events)
	}
}

func TestConvertMetrics(t *testing.T) {
	project := model.Project{Name: "orders"}
	now := uint64(time.Now().UnixNano())
	req := &collectormetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricv1.ResourceMetrics{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{str("service.name", "orders-api")}},
			ScopeMetrics: []*metricv1.ScopeMetrics{{
				Metrics: []*metricv1.Metric{
					{Name: "cpu.usage", Unit: "%", Data: &metricv1.Metric_Gauge{Gauge: &metricv1.Gauge{DataPoints: []*metricv1.NumberDataPoint{{
						TimeUnixNano: now, Value: &metricv1.NumberDataPoint_AsDouble{AsDouble: 42.5}, Attributes: []*commonv1.KeyValue{str("host", "a")},
					}}}}},
					{Name: "requests", Unit: "1", Data: &metricv1.Metric_Sum{Sum: &metricv1.Sum{AggregationTemporality: metricv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, IsMonotonic: true, DataPoints: []*metricv1.NumberDataPoint{{
						TimeUnixNano: now, Value: &metricv1.NumberDataPoint_AsInt{AsInt: 10},
					}}}}},
					{Name: "latency", Unit: "ms", Data: &metricv1.Metric_Histogram{Histogram: &metricv1.Histogram{AggregationTemporality: metricv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, DataPoints: []*metricv1.HistogramDataPoint{{
						TimeUnixNano: now, Count: 2, Sum: proto.Float64(100),
						ExplicitBounds: []float64{50}, BucketCounts: []uint64{1, 1},
					}}}}},
					{Name: "rpc.duration", Unit: "ms", Data: &metricv1.Metric_Summary{Summary: &metricv1.Summary{DataPoints: []*metricv1.SummaryDataPoint{{
						TimeUnixNano: now, Count: 2, Sum: 30,
						QuantileValues: []*metricv1.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 10}, {Quantile: 0.95, Value: 20}},
					}}}}},
				},
			}},
		}},
	}
	points := ConvertMetrics(req, project, time.Now())
	if len(points) != 4 {
		t.Fatalf("expected 4 metric points, got %d", len(points))
	}
	if points[0].Name != "cpu.usage" || points[0].Value != 42.5 || points[0].Type != "gauge" || points[0].Attributes["host"] != "a" {
		t.Fatalf("unexpected gauge: %+v", points[0])
	}
	if points[1].Value != 10 || points[1].Type != "sum" || !points[1].IsMonotonic || points[1].Temporality != "cumulative" {
		t.Fatalf("unexpected sum: %+v", points[1])
	}
	if points[2].Value != 50 || points[2].Count != 2 || points[2].Type != "histogram" {
		t.Fatalf("unexpected histogram: %+v", points[2])
	}
	if points[2].Distribution == nil || len(points[2].Distribution.BucketCounts) != 2 || points[2].Distribution.ExplicitBounds[0] != 50 {
		t.Fatalf("unexpected histogram distribution: %+v", points[2].Distribution)
	}
	if points[3].Type != "summary" || len(points[3].Quantiles) != 2 || points[3].Quantiles[1].Value != 20 {
		t.Fatalf("unexpected summary quantiles: %+v", points[3])
	}
}
