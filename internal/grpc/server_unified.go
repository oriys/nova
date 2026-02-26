package grpc

import (
"fmt"
"net"

"github.com/oriys/nova/internal/compiler"
"github.com/oriys/nova/internal/executor"
"github.com/oriys/nova/internal/logging"
"github.com/oriys/nova/internal/pool"
"github.com/oriys/nova/internal/service"
"github.com/oriys/nova/internal/store"
"google.golang.org/grpc"
"google.golang.org/grpc/credentials"
"google.golang.org/grpc/health"
"google.golang.org/grpc/health/grpc_health_v1"
"google.golang.org/grpc/reflection"
)

// UnifiedServer manages both data plane and control plane gRPC services
type UnifiedServer struct {
dataPlane    *DataPlaneServer
controlPlane *ControlPlaneServer
grpcServer   *grpc.Server
listener     net.Listener
}

// Config holds configuration for the unified gRPC server
type Config struct {
Address string

// TLS settings (optional)
CertFile string
KeyFile  string

// Inter-service auth token (optional)
ServiceToken string

// Data plane dependencies
Store    *store.Store
Executor *executor.Executor
Pool     *pool.Pool

// Control plane dependencies
FunctionService *service.FunctionService
Compiler        *compiler.Compiler
}

// NewUnifiedServer creates a new unified gRPC server with both planes
func NewUnifiedServer(cfg *Config) (*UnifiedServer, error) {
// Create data plane server
dataPlane := NewDataPlaneServer(cfg.Store, cfg.Executor, cfg.Pool)

// Create control plane server
controlPlane := NewControlPlaneServer(cfg.Store, cfg.FunctionService, cfg.Compiler)

// Create gRPC server with interceptors
opts := []grpc.ServerOption{
grpc.ChainUnaryInterceptor(
serviceAuthInterceptor(cfg.ServiceToken),
loggingInterceptor,
errorHandlingInterceptor,
),
}
if cfg.CertFile != "" && cfg.KeyFile != "" {
creds, err := credentials.NewServerTLSFromFile(cfg.CertFile, cfg.KeyFile)
if err != nil {
return nil, fmt.Errorf("load TLS credentials: %w", err)
}
opts = append(opts, grpc.Creds(creds))
logging.Op().Info("unified gRPC TLS enabled", "cert", cfg.CertFile)
} else {
logging.Op().Warn("unified gRPC server running without TLS")
}

grpcServer := grpc.NewServer(opts...)

// Register health service
healthServer := health.NewServer()
grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

// Enable reflection for debugging
reflection.Register(grpcServer)

return &UnifiedServer{
dataPlane:    dataPlane,
controlPlane: controlPlane,
grpcServer:   grpcServer,
}, nil
}

// Start starts the unified gRPC server
func (s *UnifiedServer) Start(address string) error {
lis, err := net.Listen("tcp", address)
if err != nil {
return fmt.Errorf("failed to listen: %w", err)
}

s.listener = lis
logging.Op().Info("unified gRPC server starting", "address", address)

go func() {
if err := s.grpcServer.Serve(lis); err != nil {
logging.Op().Error("gRPC server error", "error", err)
}
}()

logging.Op().Info("unified gRPC server started", "address", address)
return nil
}

// Stop gracefully stops the gRPC server
func (s *UnifiedServer) Stop() {
if s.grpcServer != nil {
logging.Op().Info("stopping gRPC server")
s.grpcServer.GracefulStop()
}
if s.listener != nil {
s.listener.Close()
}
}

// GetDataPlane returns the data plane server
func (s *UnifiedServer) GetDataPlane() *DataPlaneServer {
return s.dataPlane
}

// GetControlPlane returns the control plane server
func (s *UnifiedServer) GetControlPlane() *ControlPlaneServer {
return s.controlPlane
}
