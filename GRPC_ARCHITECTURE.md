# gRPC Architecture: Data Plane and Control Plane Separation

## Overview

Nova now supports gRPC-based communication between data plane (function execution) and control plane (management) services. This enables flexible deployment architectures and better scalability.

## Architecture

### Service Separation

```
┌──────────────────────────────────────────────────┐
│              Nova Unified Server                  │
│                                                   │
│  ┌────────────────┐      ┌─────────────────────┐ │
│  │  Data Plane    │      │  Control Plane      │ │
│  │                │      │                     │ │
│  │ • Invoke       │      │ • CreateFunction    │ │
│  │ • InvokeAsync  │      │ • GetFunction       │ │
│  │ • Health       │      │ • ListFunctions     │ │
│  │ • GetMetrics   │      │ • UpdateFunction    │ │
│  │ • ListInvocs   │      │ • DeleteFunction    │ │
│  └────────────────┘      │ • UpdateCode        │ │
│                          │ • ListRuntimes      │ │
│                          └─────────────────────┘ │
└──────────────────────────────────────────────────┘
            ▲
            │ gRPC (port 9090)
            │
    ┌───────┴───────┐
    │    Clients    │
    └───────────────┘
```

## Services

### Data Plane Service

**Purpose**: Handle function invocation and runtime operations

**RPCs**:
- `Invoke(InvokeRequest) -> InvokeResponse` - Synchronous function invocation
- `InvokeAsync(InvokeAsyncRequest) -> InvokeAsyncResponse` - Asynchronous invocation
- `Health(HealthRequest) -> HealthResponse` - Health check
- `GetMetrics(GetMetricsRequest) -> GetMetricsResponse` - Function metrics
- `ListInvocations(ListInvocationsRequest) -> ListInvocationsResponse` - Invocation history

**Key Messages**:
```protobuf
message InvokeRequest {
  string function = 1;
  bytes payload = 2;
  int32 timeout_s = 3;
  map<string, string> metadata = 4;
}

message InvokeResponse {
  string request_id = 1;
  bytes output = 2;
  string error = 3;
  int64 duration_ms = 4;
  bool cold_start = 5;
}
```

### Control Plane Service

**Purpose**: Manage functions, runtimes, and configuration

**RPCs**:
- `CreateFunction(CreateFunctionRequest) -> FunctionInfo` - Create new function
- `GetFunction(GetFunctionRequest) -> FunctionInfo` - Get function metadata
- `ListFunctions(ListFunctionsRequest) -> ListFunctionsResponse` - List all functions
- `UpdateFunction(UpdateFunctionRequest) -> FunctionInfo` - Update function config
- `DeleteFunction(DeleteFunctionRequest) -> DeleteFunctionResponse` - Delete function
- `UpdateFunctionCode(UpdateFunctionCodeRequest) -> UpdateFunctionCodeResponse` - Update code
- `ListRuntimes(ListRuntimesRequest) -> ListRuntimesResponse` - List runtimes

**Key Messages**:
```protobuf
message CreateFunctionRequest {
  string name = 1;
  string runtime = 2;
  string handler = 3;
  bytes code = 4;
  int32 memory_mb = 5;
  int32 timeout_s = 6;
  map<string, string> env_vars = 7;
}

message FunctionInfo {
  string id = 1;
  string name = 2;
  string runtime = 3;
  int32 memory_mb = 5;
  int32 timeout_s = 6;
  // ... more fields
}
```

## Configuration

### Config File (YAML)

```yaml
grpc:
  enabled: true
  addr: ":9090"
  mode: "unified"  # unified, dataplane, controlplane
```

### Environment Variables

```bash
NOVA_GRPC_ENABLED=true
NOVA_GRPC_ADDR=:9090
NOVA_GRPC_MODE=unified
```

### CLI Flags

```bash
nova daemon --grpc-enabled --grpc-addr :9090
```

## Deployment Modes

### Mode 1: Unified (Default)

Both planes run in the same process. Best for:
- Development
- Small deployments
- Single-node setups
- Backwards compatibility

```bash
nova daemon --http :8080 --grpc-enabled --grpc-addr :9090
```

### Mode 2: Separate Planes (Future)

Run data plane and control plane as separate services:

**Data Plane Worker**:
```bash
nova daemon --mode dataplane --grpc-addr :9090
```

**Control Plane**:
```bash
nova daemon --mode controlplane --grpc-addr :9091
```

Benefits:
- Scale data plane workers independently
- Control plane can coordinate multiple workers
- Better resource isolation
- Fault isolation

### Mode 3: Cluster (Future)

Multiple data plane workers coordinated by control plane:

```
┌─────────────────┐
│ Control Plane   │
│ :9091          │
└────────┬────────┘
         │
    ┌────┴──────────┬────────────┐
    │               │            │
┌───▼───┐      ┌───▼───┐   ┌───▼───┐
│Worker1│      │Worker2│   │Worker3│
│:9090  │      │:9090  │   │:9090  │
└───────┘      └───────┘   └───────┘
```

## Implementation Details

### Server Structure

```go
// Unified server managing both planes
type UnifiedServer struct {
    dataPlane    *DataPlaneServer
    controlPlane *ControlPlaneServer
    grpcServer   *grpc.Server
}

// Data plane handles invocations
type DataPlaneServer struct {
    store    *store.Store
    executor *executor.Executor
    pool     *pool.Pool
}

// Control plane handles management
type ControlPlaneServer struct {
    store           *store.Store
    functionService *service.FunctionService
    compiler        *compiler.Compiler
}
```

### Interceptors

**Logging Interceptor**:
- Logs all requests and responses
- Records duration
- Logs errors

**Error Handling Interceptor**:
- Converts Go errors to gRPC status codes
- Provides consistent error responses

## Client Usage (Future)

### Go Client

```go
// Connect to server
conn, err := grpc.Dial("localhost:9090", grpc.WithInsecure())
defer conn.Close()

// Data plane operations
dpClient := novapb.NewDataPlaneClient(conn)
resp, err := dpClient.Invoke(ctx, &novapb.InvokeRequest{
    Function: "hello-world",
    Payload:  []byte(`{"name": "Alice"}`),
})

// Control plane operations
cpClient := novapb.NewControlPlaneClient(conn)
fn, err := cpClient.CreateFunction(ctx, &novapb.CreateFunctionRequest{
    Name:     "hello-world",
    Runtime:  "python",
    Handler:  "handler",
    Code:     []byte("def handler(event): return {'message': 'hello'}"),
    MemoryMB: 128,
    TimeoutS: 30,
})
```

### CLI Client (grpcurl)

```bash
# List functions
grpcurl -plaintext localhost:9090 nova.v1.ControlPlane/ListFunctions

# Invoke function
grpcurl -plaintext -d '{"function":"hello","payload":"eyJuYW1lIjoid29ybGQifQ=="}' \
  localhost:9090 nova.v1.DataPlane/Invoke

# Health check
grpcurl -plaintext localhost:9090 nova.v1.DataPlane/Health
```

## Benefits

### 1. Service Separation
- Clear boundaries between concerns
- Independent development and deployment
- Better code organization

### 2. Scalability
- Scale data plane workers independently
- Control plane handles coordination
- Horizontal scaling support

### 3. Performance
- Binary protocol (Protocol Buffers)
- HTTP/2 multiplexing
- Efficient serialization
- Streaming support (future)

### 4. Language Agnostic
- Proto definitions work with any language
- Easy to create clients in Python, Node.js, Java, etc.
- Cross-platform compatibility

### 5. Backwards Compatible
- HTTP API still available
- gRPC is optional
- Gradual migration path

## Future Enhancements

### 1. gRPC Streaming
```protobuf
// Streaming invocation for real-time results
rpc InvokeStream(InvokeRequest) returns (stream InvokeStreamChunk);
```

### 2. Load Balancing
- Client-side load balancing
- Server-side load balancing with proxy
- Integration with service mesh (Istio, Linkerd)

### 3. Service Discovery
- Consul integration
- Kubernetes service discovery
- DNS-based discovery

### 4. Security
- TLS/mTLS support
- Authentication interceptors
- Authorization policies

### 5. Observability
- OpenTelemetry tracing
- Prometheus metrics
- Structured logging with correlation IDs

## Migration Guide

### From HTTP to gRPC

**Before** (HTTP):
```bash
curl -X POST http://localhost:8080/functions/hello/invoke \
  -H "Content-Type: application/json" \
  -d '{"name":"world"}'
```

**After** (gRPC):
```bash
grpcurl -plaintext -d '{"function":"hello","payload":"eyJuYW1lIjoid29ybGQifQ=="}' \
  localhost:9090 nova.v1.DataPlane/Invoke
```

Or use gRPC client library:
```go
resp, err := client.Invoke(ctx, &InvokeRequest{
    Function: "hello",
    Payload:  json.Marshal(map[string]string{"name": "world"}),
})
```

## Troubleshooting

### Check gRPC is Enabled
```bash
# Should show gRPC server starting
nova daemon --grpc-enabled --grpc-addr :9090 2>&1 | grep gRPC
```

### Test with grpcurl
```bash
# List services
grpcurl -plaintext localhost:9090 list

# List methods
grpcurl -plaintext localhost:9090 list nova.v1.DataPlane

# Describe service
grpcurl -plaintext localhost:9090 describe nova.v1.DataPlane
```

### Common Issues

**Connection Refused**:
- Check gRPC is enabled in config
- Verify port is not blocked
- Check server logs

**Method Not Found**:
- Verify proto definitions are up to date
- Check service registration
- Enable reflection on server

## Performance Considerations

### gRPC vs HTTP

**gRPC Advantages**:
- ~30% smaller payloads (protobuf vs JSON)
- ~2-3x lower latency for small requests
- Better throughput with HTTP/2 multiplexing
- Streaming support

**When to Use gRPC**:
- Service-to-service communication
- High-throughput workloads
- Low-latency requirements
- Streaming data

**When to Use HTTP**:
- Browser clients
- External API consumers
- Simple CRUD operations
- Debugging/testing

## References

- [gRPC Official Documentation](https://grpc.io/)
- [Protocol Buffers](https://protobuf.dev/)
- [gRPC Best Practices](https://grpc.io/docs/guides/performance/)
- Nova HTTP API Documentation
