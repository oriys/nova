package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the Nova gRPC service
type Server struct {
	novapb.UnimplementedNovaServiceServer
	store    *store.RedisStore
	executor *executor.Executor
	server   *grpc.Server
}

// NewServer creates a new gRPC server
func NewServer(s *store.RedisStore, exec *executor.Executor) *Server {
	return &Server{
		store:    s,
		executor: exec,
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

// InvokeAsync handles asynchronous function invocation (placeholder)
func (s *Server) InvokeAsync(ctx context.Context, req *novapb.InvokeRequest) (*novapb.InvokeAsyncResponse, error) {
	// TODO: Implement async invocation with background execution
	return nil, status.Error(codes.Unimplemented, "async invocation not implemented")
}

// GetFunction returns function metadata
func (s *Server) GetFunction(ctx context.Context, req *novapb.GetFunctionRequest) (*novapb.FunctionInfo, error) {
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
	funcs, err := s.store.ListFunctions(ctx)
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
	components := make(map[string]string)

	// Check Redis
	if err := s.store.Ping(ctx); err != nil {
		components["redis"] = "unhealthy: " + err.Error()
	} else {
		components["redis"] = "healthy"
	}

	components["grpc"] = "healthy"

	return &novapb.HealthCheckResponse{
		Status:     "ok",
		Components: components,
	}, nil
}
