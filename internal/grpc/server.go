package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/api/dataplane"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/networkpolicy"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Server implements the Nova gRPC service
type Server struct {
	novapb.UnimplementedNovaServiceServer
	store           *store.Store
	executor        *executor.Executor
	server          *grpc.Server
	dataPlaneRouter http.Handler
}

// NewServer creates a new gRPC server
func NewServer(s *store.Store, exec *executor.Executor, p *pool.Pool) *Server {
	mux := http.NewServeMux()
	dpHandler := &dataplane.Handler{
		Store: s,
		Exec:  exec,
		Pool:  p,
	}
	dpHandler.RegisterRoutes(mux)

	return &Server{
		store:           s,
		executor:        exec,
		dataPlaneRouter: mux,
	}
}

// Start starts the gRPC server on the given address
func (s *Server) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.server = grpc.NewServer()
	novapb.RegisterNovaServiceServer(s.server, s)

	logging.Op().Info("gRPC server started", "addr", addr)

	go func() {
		if err := s.server.Serve(lis); err != nil {
			logging.Op().Error("gRPC server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the gRPC server
func (s *Server) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}

// Invoke handles synchronous function invocation
func (s *Server) Invoke(ctx context.Context, req *novapb.InvokeRequest) (*novapb.InvokeResponse, error) {
	ctx = applyTenantScopeFromMetadata(ctx)

	if req.Function == "" {
		return nil, status.Error(codes.InvalidArgument, "function name is required")
	}

	// Validate payload is valid JSON (or empty)
	var payload json.RawMessage
	if len(req.Payload) > 0 {
		if !json.Valid(req.Payload) {
			return nil, status.Error(codes.InvalidArgument, "payload must be valid JSON")
		}
		payload = req.Payload
	} else {
		payload = json.RawMessage("{}")
	}

	fn, err := s.store.GetFunctionByName(ctx, req.Function)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "function not found: %v", err)
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if err := networkpolicy.EnforceIngress(fn.Name, fn.NetworkPolicy, ingressCallerFromMetadata(md)); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	// Invoke function
	resp, err := s.executor.Invoke(ctx, req.Function, payload)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "invoke failed: %v", err)
	}

	return &novapb.InvokeResponse{
		RequestId:  resp.RequestID,
		Output:     resp.Output,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
		ColdStart:  resp.ColdStart,
	}, nil
}

// InvokeAsync handles asynchronous function invocation.
func (s *Server) InvokeAsync(ctx context.Context, req *novapb.InvokeRequest) (*novapb.InvokeAsyncResponse, error) {
	ctx = applyTenantScopeFromMetadata(ctx)

	if req.Function == "" {
		return nil, status.Error(codes.InvalidArgument, "function name is required")
	}

	var payload json.RawMessage
	if len(req.Payload) > 0 {
		if !json.Valid(req.Payload) {
			return nil, status.Error(codes.InvalidArgument, "payload must be valid JSON")
		}
		payload = req.Payload
	} else {
		payload = json.RawMessage("{}")
	}

	fn, err := s.store.GetFunctionByName(ctx, req.Function)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "function not found: %v", err)
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if err := networkpolicy.EnforceIngress(fn.Name, fn.NetworkPolicy, ingressCallerFromMetadata(md)); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}

	inv := store.NewAsyncInvocation(fn.ID, fn.Name, payload)

	if req.Metadata != nil {
		if maxAttempts := parsePositiveInt(req.Metadata["max_attempts"]); maxAttempts > 0 {
			inv.MaxAttempts = maxAttempts
		}
		if backoffBase := parsePositiveInt(req.Metadata["backoff_base_ms"]); backoffBase > 0 {
			inv.BackoffBaseMS = backoffBase
		}
		if backoffMax := parsePositiveInt(req.Metadata["backoff_max_ms"]); backoffMax > 0 {
			inv.BackoffMaxMS = backoffMax
		}
		if inv.BackoffMaxMS < inv.BackoffBaseMS {
			inv.BackoffMaxMS = inv.BackoffBaseMS
		}
	}

	idempotencyKey := ""
	idempotencyTTL := store.DefaultAsyncIdempotencyTTL
	if req.Metadata != nil {
		idempotencyKey = strings.TrimSpace(req.Metadata["idempotency_key"])
		if ttlSeconds := parsePositiveInt(req.Metadata["idempotency_ttl_s"]); ttlSeconds > 0 {
			idempotencyTTL = time.Duration(ttlSeconds) * time.Second
		}
	}

	if idempotencyKey != "" {
		enqueued, deduplicated, err := s.store.EnqueueAsyncInvocationWithIdempotency(ctx, inv, idempotencyKey, idempotencyTTL)
		if err != nil {
			if errors.Is(err, store.ErrInvalidIdempotencyKey) {
				return nil, status.Errorf(codes.InvalidArgument, "%v", err)
			}
			return nil, status.Errorf(codes.Internal, "enqueue async invocation: %v", err)
		}
		state := "queued"
		if deduplicated {
			state = "replay"
		}
		return &novapb.InvokeAsyncResponse{
			RequestId: enqueued.ID,
			Status:    state,
		}, nil
	}

	if err := s.store.EnqueueAsyncInvocation(ctx, inv); err != nil {
		return nil, status.Errorf(codes.Internal, "enqueue async invocation: %v", err)
	}

	return &novapb.InvokeAsyncResponse{
		RequestId: inv.ID,
		Status:    "queued",
	}, nil
}

// GetFunction returns function metadata
func (s *Server) GetFunction(ctx context.Context, req *novapb.GetFunctionRequest) (*novapb.FunctionInfo, error) {
	ctx = applyTenantScopeFromMetadata(ctx)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "function name is required")
	}

	fn, err := s.store.GetFunctionByName(ctx, req.Name)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "function not found: %v", err)
	}

	return &novapb.FunctionInfo{
		Id:          fn.ID,
		Name:        fn.Name,
		Runtime:     string(fn.Runtime),
		Handler:     fn.Handler,
		MemoryMb:    int32(fn.MemoryMB),
		TimeoutS:    int32(fn.TimeoutS),
		Mode:        string(fn.Mode),
		MinReplicas: int32(fn.MinReplicas),
		Version:     int32(fn.Version),
	}, nil
}

// ListFunctions returns all registered functions
func (s *Server) ListFunctions(ctx context.Context, req *novapb.ListFunctionsRequest) (*novapb.ListFunctionsResponse, error) {
	ctx = applyTenantScopeFromMetadata(ctx)

	funcs, err := s.store.ListFunctions(ctx, 0, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list functions: %v", err)
	}

	resp := &novapb.ListFunctionsResponse{
		Functions: make([]*novapb.FunctionInfo, 0, len(funcs)),
	}

	for _, fn := range funcs {
		resp.Functions = append(resp.Functions, &novapb.FunctionInfo{
			Id:          fn.ID,
			Name:        fn.Name,
			Runtime:     string(fn.Runtime),
			Handler:     fn.Handler,
			MemoryMb:    int32(fn.MemoryMB),
			TimeoutS:    int32(fn.TimeoutS),
			Mode:        string(fn.Mode),
			MinReplicas: int32(fn.MinReplicas),
			Version:     int32(fn.Version),
		})
	}

	return resp, nil
}

// HealthCheck returns service health status
func (s *Server) HealthCheck(ctx context.Context, req *novapb.HealthCheckRequest) (*novapb.HealthCheckResponse, error) {
	ctx = applyTenantScopeFromMetadata(ctx)

	components := make(map[string]string)
	serviceStatus := "ok"

	// Check Postgres
	if err := s.store.PingPostgres(ctx); err != nil {
		components["postgres"] = "unhealthy: " + err.Error()
		serviceStatus = "degraded"
	} else {
		components["postgres"] = "healthy"
	}

	components["grpc"] = "healthy"

	return &novapb.HealthCheckResponse{
		Status:     serviceStatus,
		Components: components,
	}, nil
}

// ProxyHTTP proxies data-plane HTTP requests via gRPC.
func (s *Server) ProxyHTTP(ctx context.Context, req *novapb.ProxyHTTPRequest) (*novapb.ProxyHTTPResponse, error) {
	ctx = applyTenantScopeFromMetadata(ctx)

	if s.dataPlaneRouter == nil {
		return nil, status.Error(codes.Unavailable, "data plane router unavailable")
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if query := strings.TrimPrefix(strings.TrimSpace(req.Query), "?"); query != "" {
		path = path + "?" + query
	}

	httpReq := httptest.NewRequest(method, path, bytes.NewReader(req.Body)).WithContext(ctx)
	for key, value := range req.Headers {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		httpReq.Header.Set(k, value)
	}

	rec := httptest.NewRecorder()
	s.dataPlaneRouter.ServeHTTP(rec, httpReq)

	resp := rec.Result()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read proxied response body: %v", err)
	}

	headers := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		if len(values) == 0 {
			continue
		}
		headers[key] = strings.Join(values, ", ")
	}

	return &novapb.ProxyHTTPResponse{
		StatusCode: int32(resp.StatusCode),
		Body:       body,
		Headers:    headers,
	}, nil
}

func parsePositiveInt(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func ingressCallerFromMetadata(md metadata.MD) networkpolicy.Caller {
	port := parsePositiveInt(metadataValue(md, "x-nova-source-port"))
	if port > 65535 {
		port = 0
	}
	return networkpolicy.Caller{
		SourceFunction: metadataValue(md, "x-nova-source-function"),
		SourceIP:       metadataValue(md, "x-nova-source-ip"),
		Protocol:       metadataValue(md, "x-nova-source-protocol"),
		Port:           port,
	}
}

func applyTenantScopeFromMetadata(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	currentScope := store.TenantScopeFromContext(ctx)
	tenantID := metadataValue(md, "x-nova-tenant")
	namespace := metadataValue(md, "x-nova-namespace")

	if tenantID == "" {
		tenantID = currentScope.TenantID
	}
	if namespace == "" {
		namespace = currentScope.Namespace
	}
	if tenantID == currentScope.TenantID && namespace == currentScope.Namespace {
		return ctx
	}

	return store.WithTenantScope(ctx, tenantID, namespace)
}

func metadataValue(md metadata.MD, key string) string {
	values := md.Get(strings.ToLower(strings.TrimSpace(key)))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
