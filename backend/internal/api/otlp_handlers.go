package api

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/otlp"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/catch/catch/backend/internal/store"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	maxOTLPBytes  = 20 << 20
	maxOTLPEvents = 5000
)

var errOTLPTooLarge = errors.New("OTLP payload too large")

// otlpLogs receives OTLP/HTTP logs (protobuf or JSON).
func (a *API) otlpLogs(w http.ResponseWriter, r *http.Request) {
	tenant, project, ok := a.otlpTarget(w, r)
	if !ok {
		return
	}
	r = a.otlpContext(r, tenant, project)
	body, err := readOTLPBody(w, r)
	if err != nil {
		writeOTLPReadError(w, err)
		return
	}
	req, err := otlp.DecodeLogs(body, r.Header.Get("Content-Type"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_otlp", err.Error())
		return
	}
	rejected, err := a.processOTLPLogs(r.Context(), tenant, project, req)
	if err != nil {
		a.writeOTLPProcessError(w, r, project, err)
		return
	}
	a.writeOTLPResponse(w, r, "logs", rejected, partialMessage(rejected, "日志"))
}

// otlpTraces receives OTLP/HTTP traces, stores all spans, and turns error spans
// into Catch issues.
func (a *API) otlpTraces(w http.ResponseWriter, r *http.Request) {
	tenant, project, ok := a.otlpTarget(w, r)
	if !ok {
		return
	}
	r = a.otlpContext(r, tenant, project)
	body, err := readOTLPBody(w, r)
	if err != nil {
		writeOTLPReadError(w, err)
		return
	}
	req, err := otlp.DecodeTraces(body, r.Header.Get("Content-Type"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_otlp", err.Error())
		return
	}
	rejected, err := a.processOTLPTraces(r.Context(), tenant, project, req)
	if err != nil {
		a.writeOTLPProcessError(w, r, project, err)
		return
	}
	a.writeOTLPResponse(w, r, "traces", rejected, partialMessage(rejected, "span"))
}

func (a *API) otlpMetrics(w http.ResponseWriter, r *http.Request) {
	tenant, project, ok := a.otlpTarget(w, r)
	if !ok {
		return
	}
	r = a.otlpContext(r, tenant, project)
	body, err := readOTLPBody(w, r)
	if err != nil {
		writeOTLPReadError(w, err)
		return
	}
	req, err := otlp.DecodeMetrics(body, r.Header.Get("Content-Type"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_otlp", err.Error())
		return
	}
	rejected, err := a.processOTLPMetrics(r.Context(), project, req)
	if err != nil {
		a.writeOTLPProcessError(w, r, project, err)
		return
	}
	a.writeOTLPResponse(w, r, "metrics", rejected, partialMessage(rejected, "数据点"))
}

func (a *API) writeOTLPProcessError(w http.ResponseWriter, r *http.Request, project model.Project, err error) {
	if errors.Is(err, errOTLPRateLimited) {
		w.Header().Set("Retry-After", "60")
		a.logger.Slog.WarnContext(r.Context(), "otlp rate limited", "projectId", project.ID.Hex())
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "采集速率超限，请稍后重试")
		return
	}
	a.logger.Slog.ErrorContext(r.Context(), "otlp persist failed", "error", err, "projectId", project.ID.Hex())
	httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存 OTLP 数据")
}

func (a *API) otlpContext(r *http.Request, tenant model.Tenant, project model.Project) *http.Request {
	ctx := reqctx.WithTenant(r.Context(), tenant)
	ctx = reqctx.WithProject(ctx, project)
	if h := reqctx.HolderFrom(ctx); h != nil {
		h.SetTenant(tenant)
		h.SetProject(project)
	}
	return r.WithContext(ctx)
}

func writeOTLPReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, errOTLPTooLarge) {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error())
		return
	}
	httpx.Error(w, http.StatusBadRequest, "invalid_otlp_body", err.Error())
}

func readOTLPBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body, err := readLimited(w, r, maxOTLPBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errOTLPTooLarge, err)
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip") {
		return body, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("无法解压 gzip OTLP 请求")
	}
	defer zr.Close()
	raw, err := io.ReadAll(io.LimitReader(zr, maxOTLPBytes+1))
	if err != nil {
		return nil, errors.New("无法解压 gzip OTLP 请求")
	}
	if len(raw) > maxOTLPBytes {
		return nil, fmt.Errorf("%w: 解压后的 OTLP 请求超过 20MB", errOTLPTooLarge)
	}
	return raw, nil
}

func (a *API) otlpTarget(w http.ResponseWriter, r *http.Request) (model.Tenant, model.Project, bool) {
	key := extractIngestKey(r)
	if key == "" {
		httpx.Error(w, http.StatusUnauthorized, "missing_ingest_key", "缺少项目采集密钥")
		return model.Tenant{}, model.Project{}, false
	}
	project, err := a.store.ProjectByKey(r.Context(), key)
	if err != nil {
		if store.IsNoDocuments(err) {
			httpx.Error(w, http.StatusUnauthorized, "invalid_ingest_key", "采集密钥无效")
			return model.Tenant{}, model.Project{}, false
		}
		a.logError(r, "load otlp project failed", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法校验采集密钥")
		return model.Tenant{}, model.Project{}, false
	}
	tenant, err := a.store.TenantByID(r.Context(), project.TenantID)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_ingest_key", "采集密钥无效")
		return model.Tenant{}, model.Project{}, false
	}
	return tenant, project, true
}

func (a *API) writeOTLPResponse(w http.ResponseWriter, r *http.Request, signal string, rejected int64, message string) {
	jsonResponse := strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json")
	var payload []byte
	var err error
	switch signal {
	case "logs":
		resp := &collectorlogsv1.ExportLogsServiceResponse{}
		if rejected > 0 {
			resp.PartialSuccess = &collectorlogsv1.ExportLogsPartialSuccess{RejectedLogRecords: rejected, ErrorMessage: message}
		}
		if jsonResponse {
			payload, err = protojson.Marshal(resp)
		} else {
			payload, err = proto.Marshal(resp)
		}
	case "traces":
		resp := &collectortracev1.ExportTraceServiceResponse{}
		if rejected > 0 {
			resp.PartialSuccess = &collectortracev1.ExportTracePartialSuccess{RejectedSpans: rejected, ErrorMessage: message}
		}
		if jsonResponse {
			payload, err = protojson.Marshal(resp)
		} else {
			payload, err = proto.Marshal(resp)
		}
	case "metrics":
		resp := &collectormetricsv1.ExportMetricsServiceResponse{}
		if rejected > 0 {
			resp.PartialSuccess = &collectormetricsv1.ExportMetricsPartialSuccess{RejectedDataPoints: rejected, ErrorMessage: message}
		}
		if jsonResponse {
			payload, err = protojson.Marshal(resp)
		} else {
			payload, err = proto.Marshal(resp)
		}
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法编码 OTLP 响应")
		return
	}
	if jsonResponse {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "application/x-protobuf")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func partialMessage(rejected int64, unit string) string {
	if rejected <= 0 {
		return ""
	}
	return fmt.Sprintf("单次最多处理 %d 条%s，已拒绝 %d 条", maxOTLPEvents, unit, rejected)
}
