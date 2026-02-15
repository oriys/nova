# Backend Split: Nova / Comet / Corona / Nebula / Aurora / Zenith

后端已从三服务架构升级为五平面架构（同仓库内多服务）：

## 五平面架构

| 平面 | 服务名 | 角色 | 端口 | 协议 |
|------|--------|------|------|------|
| Control Plane | **nova** | 控制平面：函数管理、配置、租户、API Key、网关路由 | 9001 | HTTP REST |
| Isolation & Execution Plane | **comet** | 隔离执行平面：执行器、池化、VM / 容器生命周期 | 9090 | gRPC |
| Scheduler / Placement Plane | **corona** | 调度放置平面：Cron 调度、自动扩缩 | — | 后台 Worker |
| Event Ingestion Plane | **nebula** | 事件摄入平面：事件总线、异步队列、工作流引擎 | — | 后台 Worker |
| Observability Plane | **aurora** | 可观测平面：SLO 评估、Prometheus 指标、输出捕获 | 9002 | HTTP /metrics |
| Gateway | **zenith** | 统一网关入口，供 UI / MCP / CLI 调用 | 9000 | HTTP |

## 目录与入口

- 控制平面：`cmd/nova`
- 隔离执行平面：`cmd/comet`
- 调度放置平面：`cmd/corona`
- 事件摄入平面：`cmd/nebula`
- 可观测平面：`cmd/aurora`
- 网关：`cmd/zenith`

## 构建

```bash
make build
```

会生成：

- `bin/nova`
- `bin/comet`
- `bin/corona`
- `bin/nebula`
- `bin/aurora`
- `bin/zenith`

## 运行

1. 启动 Nova（控制平面）

```bash
./bin/nova daemon --http :9001 --config configs/nova.json
```

2. 启动 Comet（隔离执行平面，仅 gRPC）

```bash
./bin/comet daemon --grpc :9090 --config configs/nova.json
```

3. 启动 Corona（调度放置平面）

```bash
./bin/corona daemon --config configs/nova.json --comet-grpc 127.0.0.1:9090
```

4. 启动 Nebula（事件摄入平面）

```bash
./bin/nebula daemon --config configs/nova.json --comet-grpc 127.0.0.1:9090
```

5. 启动 Aurora（可观测平面）

```bash
./bin/aurora daemon --config configs/nova.json --listen :9002
```

6. 启动 Zenith（统一入口）

```bash
./bin/zenith serve \
  --listen :9000 \
  --nova-url http://127.0.0.1:9001 \
  --comet-grpc 127.0.0.1:9090
```

## 请求路由规则（Zenith）

- `POST /functions/{name}/invoke`：走 gRPC 调用 Comet
- 数据面路径（如 `invoke-async` / logs / metrics / heatmap / invocations / async-invocations）：通过 gRPC `ProxyHTTP` 转发到 Comet 内部数据面处理器
- 其余管理类路径：反向代理到 Nova HTTP

## Docker 启动（11 服务）

`docker-compose.yml` 已配置为多服务独立容器启动（核心后端 6 服务 + Lumen + Postgres + rootfs-builder + seeder + workflow-seeder）：

- `nova` 使用 `Dockerfile` 的 `nova-runtime` target
- `comet` 使用 `Dockerfile` 的 `comet-runtime` target
- `corona` 使用 `Dockerfile` 的 `corona-runtime` target
- `nebula` 使用 `Dockerfile` 的 `nebula-runtime` target
- `aurora` 使用 `Dockerfile` 的 `aurora-runtime` target
- `zenith` 使用 `Dockerfile` 的 `zenith-runtime` target
- `lumen` 使用 `lumen/Dockerfile`

## 默认接入地址

- Lumen 默认通过 `BACKEND_URL`（compose 中已设为 `http://zenith:9000`）访问 Zenith
- Atlas 使用 `ZENITH_URL`，默认 `http://localhost:9000`
- Orbit 使用 `--server` 或 `ZENITH_URL`

## 健康检查

Zenith 提供统一健康探针：

- `GET /health/live`
- `GET /health/startup`
- `GET /health`
- `GET /health/ready`

`/health` 和 `/health/ready` 会同时检查 Nova 与 Comet 的可用性。

Aurora 提供可观测平面探针：

- `GET /health` — Aurora 自身状态
- `GET /metrics` — Prometheus 指标端点

## 远程调用模式

Corona 和 Nebula 通过 `--comet-grpc` 参数指定 Comet 的 gRPC 地址。
当指定该参数时，它们使用 `RemoteInvoker` 通过 gRPC 调用 Comet 来执行函数，
无需在本地维护执行池。如果不指定 `--comet-grpc`，则回退到本地执行器模式。

## 内部包归属

| 平面 | 核心内部包 |
|------|-----------|
| Control Plane (nova) | `api/controlplane`, `store`, `domain`, `auth`, `authz`, `secrets`, `gateway`, `compiler`, `service`, `layer`, `ai`, `config` |
| Isolation & Execution (comet) | `executor`, `pool`, `backend`, `firecracker`, `docker`, `kubernetes`, `wasm`, `libkrun` |
| Scheduler / Placement (corona) | `scheduler`, `autoscaler`, `cluster` |
| Event Ingestion (nebula) | `eventbus`, `asyncqueue`, `workflow`, `triggers` |
| Observability (aurora) | `observability`, `metrics`, `logging`, `slo`, `advisor`, `output` |
