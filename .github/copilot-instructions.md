# Copilot instructions for Nova

## Build, test, and lint commands

- Discover available targets: `make` or `make help`
- Build all backend services + guest agent: `make build`
- Cross-compile backend services + agent for linux/amd64: `make build-linux`
- Build Lumen frontend: `make frontend`
- Run unit tests: `make test-unit` (runs `go test -short -count=1 ./internal/...`)
- Run integration tests with local deps:
  - `make env-up`
  - `make test-integration`
  - `make env-down`
- Run a single Go test package: `go test -short -count=1 ./internal/executor`
- Run a single Go test: `go test -short -count=1 ./internal/executor -run TestPersistInvocationLog_DropsPayloadsByDefault`
- Go lint command used in CI: `go vet ./internal/...`
- Frontend lint/i18n checks:
  - `cd lumen && npm run lint`
  - `cd lumen && npm run i18n:check`
  - `cd lumen && npm run i18n:scan-hardcoded`

## High-level architecture

- Nova is a split multi-service backend behind a gateway:
  - `zenith` = HTTP gateway entrypoint for UI/MCP/CLI traffic
  - `nova` = control plane (function/resource management APIs)
  - `comet` = isolation/execution plane (gRPC invocation + pools/backends)
  - `corona` = scheduler/placement worker
  - `nebula` = event bus + async queue + workflow worker
  - `aurora` = observability plane (`/metrics`, `/health`)
  - `agent` = guest process inside VM/container that runs user code
- Request routing is split at Zenith (`internal/zenith/server.go`):
  - `POST /functions/{name}/invoke` and data-plane-only routes are sent to Comet over gRPC
  - management/control routes are reverse-proxied to Nova HTTP
- API composition lives in `internal/api/server.go`:
  - control-plane routes are registered from `internal/api/controlplane`
  - data-plane routes are registered from `internal/api/dataplane`
  - shared middleware stack handles tracing, rate limiting, auth/authz, and tenant scope
- Invocation pipeline: metadata in Postgres -> executor -> per-function pool -> backend (Firecracker default, Docker/WASM/Kubernetes/libkrun optional) -> guest agent over vsock/TCP.

## Key conventions for this repo

- If you add a data-plane HTTP endpoint, wire both sides:
  1. Register route in `internal/api/dataplane/handlers.go`
  2. Update Zenith routing in `internal/zenith/server.go` (`isCometOnlyHTTPPath` and/or function path matching) so the gateway forwards to Comet.
- `corona` and `nebula` support two invocation modes:
  - remote mode via `--comet-grpc` (preferred in split deployments)
  - local fallback mode that builds its own executor/pool when `--comet-grpc` is not set
- Config precedence is standardized: CLI flags > `NOVA_*` env vars > config file.
- Build convention: Go binaries are built with `CGO_ENABLED=0`; `cmd/agent` is always built for `linux/amd64`.
- Runtime contract conventions:
  - interpreted runtimes use `handler(event, context)` via generated bootstraps
  - compiled runtimes read JSON input from `argv[1]` and print JSON to stdout
- Lumen i18n convention:
  - keep locale registry in `lumen/i18n/config.ts`
  - keep translated strings in `lumen/messages/*.json`
  - avoid hardcoded user-facing strings; validate with `npm run i18n:check` and `npm run i18n:scan-hardcoded`
