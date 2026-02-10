package grpc

import (
"context"
"fmt"
"time"

"github.com/oriys/nova/internal/compiler"
"github.com/oriys/nova/internal/domain"
"github.com/oriys/nova/internal/logging"
"github.com/oriys/nova/internal/service"
"github.com/oriys/nova/internal/store"
)

// ControlPlaneServer implements the control plane gRPC service
type ControlPlaneServer struct {
store           *store.Store
functionService *service.FunctionService
compiler        *compiler.Compiler
}

// NewControlPlaneServer creates a new control plane gRPC server
func NewControlPlaneServer(s *store.Store, fs *service.FunctionService, c *compiler.Compiler) *ControlPlaneServer {
return &ControlPlaneServer{
store:           s,
functionService: fs,
compiler:        c,
}
}

// CreateFunctionRequest represents a create function request
type CreateFunctionRequest struct {
Name      string
Runtime   string
Handler   string
Code      []byte
MemoryMB  int32
TimeoutS  int32
EnvVars   map[string]string
Mode      string
}

// FunctionInfo represents function metadata
type FunctionInfo struct {
ID          string
Name        string
Runtime     string
Handler     string
MemoryMB    int32
TimeoutS    int32
Mode        string
MinReplicas int32
Version     int32
CreatedAt   string
UpdatedAt   string
}

// CreateFunction creates a new function
func (s *ControlPlaneServer) CreateFunction(ctx context.Context, req *CreateFunctionRequest) (*FunctionInfo, error) {
if req.Name == "" {
return nil, fmt.Errorf("function name is required")
}
if req.Runtime == "" {
return nil, fmt.Errorf("runtime is required")
}

fn := &domain.Function{
Name:     req.Name,
Runtime:  domain.Runtime(req.Runtime),
Handler:  req.Handler,
MemoryMB: int(req.MemoryMB),
TimeoutS: int(req.TimeoutS),
EnvVars:  req.EnvVars,
}

if req.Mode != "" {
fn.Mode = domain.ExecutionMode(req.Mode)
}

// Save function metadata
if err := s.store.SaveFunction(ctx, fn); err != nil {
return nil, fmt.Errorf("save function: %w", err)
}

// Save function code if provided
if len(req.Code) > 0 {
codeRecord := &store.FunctionCode{
FunctionID: fn.ID,
SourceCode: string(req.Code),
UpdatedAt:  time.Now(),
}
if err := s.store.SaveFunctionCode(ctx, codeRecord); err != nil {
return nil, fmt.Errorf("save function code: %w", err)
}
}

logging.Op().Info("function created via gRPC", "name", fn.Name, "id", fn.ID)

return &FunctionInfo{
ID:          fn.ID,
Name:        fn.Name,
Runtime:     string(fn.Runtime),
Handler:     fn.Handler,
MemoryMB:    int32(fn.MemoryMB),
TimeoutS:    int32(fn.TimeoutS),
Mode:        string(fn.Mode),
MinReplicas: int32(fn.MinReplicas),
Version:     int32(fn.Version),
CreatedAt:   fn.CreatedAt.Format(time.RFC3339),
UpdatedAt:   fn.UpdatedAt.Format(time.RFC3339),
}, nil
}

// GetFunctionRequest represents a get function request
type GetFunctionRequest struct {
Name string
}

// GetFunction returns function metadata
func (s *ControlPlaneServer) GetFunction(ctx context.Context, req *GetFunctionRequest) (*FunctionInfo, error) {
if req.Name == "" {
return nil, fmt.Errorf("function name is required")
}

fn, err := s.store.GetFunctionByName(ctx, req.Name)
if err != nil {
return nil, fmt.Errorf("get function: %w", err)
}

return &FunctionInfo{
ID:          fn.ID,
Name:        fn.Name,
Runtime:     string(fn.Runtime),
Handler:     fn.Handler,
MemoryMB:    int32(fn.MemoryMB),
TimeoutS:    int32(fn.TimeoutS),
Mode:        string(fn.Mode),
MinReplicas: int32(fn.MinReplicas),
Version:     int32(fn.Version),
CreatedAt:   fn.CreatedAt.Format(time.RFC3339),
UpdatedAt:   fn.UpdatedAt.Format(time.RFC3339),
}, nil
}

// ListFunctionsRequest represents a list functions request
type ListFunctionsRequest struct {
Limit  int32
Offset int32
}

// ListFunctionsResponse represents a list of functions
type ListFunctionsResponse struct {
Functions []*FunctionInfo
Total     int32
}

// ListFunctions returns all registered functions
func (s *ControlPlaneServer) ListFunctions(ctx context.Context, req *ListFunctionsRequest) (*ListFunctionsResponse, error) {
limit := int(req.Limit)
offset := int(req.Offset)

if limit <= 0 {
limit = 100
}

functions, err := s.store.ListFunctions(ctx, limit, offset)
if err != nil {
return nil, fmt.Errorf("list functions: %w", err)
}

infos := make([]*FunctionInfo, 0, len(functions))
for _, fn := range functions {
infos = append(infos, &FunctionInfo{
ID:          fn.ID,
Name:        fn.Name,
Runtime:     string(fn.Runtime),
Handler:     fn.Handler,
MemoryMB:    int32(fn.MemoryMB),
TimeoutS:    int32(fn.TimeoutS),
Mode:        string(fn.Mode),
MinReplicas: int32(fn.MinReplicas),
Version:     int32(fn.Version),
CreatedAt:   fn.CreatedAt.Format(time.RFC3339),
UpdatedAt:   fn.UpdatedAt.Format(time.RFC3339),
})
}

return &ListFunctionsResponse{
Functions: infos,
Total:     int32(len(infos)),
}, nil
}

// DeleteFunctionRequest represents a delete function request
type DeleteFunctionRequest struct {
Name string
}

// DeleteFunctionResponse represents a delete function response
type DeleteFunctionResponse struct {
Success bool
}

// DeleteFunction deletes a function
func (s *ControlPlaneServer) DeleteFunction(ctx context.Context, req *DeleteFunctionRequest) (*DeleteFunctionResponse, error) {
if req.Name == "" {
return nil, fmt.Errorf("function name is required")
}

fn, err := s.store.GetFunctionByName(ctx, req.Name)
if err != nil {
return nil, fmt.Errorf("get function: %w", err)
}

if err := s.store.DeleteFunction(ctx, fn.ID); err != nil {
return nil, fmt.Errorf("delete function: %w", err)
}

logging.Op().Info("function deleted via gRPC", "name", req.Name, "id", fn.ID)

return &DeleteFunctionResponse{
Success: true,
}, nil
}
