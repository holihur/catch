// Package otlp converts OpenTelemetry OTLP logs and error spans into Catch
// events. Metrics and non-error spans are intentionally ignored.
package otlp

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/ingest"
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

// DecodeLogs supports both OTLP/HTTP protobuf and OTLP/HTTP JSON.
func DecodeLogs(body []byte, contentType string) (*collectorlogsv1.ExportLogsServiceRequest, error) {
	req := &collectorlogsv1.ExportLogsServiceRequest{}
	if isJSON(contentType) {
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, req); err != nil {
			return nil, fmt.Errorf("invalid OTLP JSON logs: %w", err)
		}
		return req, nil
	}
	if err := proto.Unmarshal(body, req); err != nil {
		return nil, fmt.Errorf("invalid OTLP protobuf logs: %w", err)
	}
	return req, nil
}

// DecodeTraces supports both OTLP/HTTP protobuf and OTLP/HTTP JSON.
func DecodeTraces(body []byte, contentType string) (*collectortracev1.ExportTraceServiceRequest, error) {
	req := &collectortracev1.ExportTraceServiceRequest{}
	if isJSON(contentType) {
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, req); err != nil {
			return nil, fmt.Errorf("invalid OTLP JSON traces: %w", err)
		}
		return req, nil
	}
	if err := proto.Unmarshal(body, req); err != nil {
		return nil, fmt.Errorf("invalid OTLP protobuf traces: %w", err)
	}
	return req, nil
}

// DecodeMetrics supports both OTLP/HTTP protobuf and OTLP/HTTP JSON.
func DecodeMetrics(body []byte, contentType string) (*collectormetricsv1.ExportMetricsServiceRequest, error) {
	req := &collectormetricsv1.ExportMetricsServiceRequest{}
	if isJSON(contentType) {
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, req); err != nil {
			return nil, fmt.Errorf("invalid OTLP JSON metrics: %w", err)
		}
		return req, nil
	}
	if err := proto.Unmarshal(body, req); err != nil {
		return nil, fmt.Errorf("invalid OTLP protobuf metrics: %w", err)
	}
	return req, nil
}

func isJSON(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "json")
}

type resourceInfo struct {
	service  string
	version  string
	env      string
	platform string
	attrs    map[string]any
}

// ConvertLogs maps OTLP log records to Catch log/error events. Error-level and
// exception records become error events and are grouped into issues.
func ConvertLogs(req *collectorlogsv1.ExportLogsServiceRequest, project model.Project, now time.Time) []model.Event {
	var out []model.Event
	for _, rl := range req.GetResourceLogs() {
		ri := resourceInformation(rl.GetResource(), project)
		for _, sl := range rl.GetScopeLogs() {
			scopeAttrs := map[string]any{
				"name":    sl.GetScope().GetName(),
				"version": sl.GetScope().GetVersion(),
			}
			for _, record := range sl.GetLogRecords() {
				attrs := attributesToMap(record.GetAttributes())
				message := anyValueString(record.GetBody())
				errType := stringAttribute(attrs, "exception.type")
				stack := stringAttribute(attrs, "exception.stacktrace")
				if message == "" {
					message = stringAttribute(attrs, "exception.message")
				}
				if message == "" {
					message = record.GetEventName()
				}
				if message == "" {
					message = record.GetSeverityText()
				}
				if message == "" {
					message = "OTel log"
				}
				level := severityLevel(record.GetSeverityNumber(), record.GetSeverityText())
				kind := model.KindLog
				if errType != "" || stack != "" || level == "error" || level == "fatal" ||
					strings.EqualFold(record.GetEventName(), "exception") {
					kind = model.KindError
				}
				ts := timestamp(record.GetTimeUnixNano(), now)
				if record.GetTimeUnixNano() == 0 {
					ts = timestamp(record.GetObservedTimeUnixNano(), now)
				}
				traceID := hex.EncodeToString(record.GetTraceId())
				spanID := hex.EncodeToString(record.GetSpanId())
				tags := mergedTags(ri.attrs, attrs)
				if traceID != "" {
					tags["trace_id"] = traceID
				}
				if spanID != "" {
					tags["span_id"] = spanID
				}
				if record.GetEventName() != "" {
					tags["event_name"] = record.GetEventName()
				}
				context := map[string]any{
					"otel": map[string]any{
						"resource":   ri.attrs,
						"scope":      scopeAttrs,
						"attributes": attrs,
						"traceId":    traceID,
						"spanId":     spanID,
					},
				}
				event := baseEvent(ri, project, ts, now)
				event.Kind = kind
				event.Level = level
				event.Message = ingest.Truncate(message, 8000)
				event.ErrorType = ingest.Truncate(errType, 200)
				event.Stack = ingest.Truncate(stack, 32000)
				event.Culprit = ingest.Truncate(ingest.Culprit(stack), 300)
				event.Tags = tags
				event.Context = capContext(context)
				event.User = userFromAttributes(attrs)
				event.Request = requestFromAttributes(attrs, traceID)
				event.URL = requestURL(event.Request)
				if kind == model.KindError {
					event.Fingerprint = ingest.Fingerprint(event.Source, event.ErrorType, event.Message, event.Culprit)
				}
				out = append(out, event)
			}
		}
	}
	return out
}

// ConvertTraces maps only error spans (or spans with exception events) to Catch
// error events. Catch does not currently store a full trace UI.
func ConvertTraces(req *collectortracev1.ExportTraceServiceRequest, project model.Project, now time.Time) []model.Event {
	var out []model.Event
	for _, rs := range req.GetResourceSpans() {
		ri := resourceInformation(rs.GetResource(), project)
		for _, ss := range rs.GetScopeSpans() {
			scopeAttrs := map[string]any{
				"name":    ss.GetScope().GetName(),
				"version": ss.GetScope().GetVersion(),
			}
			for _, span := range ss.GetSpans() {
				var exception *tracev1.Span_Event
				for _, ev := range span.GetEvents() {
					if ev.GetName() == "exception" || hasKey(ev.GetAttributes(), "exception.type") || hasKey(ev.GetAttributes(), "exception.message") {
						exception = ev
						break
					}
				}
				isError := span.GetStatus().GetCode() == tracev1.Status_STATUS_CODE_ERROR
				spanAttrs := attributesToMap(span.GetAttributes())
				spanException := hasKey(span.GetAttributes(), "exception.type") ||
					hasKey(span.GetAttributes(), "exception.message") ||
					hasKey(span.GetAttributes(), "exception.stacktrace")
				if !isError && exception == nil && !spanException {
					continue
				}
				excAttrs := map[string]any{}
				if exception != nil {
					excAttrs = attributesToMap(exception.GetAttributes())
				}
				// Some SDKs attach exception.* directly to the span instead of
				// emitting an exception event.
				if len(excAttrs) == 0 && (isError || spanException) {
					excAttrs = spanAttrs
				}
				message := stringAttribute(excAttrs, "exception.message")
				if message == "" {
					message = span.GetStatus().GetMessage()
				}
				if message == "" {
					message = span.GetName()
				}
				if message == "" {
					message = "OTel span error"
				}
				errType := stringAttribute(excAttrs, "exception.type")
				if errType == "" {
					errType = "SpanError"
				}
				stack := stringAttribute(excAttrs, "exception.stacktrace")
				traceID := hex.EncodeToString(span.GetTraceId())
				spanID := hex.EncodeToString(span.GetSpanId())
				ts := timestamp(span.GetEndTimeUnixNano(), now)
				if exception != nil && exception.GetTimeUnixNano() > 0 {
					ts = timestamp(exception.GetTimeUnixNano(), now)
				}
				tags := mergedTags(ri.attrs, spanAttrs)
				tags["span_name"] = ingest.Truncate(span.GetName(), 200)
				tags["otel_status"] = "ERROR"
				if traceID != "" {
					tags["trace_id"] = traceID
				}
				if spanID != "" {
					tags["span_id"] = spanID
				}
				context := map[string]any{
					"otel": map[string]any{
						"resource":     ri.attrs,
						"scope":        scopeAttrs,
						"attributes":   spanAttrs,
						"exception":    excAttrs,
						"traceId":      traceID,
						"spanId":       spanID,
						"parentSpanId": hex.EncodeToString(span.GetParentSpanId()),
						"spanName":     span.GetName(),
						"spanKind":     span.GetKind().String(),
						"status":       span.GetStatus().GetMessage(),
					},
				}
				event := baseEvent(ri, project, ts, now)
				event.Kind = model.KindError
				event.Level = "error"
				event.Message = ingest.Truncate(message, 8000)
				event.ErrorType = ingest.Truncate(errType, 200)
				event.Stack = ingest.Truncate(stack, 32000)
				event.Culprit = ingest.Truncate(ingest.Culprit(stack), 300)
				event.Tags = tags
				event.Context = capContext(context)
				event.User = userFromAttributes(spanAttrs)
				event.Request = requestFromAttributes(spanAttrs, traceID)
				if event.Request != nil && span.GetEndTimeUnixNano() > span.GetStartTimeUnixNano() {
					event.Request.DurationMS = int64((span.GetEndTimeUnixNano() - span.GetStartTimeUnixNano()) / 1e6)
				}
				event.URL = requestURL(event.Request)
				event.Fingerprint = ingest.Fingerprint(event.Source, event.ErrorType, event.Message, event.Culprit)
				out = append(out, event)
			}
		}
	}
	return out
}

// ConvertSpans maps every OTLP span into a stored trace span.
func ConvertSpans(req *collectortracev1.ExportTraceServiceRequest, project model.Project, now time.Time) []model.Span {
	var out []model.Span
	for _, rs := range req.GetResourceSpans() {
		ri := resourceInformation(rs.GetResource(), project)
		for _, ss := range rs.GetScopeSpans() {
			for _, span := range ss.GetSpans() {
				if len(span.GetTraceId()) == 0 || len(span.GetSpanId()) == 0 {
					continue
				}
				start := timestamp(span.GetStartTimeUnixNano(), now)
				end := timestamp(span.GetEndTimeUnixNano(), now)
				if end.Before(start) {
					end = start
				}
				status := "unset"
				switch span.GetStatus().GetCode() {
				case tracev1.Status_STATUS_CODE_OK:
					status = "ok"
				case tracev1.Status_STATUS_CODE_ERROR:
					status = "error"
				}
				events := make([]model.SpanEvent, 0, len(span.GetEvents()))
				for _, ev := range span.GetEvents() {
					events = append(events, model.SpanEvent{
						Time:       timestamp(ev.GetTimeUnixNano(), now),
						Name:       ev.GetName(),
						Attributes: attributesToMap(ev.GetAttributes()),
					})
				}
				kind := strings.TrimPrefix(span.GetKind().String(), "SPAN_KIND_")
				out = append(out, model.Span{
					TenantID: project.TenantID, ProjectID: project.ID,
					TraceID: hex.EncodeToString(span.GetTraceId()), SpanID: hex.EncodeToString(span.GetSpanId()),
					ParentSpanID: hex.EncodeToString(span.GetParentSpanId()),
					Name:         span.GetName(), Kind: strings.ToLower(kind),
					Service: ri.service, Environment: ri.env, Release: ri.version, Platform: ri.platform,
					StartTime: start, EndTime: end, DurationMS: end.Sub(start).Milliseconds(),
					Status: status, StatusMessage: span.GetStatus().GetMessage(),
					ScopeName: ss.GetScope().GetName(), ScopeVersion: ss.GetScope().GetVersion(),
					Attributes: attributesToMap(span.GetAttributes()), Resource: ri.attrs,
					Events: events, ReceivedAt: now,
				})
			}
		}
	}
	return out
}

// ConvertMetrics maps OTLP metrics into stored data points. Histograms and
// summaries use the average as Value so they can be charted uniformly.
func ConvertMetrics(req *collectormetricsv1.ExportMetricsServiceRequest, project model.Project, now time.Time) []model.MetricPoint {
	var out []model.MetricPoint
	for _, rm := range req.GetResourceMetrics() {
		ri := resourceInformation(rm.GetResource(), project)
		for _, sm := range rm.GetScopeMetrics() {
			for _, metric := range sm.GetMetrics() {
				if strings.TrimSpace(metric.GetName()) == "" {
					continue
				}
				base := model.MetricPoint{
					TenantID: project.TenantID, ProjectID: project.ID,
					Name: metric.GetName(), Description: metric.GetDescription(), Unit: metric.GetUnit(),
					Service: ri.service, Environment: ri.env, Release: ri.version, Platform: ri.platform,
					Resource: ri.attrs, ReceivedAt: now,
				}
				switch data := metric.GetData().(type) {
				case *metricv1.Metric_Gauge:
					base.Type = "gauge"
					for _, dp := range data.Gauge.GetDataPoints() {
						p := base
						p.Timestamp = timestamp(dp.GetTimeUnixNano(), now)
						p.StartTime = timestamp(dp.GetStartTimeUnixNano(), now)
						p.Value = numberValue(dp)
						p.Attributes = stringAttributes(dp.GetAttributes())
						out = append(out, p)
					}
				case *metricv1.Metric_Sum:
					base.Type = "sum"
					base.Temporality = temporality(data.Sum.GetAggregationTemporality())
					base.IsMonotonic = data.Sum.GetIsMonotonic()
					for _, dp := range data.Sum.GetDataPoints() {
						p := base
						p.Timestamp = timestamp(dp.GetTimeUnixNano(), now)
						p.StartTime = timestamp(dp.GetStartTimeUnixNano(), now)
						p.Value = numberValue(dp)
						p.Attributes = stringAttributes(dp.GetAttributes())
						out = append(out, p)
					}
				case *metricv1.Metric_Histogram:
					base.Type = "histogram"
					base.Temporality = temporality(data.Histogram.GetAggregationTemporality())
					for _, dp := range data.Histogram.GetDataPoints() {
						p := base
						p.Timestamp = timestamp(dp.GetTimeUnixNano(), now)
						p.StartTime = timestamp(dp.GetStartTimeUnixNano(), now)
						p.Count = dp.GetCount()
						if dp.Sum != nil {
							p.Sum = *dp.Sum
						}
						p.Value = average(p.Sum, p.Count)
						p.Min = dp.Min
						p.Max = dp.Max
						p.Attributes = stringAttributes(dp.GetAttributes())
						out = append(out, p)
					}
				case *metricv1.Metric_ExponentialHistogram:
					base.Type = "exponential_histogram"
					base.Temporality = temporality(data.ExponentialHistogram.GetAggregationTemporality())
					for _, dp := range data.ExponentialHistogram.GetDataPoints() {
						p := base
						p.Timestamp = timestamp(dp.GetTimeUnixNano(), now)
						p.StartTime = timestamp(dp.GetStartTimeUnixNano(), now)
						p.Count = dp.GetCount()
						if dp.Sum != nil {
							p.Sum = *dp.Sum
						}
						p.Value = average(p.Sum, p.Count)
						p.Min = dp.Min
						p.Max = dp.Max
						p.Attributes = stringAttributes(dp.GetAttributes())
						out = append(out, p)
					}
				case *metricv1.Metric_Summary:
					base.Type = "summary"
					for _, dp := range data.Summary.GetDataPoints() {
						p := base
						p.Timestamp = timestamp(dp.GetTimeUnixNano(), now)
						p.StartTime = timestamp(dp.GetStartTimeUnixNano(), now)
						p.Count = dp.GetCount()
						p.Sum = dp.GetSum()
						p.Value = average(p.Sum, p.Count)
						p.Attributes = stringAttributes(dp.GetAttributes())
						out = append(out, p)
					}
				}
			}
		}
	}
	return out
}

func numberValue(dp *metricv1.NumberDataPoint) float64 {
	switch v := dp.GetValue().(type) {
	case *metricv1.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	case *metricv1.NumberDataPoint_AsDouble:
		return v.AsDouble
	}
	return 0
}

func average(sum float64, count uint64) float64 {
	if count == 0 {
		return sum
	}
	return sum / float64(count)
}

func temporality(t metricv1.AggregationTemporality) string {
	switch t {
	case metricv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA:
		return "delta"
	case metricv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE:
		return "cumulative"
	}
	return ""
}

func stringAttributes(attrs []*commonv1.KeyValue) map[string]string {
	out := map[string]string{}
	for _, kv := range attrs {
		if kv == nil || kv.GetKey() == "" {
			continue
		}
		if v := anyValueString(kv.GetValue()); v != "" {
			out[ingest.Truncate(kv.GetKey(), 64)] = ingest.Truncate(v, 500)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func baseEvent(ri resourceInfo, project model.Project, ts, now time.Time) model.Event {
	return model.Event{
		TenantID:    project.TenantID,
		ProjectID:   project.ID,
		Source:      model.SourceBackend,
		Level:       "info",
		Timestamp:   ts,
		ReceivedAt:  now,
		Environment: ri.env,
		Release:     ri.version,
		Service:     ri.service,
		Platform:    ri.platform,
	}
}

func resourceInformation(res *resourcev1.Resource, project model.Project) resourceInfo {
	attrs := attributesToMap(res.GetAttributes())
	ri := resourceInfo{
		service:  stringAttribute(attrs, "service.name"),
		version:  stringAttribute(attrs, "service.version"),
		env:      stringAttribute(attrs, "deployment.environment.name"),
		platform: stringAttribute(attrs, "telemetry.sdk.language"),
		attrs:    attrs,
	}
	if ri.env == "" {
		ri.env = stringAttribute(attrs, "deployment.environment")
	}
	if ri.service == "" {
		ri.service = project.Name
	}
	if ns := stringAttribute(attrs, "service.namespace"); ns != "" {
		ri.service = ns + "/" + ri.service
	}
	if ri.platform == "" {
		ri.platform = project.Platform
	}
	if ri.version == "" {
		ri.version = stringAttribute(attrs, "deployment.version")
	}
	return ri
}

func attributesToMap(attrs []*commonv1.KeyValue) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, kv := range attrs {
		if kv == nil || kv.GetKey() == "" {
			continue
		}
		out[kv.GetKey()] = anyValue(kv.GetValue())
	}
	return out
}

func anyValue(v *commonv1.AnyValue) any {
	if v == nil {
		return nil
	}
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return x.StringValue
	case *commonv1.AnyValue_BoolValue:
		return x.BoolValue
	case *commonv1.AnyValue_IntValue:
		return x.IntValue
	case *commonv1.AnyValue_DoubleValue:
		return x.DoubleValue
	case *commonv1.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(x.BytesValue)
	case *commonv1.AnyValue_ArrayValue:
		values := make([]any, 0, len(x.ArrayValue.GetValues()))
		for _, item := range x.ArrayValue.GetValues() {
			values = append(values, anyValue(item))
		}
		return values
	case *commonv1.AnyValue_KvlistValue:
		values := map[string]any{}
		for _, kv := range x.KvlistValue.GetValues() {
			if kv != nil && kv.GetKey() != "" {
				values[kv.GetKey()] = anyValue(kv.GetValue())
			}
		}
		return values
	}
	return nil
}

func anyValueString(v *commonv1.AnyValue) string {
	if v == nil {
		return ""
	}
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return x.StringValue
	case *commonv1.AnyValue_BoolValue:
		return strconv.FormatBool(x.BoolValue)
	case *commonv1.AnyValue_IntValue:
		return strconv.FormatInt(x.IntValue, 10)
	case *commonv1.AnyValue_DoubleValue:
		return strconv.FormatFloat(x.DoubleValue, 'g', -1, 64)
	case *commonv1.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(x.BytesValue)
	default:
		raw, _ := json.Marshal(anyValue(v))
		return string(raw)
	}
}

func stringAttribute(attrs map[string]any, key string) string {
	v, ok := attrs[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	}
	return ""
}

func intAttribute(attrs map[string]any, keys ...string) int {
	for _, key := range keys {
		if raw, ok := attrs[key]; ok {
			switch x := raw.(type) {
			case int64:
				return int(x)
			case float64:
				return int(x)
			case string:
				if n, err := strconv.Atoi(x); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func severityLevel(num logsv1.SeverityNumber, text string) string {
	// Numeric severity is the normalized signal; severity text is the original
	// vendor string and is used only when the number is unspecified.
	switch {
	case num >= logsv1.SeverityNumber_SEVERITY_NUMBER_FATAL:
		return "fatal"
	case num >= logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR:
		return "error"
	case num >= logsv1.SeverityNumber_SEVERITY_NUMBER_WARN:
		return "warn"
	case num >= logsv1.SeverityNumber_SEVERITY_NUMBER_INFO:
		return "info"
	case num >= logsv1.SeverityNumber_SEVERITY_NUMBER_TRACE:
		return "debug"
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "trace", "debug":
		return "debug"
	case "info", "notice", "log":
		return "info"
	case "warn", "warning":
		return "warn"
	case "error", "err":
		return "error"
	case "fatal", "critical", "crit", "panic":
		return "fatal"
	}
	return "info"
}

func timestamp(nanos uint64, now time.Time) time.Time {
	if nanos == 0 {
		return now
	}
	t := time.Unix(0, int64(nanos)).UTC()
	if t.After(now.Add(5*time.Minute)) || t.Before(now.AddDate(-1, 0, 0)) {
		return now
	}
	return t
}

func mergedTags(resource map[string]any, attrs map[string]any) map[string]string {
	out := map[string]string{}
	add := func(m map[string]any) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if len(out) >= 30 {
				return
			}
			v := stringAttribute(m, k)
			if v == "" {
				continue
			}
			out[ingest.Truncate(k, 64)] = ingest.Truncate(v, 500)
		}
	}
	add(resource)
	add(attrs)
	if len(out) == 0 {
		return nil
	}
	return out
}

func userFromAttributes(attrs map[string]any) *model.EventUser {
	id := firstString(attrs, "enduser.id", "user.id", "user_id")
	email := firstString(attrs, "enduser.email", "user.email", "user_email")
	name := firstString(attrs, "enduser.name", "user.name", "user_name")
	if id == "" && email == "" && name == "" {
		return nil
	}
	return &model.EventUser{ID: id, Email: email, Username: name}
}

func requestFromAttributes(attrs map[string]any, traceID string) *model.EventRequest {
	method := firstString(attrs, "http.request.method", "http.method")
	url := firstString(attrs, "url.full", "http.url", "http.target", "http.route")
	status := intAttribute(attrs, "http.response.status_code", "http.status_code")
	if method == "" && url == "" && status == 0 && traceID == "" {
		return nil
	}
	return &model.EventRequest{
		Method: method, URL: ingest.Truncate(url, 2048), Status: status,
		RequestID: ingest.Truncate(traceID, 200),
	}
}

func requestURL(req *model.EventRequest) string {
	if req == nil {
		return ""
	}
	return req.URL
}

func firstString(attrs map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := stringAttribute(attrs, key); v != "" {
			return v
		}
	}
	return ""
}

func hasKey(attrs []*commonv1.KeyValue, key string) bool {
	for _, kv := range attrs {
		if kv.GetKey() == key {
			return true
		}
	}
	return false
}

func capContext(ctx map[string]any) map[string]any {
	raw, err := json.Marshal(ctx)
	if err != nil {
		return map[string]any{"_truncated": true}
	}
	if len(raw) > 16<<10 {
		return map[string]any{"_truncated": true, "size": len(raw)}
	}
	return ctx
}
