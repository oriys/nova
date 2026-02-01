# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Nova is a minimal serverless platform that runs functions in isolated Firecracker microVMs. It supports Python, Go, Rust, and WASM runtimes. The host communicates with VMs over vsock using a length-prefixed JSON protocol.

## Build Commands

```bash
make build          # Build nova (native) + nova-agent (linux/amd64) into bin/
make build-linux    # Cross-compile nova for linux/amd64 + nova-agent
make clean          # Remove bin/ directory
make deploy SERVER=root@your-server  # Build linux + deploy via SCP
```

All binaries use `CGO_ENABLED=0`. The agent is always cross-compiled for `linux/amd64` since it runs inside VMs.

There are no tests or linting configured in this project.

## Architecture

### Two Binaries

- **`cmd/nova/main.go`** - CLI and daemon. Single-file (~1800 lines) Cobra app with 12 commands: `register`, `list`, `get`, `delete`, `update`, `invoke`, `snapshot`, `version`, `schedule`, `daemon`.
- **`cmd/agent/main.go`** - Guest agent that runs as PID 1 inside Firecracker VMs. Listens on vsock port 9999, receives Init/Exec/Ping/Stop messages, executes user code.

### Internal Packages

- **`domain/`** - Data models: `Function`, `Runtime`, `ExecutionMode`, `ResourceLimits`, `InvokeResponse`, `FunctionVersion`, `FunctionAlias`
- **`store/`** - Postgres-backed metadata (functions/versions/aliases) with Redis for rate limiting, logs, API keys, and secrets.
- **`firecracker/`** - VM lifecycle (1369 lines): Firecracker process management, boot/snapshot/stop, network setup (TAP devices, bridge `novabr0`, IP allocation in 172.30.0.0/24), code drive creation via `debugfs`, vsock client with length-prefixed JSON protocol.
- **`pool/`** - Per-function VM pools with TTL cleanup (60s default), pre-warming for min-replicas, singleflight deduplication, 10-second health check pings.
- **`executor/`** - Invocation orchestration: lookup function -> check code hash for changes -> acquire VM from pool -> send Exec via vsock -> record metrics -> release VM.
- **`scheduler/`** - Cron-like scheduled invocations. Supports `@every <duration>`, `@hourly`, `@daily`, and plain durations.
- **`config/`** - Configuration from file/env/CLI flags. Covers Postgres, Redis, Firecracker paths, pool TTL, daemon settings.
- **`logging/`** - Request logging (console with emoji prefixes + JSON file output).
- **`metrics/`** - Invocation counters (total/success/failed/cold/warm), latency tracking, per-function metrics, VM lifecycle metrics. JSON + Prometheus export.
- **`pkg/vsock/`** - AF_VSOCK connection wrapper.
- **`pkg/singleflight/`** - Request deduplication.

### Key Design Patterns

- **Dual-disk VM architecture**: Shared read-only rootfs (per runtime) + per-VM 16MB ext4 code drive injected via `debugfs` (no root/mount needed).
- **Vsock control plane**: Host-to-VM communication uses 4-byte BigEndian length prefix + JSON. Message types: Init(1), Exec(2), Resp(3), Ping(4), Stop(5).
- **Two execution modes**: "process" (fork per invocation, default) and "persistent" (long-lived process, stdin/stdout JSON).
- **Snapshot support**: VMs can be paused, snapshotted (state + memory), and restored for faster cold starts.
- **Code change detection**: SHA256 hash of code file stored with function; executor detects changes and forces new VM creation.

### Runtime Execution

Functions read JSON from `argv[1]` file path, write JSON result to stdout, exit 0 on success.

| Runtime | Rootfs | Command |
|---------|--------|---------|
| Go/Rust | `base.ext4` | `/code/handler input.json` |
| Python | `python.ext4` | `python3 /code/handler input.json` |
| WASM | `wasm.ext4` | `wasmtime /code/handler -- input.json` |

### Infrastructure Requirements

Runs on Linux x86_64 with KVM (`/dev/kvm`). Requires Redis, Firecracker binary, e2fsprogs (`mkfs.ext4`, `debugfs`). Server setup: `scripts/install.sh` installs everything to `/opt/nova/`.
