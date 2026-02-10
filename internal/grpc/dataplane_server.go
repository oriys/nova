package grpc

import (
"context"
"encoding/json"
"fmt"

"github.com/oriys/nova/internal/executor"
"github.com/oriys/nova/internal/logging"
"github.com/oriys/nova/internal/pool"
"github.com/oriys/nova/internal/store"
)

// DataPlaneServer implements the data plane gRPC service
type DataPlaneServer struct {
store    *store.Store
executor *executor.Executor
pool     *pool.Pool
}

// NewDataPlaneServer creates a new data plane gRPC server
func NewDataPlaneServer(s *store.Store, exec *executor.Executor, p *pool.Pool) *DataPlaneServer {
return &DataPlaneServer{
store:    s,
executor: exec,
pool:     p,
}
}

// InvokeRequest represents a function invocation request
type InvokeRequest struct {
Function  string
Payload   []byte
TimeoutS  int32
Metadata  map[string]string
}

// InvokeResponse represents a function invocation response
type InvokeResponse struct {
RequestID  string
Output     []byte
Error      string
DurationMs int64
ColdStart  bool
}

// Invoke handles synchronous function invocation
func (s *DataPlaneServer) Invoke(ctx context.Context, req *InvokeRequest) (*InvokeResponse, error) {
if req.Function == "" {
return nil, fmt.Errorf("function name is required")
}

var payload json.RawMessage
if len(req.Payload) > 0 {
if !json.Valid(req.Payload) {
return nil, fmt.Errorf("payload must be valid JSON")
}
payload = req.Payload
} else {
payload = json.RawMessage("{}")
}

resp, err := s.executor.Invoke(ctx, req.Function, payload)
if err != nil {
return nil, fmt.Errorf("invoke failed: %w", err)
}

return &InvokeResponse{
RequestID:  resp.RequestID,
Output:     resp.Output,
Error:      resp.Error,
DurationMs: resp.DurationMs,
ColdStart:  resp.ColdStart,
}, nil
}

// HealthRequest represents a health check request
type HealthRequest struct{}

// HealthResponse represents a health check response
type HealthResponse struct {
Status     string
Components map[string]string
}

// Health returns service health status
func (s *DataPlaneServer) Health(ctx context.Context, req *HealthRequest) (*HealthResponse, error) {
components := make(map[string]string)

// Check store connectivity
if err := s.store.Ping(ctx); err != nil {
components["store"] = "unhealthy: " + err.Error()
return &HealthResponse{
Status:     "unhealthy",
Components: components,
}, nil
}
components["store"] = "healthy"

// Check pool status
if s.pool != nil {
components["pool"] = "healthy"
}

// Check executor status
if s.executor != nil {
components["executor"] = "healthy"
}

return &HealthResponse{
Status:     "healthy",
Components: components,
}, nil
}

// GetMetricsRequest represents a metrics request
type GetMetricsRequest struct {
Function     string
RangeSeconds int32
}

// GetMetricsResponse represents metrics data
type GetMetricsResponse struct {
TotalInvocations      int64
SuccessfulInvocations int64
FailedInvocations     int64
AvgDurationMs         float64
P50DurationMs         float64
P95DurationMs         float64
P99DurationMs         float64
}

// GetMetrics returns function execution metrics
func (s *DataPlaneServer) GetMetrics(ctx context.Context, req *GetMetricsRequest) (*GetMetricsResponse, error) {
// This is a simplified implementation
// In production, you would query actual metrics from the store
logging.Op().Info("GetMetrics called", "function", req.Function, "range", req.RangeSeconds)

return &GetMetricsResponse{
TotalInvocations:      0,
SuccessfulInvocations: 0,
FailedInvocations:     0,
AvgDurationMs:         0,
P50DurationMs:         0,
P95DurationMs:         0,
P99DurationMs:         0,
}, nil
}
