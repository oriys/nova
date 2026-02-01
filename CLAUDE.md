# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Nova is a minimal serverless platform that runs functions in isolated Firecracker microVMs. It supports 20+ language runtimes (Python, Go, Rust, Node.js, Ruby, Java, PHP, .NET, etc.). The host communicates with VMs over vsock using a length-prefixed JSON protocol. Lumen is the web dashboard for Nova.

## Build Commands

```bash
# Nova backend
make build          # Build nova (native) + nova-agent (linux/amd64) into bin/
make build-linux    # Cross-compile nova for linux/amd64 + nova-agent
make clean          # Remove bin/ directory
make deploy SERVER=root@your-server  # Build linux + deploy via SCP

# Docker development (recommended for local dev without KVM)
docker-compose up -d              # Start all services (postgres, redis, nova, lumen)
docker-compose up -d --build      # Rebuild and start
docker-compose logs -f lumen      # View lumen logs

# Lumen frontend (standalone)
cd lumen && npm install && npm run dev  # Dev server on localhost:3000
cd lumen && npm run build               # Production build
```

All Go binaries use `CGO_ENABLED=0`. The agent is always cross-compiled for `linux/amd64` since it runs inside VMs.

There are no tests or linting configured in this project.

## Architecture

### Three Components

1. **`cmd/nova/main.go`** - CLI and daemon. Single-file (~1800 lines) Cobra app with commands: `register`, `list`, `get`, `delete`, `update`, `invoke`, `snapshot`, `version`, `schedule`, `daemon`.

2. **`cmd/agent/main.go`** - Guest agent that runs as PID 1 inside Firecracker VMs. Listens on vsock port 9999, receives Init/Exec/Ping/Stop messages, executes user code.

3. **`lumen/`** - Next.js 15 web dashboard. Pages: Dashboard, Functions, Runtimes, Configurations, Logs, History. Uses shadcn/ui components and Recharts.

### Backend Internal Packages

- **`domain/`** - Data models: `Function`, `Runtime`, `ExecutionMode`, `ResourceLimits`, `InvokeResponse`, `FunctionVersion`, `FunctionAlias`
- **`store/`** - Postgres-backed metadata (functions/versions/aliases) with Redis for rate limiting, logs, API keys, and secrets.
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
- **`components/`** - React components (sidebar, header, dialogs, tables, charts)
- **`components/ui/`** - shadcn/ui base components
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
| .NET | `dotnet.ext4` | `/code/handler input.json` |
| Deno | `deno.ext4` | `deno run --allow-read /code/handler input.json` |
| Bun | `bun.ext4` | `bun run /code/handler input.json` |
| WASM | `wasm.ext4` | `wasmtime /code/handler -- input.json` |

### Infrastructure Requirements

Full mode (with Firecracker): Linux x86_64 with KVM (`/dev/kvm`), Firecracker binary, e2fsprogs (`mkfs.ext4`, `debugfs`).

API-only mode (Docker): Just needs Postgres and Redis. Use `docker-compose up -d` for local development.
