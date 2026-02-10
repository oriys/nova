# Comet Project - Data Plane Extraction

## Overview

We have extracted the data plane functionality from Nova into a separate project called **Comet** (彗星). This document explains the separation, architecture, and how to use both projects together.

## What is Comet?

Comet is a standalone data plane service that handles:
- Function invocation (synchronous and asynchronous)
- VM/Container pool management
- Backend orchestration (Firecracker microVMs, Docker containers)
- Runtime metrics and observability

## Location

Comet is located at: `../comet/` (sibling directory to Nova)

## Architecture

### Separated Architecture
```
┌───────────────────┐          gRPC          ┌─────────────────┐
│      Nova         │◄──────────────────────►│     Comet       │
│  (Control Plane)  │                        │  (Data Plane)   │
│                   │                        │                 │
│ • Function CRUD   │                        │ • Executor      │
│ • Configuration   │                        │ • Pool Manager  │
│ • Tenants/Quotas  │                        │ • Backend       │
│ • Workflows       │                        │ • Invocation    │
│ • Event Bus       │                        │ • Metrics       │
└───────────────────┘                        └─────────────────┘
```

## Components Moved to Comet

### Core Components (To Be Migrated)
- `internal/executor/` - Function execution orchestration
- `internal/pool/` - VM/container pool management
- `internal/backend/` - Backend abstraction
- `internal/firecracker/` - Firecracker microVM management
- `internal/docker/` - Docker container backend
- `cmd/agent/` - Guest agent (runs in VMs)
- `internal/api/dataplane/` - Data plane HTTP API

### Support Components
- `internal/logging/` - Minimal logging implementation
- `internal/metrics/` - Execution metrics
- `api/proto/dataplane.proto` - gRPC service definition

## Components Remaining in Nova

- `internal/api/controlplane/` - Control plane HTTP API
- `internal/grpc/controlplane_server.go` - Control plane gRPC
- `internal/compiler/` - Function compilation
- `internal/service/` - Business logic services
- `internal/store/` - Metadata storage (Postgres)
- `internal/workflow/` - Workflow engine
- `internal/eventbus/` - Event bus
- `internal/asyncqueue/` - Async invocation queue
- `internal/scheduler/` - Cron scheduler
- All control plane handlers and management logic

## Deployment Modes

### Mode 1: Embedded (Default - Not Yet Implemented)

Nova runs with embedded data plane (current behavior):
```bash
nova daemon --http :8080
```

### Mode 2: Standalone (Future)

Run Comet and Nova separately:

```bash
# Terminal 1: Start Comet
cd ../comet
./comet daemon --grpc-addr :9090

# Terminal 2: Start Nova with remote Comet
cd nova
nova daemon --http :8080 --comet-endpoint localhost:9090
```

### Mode 3: Cluster (Future)

Multiple Comet workers coordinated by Nova:
```bash
# Start Comet workers
./comet daemon --grpc-addr :9090 --node-id worker1
./comet daemon --grpc-addr :9091 --node-id worker2
./comet daemon --grpc-addr :9092 --node-id worker3

# Start Nova control plane
nova daemon --comet-endpoints worker1:9090,worker2:9091,worker3:9092
```

## Current Status

### ✅ Completed
- [x] Created Comet project structure
- [x] Basic go.mod and dependencies
- [x] Minimal logging package
- [x] Main entry point with daemon command
- [x] README and architecture documentation
- [x] Makefile for building
- [x] Dockerfile for containerization
- [x] Proto definitions copied

### 🔄 In Progress
- [ ] Copy executor package with all dependencies
- [ ] Copy pool manager package
- [ ] Copy backend packages (firecracker, docker)
- [ ] Copy agent code
- [ ] Implement full daemon logic
- [ ] Set up gRPC server

### 📋 TODO
- [ ] Create gRPC client in Nova
- [ ] Implement executor proxy pattern
- [ ] Add configuration for remote mode
- [ ] Integration tests
- [ ] Migration guide
- [ ] Deployment examples

## Building Comet

```bash
cd ../comet
make build
```

## Running Comet

```bash
cd ../comet
./bin/comet version
./bin/comet help
./bin/comet daemon  # Coming soon
```

## Configuration

### Comet Config (comet.yaml)
```yaml
grpc:
  addr: ":9090"
  
backend:
  type: "firecracker"
  firecracker:
    kernel_image: "/opt/nova/kernel.bin"
    rootfs_dir: "/opt/nova/rootfs"
    
pool:
  idle_ttl: "60s"
  max_vms: 100
```

### Nova Config (nova.yaml) - Future
```yaml
comet:
  mode: "remote"          # or "embedded"
  endpoint: "localhost:9090"
  timeout: "5s"
```

## Benefits of Separation

1. **Independent Scaling**: Scale execution workers separately
2. **Resource Isolation**: Dedicated nodes for execution
3. **Clearer Boundaries**: Explicit separation of concerns
4. **Better Fault Isolation**: Control plane issues don't affect running functions
5. **Simplified Deployment**: Deploy data plane on optimized hardware

## Development Workflow

### Working on Comet
```bash
cd ../comet
make build
make test
```

### Working on Nova
```bash
cd nova
make build
make test
```

### Integration Testing
```bash
# Start Comet
cd ../comet && ./bin/comet daemon &

# Start Nova (once remote mode is implemented)
cd nova && nova daemon --comet-endpoint localhost:9090
```

## Migration Notes

- This is a foundational extraction
- Full migration requires copying all executor dependencies
- Nova will maintain backwards compatibility with embedded mode
- Remote mode will be opt-in via configuration

## References

- Comet README: `../comet/README.md`
- Comet Architecture: `../comet/ARCHITECTURE.md`
- gRPC Architecture: `GRPC_ARCHITECTURE.md`
- Nova Documentation: `README.md`

## Questions?

For questions about the extraction or architecture, see:
- `ARCHITECTURE.md` in Comet project
- `GRPC_ARCHITECTURE.md` in Nova project
