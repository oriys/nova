# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Nova is a minimal serverless platform that runs functions in isolated Firecracker microVMs. It supports 20+ language runtimes (Python, Go, Rust, Node.js, Ruby, Java, PHP, etc.). The host communicates with VMs over vsock using a length-prefixed JSON protocol. Lumen is the web dashboard for Nova.

## Build Commands

Run `make` or `make help` to see all targets (interactive fzf picker if available, static list otherwise).

```bash
# Backend
make build              # Build nova (native) + agent (linux/amd64) into bin/
make build-linux        # Cross-compile nova + agent for linux/amd64
make agent              # Build only the guest agent

# Frontend (Lumen)
make frontend           # npm install + npm run build
make frontend-dev       # Dev server on localhost:3000

# Docker images
make docker-backend     # Build Nova backend Docker image
make docker-frontend    # Build Lumen frontend Docker image
make docker-runtimes    # Build all runtime Docker images
make docker-runtime-python  # Build a single runtime image

# VM rootfs
make rootfs             # Build all rootfs images via Docker
make download-assets    # Download Firecracker binary, kernel, etc.

# Full build
make all                # Backend + frontend + all Docker images

# Dev environment
make dev                # docker compose up --build (Postgres + Nova + Lumen)
make seed               # Seed sample functions

# Deploy
make deploy SERVER=root@your-server  # Cross-compile + SCP deploy

# Clean
make clean              # Remove bin/
make clean-all          # Remove bin/ + assets/ + lumen build artifacts
```

All Go binaries use `CGO_ENABLED=0`. The agent is always cross-compiled for `linux/amd64` since it runs inside VMs.

There are no tests or linting configured in this project.

## Architecture

### Five-Plane Architecture + Gateway

The backend is split into five architectural planes, each running as an independent service:

| Plane | Service | Role | Port | Protocol |
|-------|---------|------|------|----------|
| Control Plane | **nova** | Function CRUD, API keys, secrets, auth, gateway management | 9001 | HTTP REST |
| Isolation & Execution | **comet** | Executor, VM/container pool, backend lifecycle | 9090 | gRPC |
| Scheduler / Placement | **corona** | Cron scheduling, autoscaling | — | Background worker |
| Event Ingestion | **nebula** | Event bus, async queue, workflow engine | — | Background worker |
| Observability | **aurora** | SLO evaluation, Prometheus metrics, output capture | 9002 | HTTP /metrics |
| Gateway | **zenith** | Unified entry for UI/MCP/CLI traffic | 9000 | HTTP |

1. **`cmd/nova/`** - Control plane daemon. HTTP REST API for function management.
2. **`cmd/comet/`** - Isolation & execution daemon. gRPC server for function invocation via VM/container pool.
3. **`cmd/corona/`** - Scheduler/placement daemon. Runs cron scheduler and autoscaler; invokes functions via Comet gRPC.
4. **`cmd/nebula/`** - Event ingestion daemon. Runs event bus, async queue, workflow engine; invokes functions via Comet gRPC.
5. **`cmd/aurora/`** - Observability daemon. Runs SLO evaluation and exposes Prometheus metrics.
6. **`cmd/zenith/`** - Gateway. Routes UI/MCP/CLI traffic to Nova and Comet.
7. **`cmd/agent/`** - Guest agent that runs as PID 1 inside Firecracker VMs.
8. **`lumen/`** - Next.js 16 web dashboard with i18n support (en, zh-CN, zh-TW, ja, fr).

### Backend Internal Packages

- **`domain/`** - Data models: `Function`, `Runtime`, `ExecutionMode`, `ResourceLimits`, `InvokeResponse`, `FunctionVersion`, `FunctionAlias`
- **`store/`** - Postgres-backed metadata (functions/versions/aliases/logs/runtimes/config/api keys/secrets/rate limit).
- **`api/controlplane/`** - HTTP handlers for function CRUD, runtimes, snapshots
- **`api/dataplane/`** - HTTP handlers for invoke, logs, metrics, health
- **`firecracker/`** - VM lifecycle: Firecracker process management, boot/snapshot/stop, network setup (TAP devices, bridge `novabr0`, IP allocation in 172.30.0.0/24), code drive creation via `debugfs`, vsock client
- **`pool/`** - Per-function VM pools with TTL cleanup (60s default), pre-warming, singleflight deduplication
- **`executor/`** - Invocation orchestration: lookup function -> check code hash -> acquire VM -> send Exec via vsock -> record metrics -> release VM
- **`scheduler/`** - Cron-like scheduled invocations (`@every`, `@hourly`, `@daily`)
- **`config/`** - Configuration from file/env/CLI flags
- **`metrics/`** - Invocation counters, latency tracking. JSON + Prometheus export

### Frontend Structure (lumen/)

- **`app/`** - Next.js App Router pages
- **`components/`** - React components (sidebar, header, dialogs, tables, charts, language-switcher)
- **`components/ui/`** - shadcn/ui base components
- **`i18n/`** - Internationalization config (`config.ts` for shared constants, `request.ts` for server-side locale resolution)
- **`messages/`** - Translation JSON files (`en.json`, `zh-CN.json`, `zh-TW.json`, `ja.json`, `fr.json`)
- **`lib/api.ts`** - Backend API client with typed request/response
- **`lib/types.ts`** - TypeScript interfaces for domain models

### Key Design Patterns

- **Dual-disk VM architecture**: Shared read-only rootfs (per runtime) + per-VM 16MB ext4 code drive injected via `debugfs`
- **Vsock control plane**: Host-to-VM uses 4-byte BigEndian length prefix + JSON. Message types: Init(1), Exec(2), Resp(3), Ping(4), Stop(5)
- **Two execution modes**: "process" (fork per invocation) and "persistent" (long-lived process, stdin/stdout JSON)
- **Snapshot support**: VMs can be paused, snapshotted (state + memory), and restored for faster cold starts
- **Code change detection**: SHA256 hash of code file; executor detects changes and forces new VM creation
- **Inline code support**: POST /functions accepts `code` field (string) or `code_path` field (file path)

### API Endpoints

Control Plane (port 9000):
- `GET/POST /functions`, `GET/PATCH/DELETE /functions/{name}`
- `GET /runtimes`, `GET /snapshots`

Data Plane (port 9000):
- `POST /functions/{name}/invoke`
- `GET /functions/{name}/logs`, `GET /functions/{name}/metrics`
- `GET /metrics`, `GET /health`

### Runtime Execution

Functions read JSON from `argv[1]` file path, write JSON result to stdout, exit 0 on success.

| Runtime | Rootfs | Command |
|---------|--------|---------|
| Go/Rust | `base.ext4` | `/code/handler input.json` |
| Python | `python.ext4` | `python3 /code/handler input.json` |
| Node.js | `node.ext4` | `node /code/handler input.json` |
| Ruby | `ruby.ext4` | `ruby /code/handler input.json` |
| Java | `java.ext4` | `java -jar /code/handler input.json` |
| PHP | `php.ext4` | `php /code/handler input.json` |
| Deno | `deno.ext4` | `deno run --allow-read /code/handler input.json` |
| Bun | `bun.ext4` | `bun run /code/handler input.json` |
| WASM | `wasm.ext4` | `wasmtime /code/handler -- input.json` |

### Infrastructure Requirements

Full mode (with Firecracker): Linux x86_64 with KVM (`/dev/kvm`), Firecracker binary, e2fsprogs (`mkfs.ext4`, `debugfs`).

Docker mode: Useful for running the API + Lumen dashboard; microVM execution requires KVM + Firecracker + rootfs available on the host.
