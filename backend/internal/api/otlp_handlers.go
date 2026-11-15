package api

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/catch/catch/backend/internal/httpx"
	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/otlp"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/catch/catch/backend/internal/store"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

const maxOTLPBytes = 20 << 20

// otlpLogs receives OTLP/HTTP logs (protobuf or JSON).
func (a *API) otlpLogs(w http.ResponseWriter, r *http.Request) {
	tenant, project, ok := a.otlpTarget(w, r)
	if !ok {
		return
	}
	ctx := reqctx.WithTenant(r.Context(), tenant)
	ctx = reqctx.WithProject(ctx, project)
	r = r.WithContext(ctx)

	body, err := readOTLPBody(w, r)
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error())
		return
	}
	req, err := otlp.DecodeLogs(body, r.Header.Get("Content-Type"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_otlp", err.Error())
		return
	}
	events := otlp.ConvertLogs(req, project, time.Now().UTC())
	a.acceptOTLP(w, r, tenant, project, events, true)
}

// otlpTraces receives OTLP/HTTP traces and keeps only error spans/exceptions.
func (a *API) otlpTraces(w http.ResponseWriter, r *http.Request) {
	tenant, project, ok := a.otlpTarget(w, r)
	if !ok {
		return
	}
	ctx := reqctx.WithTenant(r.Context(), tenant)
	ctx = reqctx.WithProject(ctx, project)
	r = r.WithContext(ctx)

	body, err := readOTLPBody(w, r)
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error())
		return
	}
	req, err := otlp.DecodeTraces(body, r.Header.Get("Content-Type"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_otlp", err.Error())
		return
	}
	events := otlp.ConvertTraces(req, project, time.Now().UTC())
	a.acceptOTLP(w, r, tenant, project, events, false)
}

func readOTLPBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body, err := readLimited(w, r, maxOTLPBytes)
	if err != nil {
		return nil, err
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
		return nil, errors.New("解压后的 OTLP 请求超过 20MB")
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

func (a *API) acceptOTLP(w http.ResponseWriter, r *http.Request, tenant model.Tenant, project model.Project, events []model.Event, logs bool) {
	if len(events) == 0 {
		a.writeOTLPResponse(w, r, logs)
		return
	}
	if !a.limiter.allow(project.ID.Hex(), len(events)) {
		w.Header().Set("Retry-After", "60")
		a.logger.Slog.WarnContext(r.Context(), "otlp rate limited",
			"projectId", project.ID.Hex(), "batch", len(events), "limit", a.cfg.RateLimit)
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "采集速率超限，请稍后重试")
		return
	}
	if _, _, err := a.persistEvents(r.Context(), tenant, project, events); err != nil {
		a.logger.Slog.ErrorContext(r.Context(), "otlp persist failed", "error", err, "projectId", project.ID.Hex())
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "无法保存 OTLP 数据")
		return
	}
	a.writeOTLPResponse(w, r, logs)
}

func (a *API) writeOTLPResponse(w http.ResponseWriter, r *http.Request, logs bool) {
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json") {
		httpx.JSON(w, http.StatusOK, map[string]any{})
		return
	}
	var body []byte
	if logs {
		body, _ = proto.Marshal(&collectorlogsv1.ExportLogsServiceResponse{})
	} else {
		body, _ = proto.Marshal(&collectortracev1.ExportTraceServiceResponse{})
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
