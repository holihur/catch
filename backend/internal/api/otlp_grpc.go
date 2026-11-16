package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/catch/catch/backend/internal/model"
	"github.com/catch/catch/backend/internal/reqctx"
	"github.com/catch/catch/backend/internal/secure"
	"github.com/catch/catch/backend/internal/store"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// NewGRPCServer creates the OTLP gRPC server (logs, traces, metrics).
func (a *API) NewGRPCServer() (*grpc.Server, error) {
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxOTLPBytes),
		grpc.MaxSendMsgSize(maxOTLPBytes),
		grpc.UnaryInterceptor(a.grpcRecovery),
	}
	if a.cfg.GRPCTLSCert != "" || a.cfg.GRPCTLSKey != "" || a.cfg.GRPCTLSClientCA != "" {
		if a.cfg.GRPCTLSCert == "" || a.cfg.GRPCTLSKey == "" {
			return nil, errors.New("GRPC_TLS_CERT and GRPC_TLS_KEY must both be set")
		}
		cert, err := tls.LoadX509KeyPair(a.cfg.GRPCTLSCert, a.cfg.GRPCTLSKey)
		if err != nil {
			return nil, fmt.Errorf("load grpc tls keypair: %w", err)
		}
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
		if a.cfg.GRPCTLSClientCA != "" {
			pem, err := os.ReadFile(a.cfg.GRPCTLSClientCA)
			if err != nil {
				return nil, fmt.Errorf("read grpc client CA: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("GRPC_TLS_CLIENT_CA contains no valid certificates")
			}
			tlsConfig.ClientCAs = pool
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	} else if !a.cfg.GRPCAllowInsecure {
		return nil, errors.New("OTLP gRPC is disabled: configure GRPC_TLS_CERT/GRPC_TLS_KEY (and optional GRPC_TLS_CLIENT_CA for mTLS), or set GRPC_ALLOW_INSECURE=true")
	}
	s := grpc.NewServer(opts...)
	collectorlogsv1.RegisterLogsServiceServer(s, &otlpGRPCLogs{api: a})
	collectortracev1.RegisterTraceServiceServer(s, &otlpGRPCTraces{api: a})
	collectormetricsv1.RegisterMetricsServiceServer(s, &otlpGRPCMetrics{api: a})
	return s, nil
}

func (a *API) grpcRecovery(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			a.logger.Slog.ErrorContext(ctx, "grpc panic recovered", "method", info.FullMethod, "panic", rec)
			err = status.Error(codes.Internal, "internal server error")
		}
	}()
	return handler(ctx, req)
}

type otlpGRPCLogs struct {
	collectorlogsv1.UnimplementedLogsServiceServer
	api *API
}

func (g *otlpGRPCLogs) Export(ctx context.Context, req *collectorlogsv1.ExportLogsServiceRequest) (*collectorlogsv1.ExportLogsServiceResponse, error) {
	tenant, project, ctx, err := g.api.grpcTarget(ctx)
	if err != nil {
		return nil, err
	}
	rejected, err := g.api.processOTLPLogs(ctx, tenant, project, req)
	if err != nil {
		return nil, g.api.grpcProcessError(ctx, project, err)
	}
	resp := &collectorlogsv1.ExportLogsServiceResponse{}
	if rejected > 0 {
		resp.PartialSuccess = &collectorlogsv1.ExportLogsPartialSuccess{
			RejectedLogRecords: rejected, ErrorMessage: partialMessage(rejected, "日志"),
		}
	}
	g.api.logger.Slog.InfoContext(ctx, "otlp grpc logs accepted", "projectId", project.ID.Hex(), "rejected", rejected)
	return resp, nil
}

type otlpGRPCTraces struct {
	collectortracev1.UnimplementedTraceServiceServer
	api *API
}

func (g *otlpGRPCTraces) Export(ctx context.Context, req *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	tenant, project, ctx, err := g.api.grpcTarget(ctx)
	if err != nil {
		return nil, err
	}
	rejected, err := g.api.processOTLPTraces(ctx, tenant, project, req)
	if err != nil {
		return nil, g.api.grpcProcessError(ctx, project, err)
	}
	resp := &collectortracev1.ExportTraceServiceResponse{}
	if rejected > 0 {
		resp.PartialSuccess = &collectortracev1.ExportTracePartialSuccess{
			RejectedSpans: rejected, ErrorMessage: partialMessage(rejected, "span"),
		}
	}
	g.api.logger.Slog.InfoContext(ctx, "otlp grpc traces accepted", "projectId", project.ID.Hex(), "rejected", rejected)
	return resp, nil
}

type otlpGRPCMetrics struct {
	collectormetricsv1.UnimplementedMetricsServiceServer
	api *API
}

func (g *otlpGRPCMetrics) Export(ctx context.Context, req *collectormetricsv1.ExportMetricsServiceRequest) (*collectormetricsv1.ExportMetricsServiceResponse, error) {
	tenant, project, ctx, err := g.api.grpcTarget(ctx)
	if err != nil {
		return nil, err
	}
	_ = tenant
	rejected, err := g.api.processOTLPMetrics(ctx, project, req)
	if err != nil {
		return nil, g.api.grpcProcessError(ctx, project, err)
	}
	resp := &collectormetricsv1.ExportMetricsServiceResponse{}
	if rejected > 0 {
		resp.PartialSuccess = &collectormetricsv1.ExportMetricsPartialSuccess{
			RejectedDataPoints: rejected, ErrorMessage: partialMessage(rejected, "数据点"),
		}
	}
	g.api.logger.Slog.InfoContext(ctx, "otlp grpc metrics accepted", "projectId", project.ID.Hex(), "rejected", rejected)
	return resp, nil
}

func (a *API) grpcTarget(ctx context.Context) (model.Tenant, model.Project, context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	key := grpcKey(md)
	if key == "" {
		return model.Tenant{}, model.Project{}, ctx, status.Error(codes.Unauthenticated, "missing ingest key")
	}
	project, err := a.store.ProjectByKey(ctx, key)
	if err != nil {
		if store.IsNoDocuments(err) {
			return model.Tenant{}, model.Project{}, ctx, status.Error(codes.Unauthenticated, "invalid ingest key")
		}
		return model.Tenant{}, model.Project{}, ctx, status.Error(codes.Internal, "project lookup failed")
	}
	tenant, err := a.store.TenantByID(ctx, project.TenantID)
	if err != nil {
		return model.Tenant{}, model.Project{}, ctx, status.Error(codes.Unauthenticated, "invalid ingest key")
	}
	requestID := firstMetadata(md, "x-request-id")
	if requestID == "" {
		requestID = secure.RandomToken(12)
	}
	ctx = reqctx.WithRequestID(ctx, requestID)
	ctx = reqctx.WithTenant(ctx, tenant)
	ctx = reqctx.WithProject(ctx, project)
	return tenant, project, ctx, nil
}

func grpcKey(md metadata.MD) string {
	if v := firstMetadata(md, "x-ingest-key"); v != "" {
		return v
	}
	if v := firstMetadata(md, "x-project-key"); v != "" {
		return v
	}
	auth := firstMetadata(md, "authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func firstMetadata(md metadata.MD, key string) string {
	values := md.Get(strings.ToLower(key))
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func (a *API) grpcProcessError(ctx context.Context, project model.Project, err error) error {
	if errors.Is(err, errOTLPRateLimited) {
		return status.Error(codes.ResourceExhausted, "采集速率超限，请稍后重试")
	}
	a.logger.Slog.ErrorContext(ctx, "otlp grpc persist failed", "error", err, "projectId", project.ID.Hex())
	return status.Error(codes.Internal, "无法保存 OTLP 数据")
}
