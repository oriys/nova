# Nova 架构文档

> 本文档基于当前代码实现梳理 Nova 的组件与运行流程，面向运维与开发者理解系统的整体结构与关键路径。

## 1. 系统概览

Nova 是一个以 Firecracker microVM 为隔离单元的 Serverless 平台，包含控制面（函数生命周期管理）、数据面（函数调用与观测）、执行面（microVM 运行与池化）三层。核心入口包括 CLI（`cmd/nova`）、HTTP API（控制面/数据面）以及 gRPC 服务。调用执行最终由宿主机的 Executor、VM Pool 与 Firecracker Manager 协作完成，并通过 vsock 与 VM 内的 guest agent 通信执行用户代码。相关入口和组件在代码中以模块化方式组织：`internal/api`、`internal/grpc`、`internal/executor`、`internal/pool`、`internal/firecracker`、`cmd/agent` 等。【F:cmd/nova/main.go†L1-L60】【F:internal/api/server.go†L1-L106】【F:internal/grpc/server.go†L1-L70】【F:internal/executor/executor.go†L1-L54】【F:internal/pool/pool.go†L1-L83】【F:internal/firecracker/vm.go†L1-L85】【F:cmd/agent/main.go†L1-L76】

## 2. 主要组件与职责

### 2.1 CLI 与运行模式

- CLI 入口位于 `cmd/nova`，集成函数注册/调用/日志/密钥/调度等命令，并负责初始化 Store、配置、API/GRPC 服务等依赖。【F:cmd/nova/main.go†L1-L103】
- CLI 注册函数时会校验运行时与代码路径，并写入 Postgres 存储，同时计算代码哈希用于变更检测。【F:cmd/nova/main.go†L104-L190】

### 2.2 HTTP API：控制面与数据面

- HTTP 服务器由 `internal/api/server.go` 启动，注册控制面与数据面路由，并按需加载鉴权与限流中间件。【F:internal/api/server.go†L17-L101】
- 控制面路由（`internal/api/controlplane`）负责函数 CRUD、运行时管理、配置与快照管理。【F:internal/api/controlplane/handlers.go†L22-L63】
- 数据面路由（`internal/api/dataplane`）提供函数调用、健康检查、指标与日志/调用记录查询等能力。【F:internal/api/dataplane/handlers.go†L20-L55】

### 2.3 gRPC 服务

- gRPC 服务位于 `internal/grpc`，支持函数调用与元数据查询（列表、获取函数等），调用路径与 HTTP 数据面类似，最终由 Executor 执行函数。【F:internal/grpc/server.go†L1-L119】

### 2.4 执行编排：Executor

- Executor 负责从 Store 获取函数信息、解析 secrets、从 Pool 获取 VM、通过 vsock 调用 guest agent，并记录指标与调用日志。【F:internal/executor/executor.go†L33-L167】
- 调用时会创建 OpenTelemetry span，并附带 trace 信息到 vsock 请求中以实现链路追踪传播。【F:internal/executor/executor.go†L64-L120】

### 2.5 VM 管理与池化

- Firecracker Manager 负责 VM 生命周期管理（CID/IP 分配、网络桥接、socket 目录、日志目录等），并持有 VM 元信息与配置。【F:internal/firecracker/vm.go†L33-L123】
- Pool 为每个函数维护独立的 VM 池，包含清理过期 VM 的后台循环、预热（MinReplicas）与并发限制，并通过 singleflight 避免并发创建冲突。【F:internal/pool/pool.go†L38-L121】【F:internal/pool/pool.go†L130-L214】

### 2.6 Guest Agent（VM 内）

- guest agent 运行在 VM 内部，监听 vsock 连接，处理 Init/Exec/Ping/Stop 等消息，并根据运行时执行用户代码。【F:cmd/agent/main.go†L1-L117】【F:cmd/agent/main.go†L119-L190】
- 支持 process 与 persistent 两种执行模式，默认以每次调用启动新进程隔离执行。【F:cmd/agent/main.go†L38-L55】【F:cmd/agent/main.go†L144-L190】

### 2.7 存储与元数据

- Postgres 负责存储函数元数据、版本、别名、调用日志、运行时信息、配置、API Key、Secrets、限流桶等数据结构。【F:internal/store/postgres.go†L46-L132】
- Store 层将函数对象以 JSONB 形式持久化，并提供按 ID/名称读取接口。【F:internal/store/postgres.go†L134-L206】

### 2.8 观测与指标

- Metrics 模块记录调用次数、延迟、冷/热启动、VM 创建/停止等指标，并支持时序聚合与 Prometheus 适配。【F:internal/metrics/metrics.go†L10-L115】【F:internal/metrics/metrics.go†L150-L190】
- Observability 基于 OpenTelemetry，支持 OTLP HTTP 导出和采样配置，并为 executor 与 HTTP 中间件提供 trace 功能。【F:internal/observability/telemetry.go†L14-L113】

### 2.9 调度器

- Scheduler 支持简化版 Cron 语法，定期从内存任务列表触发函数调用，执行逻辑复用 Executor。【F:internal/scheduler/scheduler.go†L16-L120】【F:internal/scheduler/scheduler.go†L122-L200】

## 3. 关键流程

### 3.1 函数注册（控制面）

1. CLI 或 HTTP 控制面接收函数注册请求。
2. 校验运行时与代码路径，生成函数 ID 与代码哈希。
3. 将函数元数据写入 Postgres（functions 表）。

对应实现：CLI `register` 命令与控制面 `CreateFunction` 处理器，二者最终写入 Store。【F:cmd/nova/main.go†L104-L190】【F:internal/api/controlplane/handlers.go†L65-L174】

### 3.2 函数调用（数据面 / gRPC）

1. 数据面或 gRPC 接收调用请求，解析 payload。
2. Executor 从 Store 获取函数信息，解析 secrets。
3. Pool 获取可用 VM（冷启动或复用），建立 vsock 通道。
4. Executor 通过 vsock 调用 guest agent 执行代码。
5. 记录日志与指标，响应客户端。

对应实现：HTTP `InvokeFunction`、gRPC `Invoke`、Executor `Invoke` 以及 Pool/Firecracker 的获取/创建逻辑。【F:internal/api/dataplane/handlers.go†L57-L90】【F:internal/grpc/server.go†L63-L110】【F:internal/executor/executor.go†L56-L200】【F:internal/pool/pool.go†L38-L121】

## 4. 通信与执行协议

### 4.1 Host ↔ Guest（vsock）

- guest agent 使用 vsock 监听端口（默认 9999），接收 Init/Exec/Ping/Stop 消息。【F:cmd/agent/main.go†L26-L76】【F:cmd/agent/main.go†L97-L144】
- vsock 实现基于 `mdlayher/vsock`，封装在 `internal/pkg/vsock` 中。【F:internal/pkg/vsock/vsock.go†L1-L14】

### 4.2 Guest 执行模型

- Init 指令负责设置运行时、handler、环境变量、执行模式；Exec 指令触发执行并返回输出、错误与耗时等信息。【F:cmd/agent/main.go†L62-L94】【F:cmd/agent/main.go†L119-L190】

## 5. 资源隔离与 VM 池策略

- Firecracker Manager 为每个 VM 分配 CID、IP，并维护桥接网络配置，同时管理 socket、日志、snapshot 目录。【F:internal/firecracker/vm.go†L33-L123】
- Pool 按函数维度维护 VM 列表，默认空闲 60 秒后清理，支持 MinReplicas 预热与并发限制。【F:internal/pool/pool.go†L20-L121】【F:internal/pool/pool.go†L130-L214】

## 6. 安全与治理

- HTTP API 支持鉴权（JWT / API Key）与限流中间件，按配置启用。【F:internal/api/server.go†L35-L101】
- API Key、Secrets 与限流桶均持久化在 Postgres 表中，便于集中管理与审计。【F:internal/store/postgres.go†L90-L132】

## 7. 可观测性与健康检查

- 数据面提供健康检查接口（/health、/health/ready 等）并基于 Store/Pool 状态反馈服务可用性。【F:internal/api/dataplane/handlers.go†L92-L154】
- 指标接口提供 JSON 与 Prometheus 格式输出，供监控系统采集。【F:internal/api/dataplane/handlers.go†L32-L55】【F:internal/metrics/metrics.go†L10-L115】

## 8. 目录与模块关系参考

```
cmd/
  nova/        # CLI 与 daemon 入口
  agent/       # VM 内 guest agent
internal/
  api/         # HTTP 控制面 + 数据面
  grpc/        # gRPC 服务
  executor/    # 调用编排
  pool/        # VM 池
  firecracker/ # VM 生命周期
  store/       # Postgres 存储
  metrics/     # 指标
  observability/ # OpenTelemetry
  scheduler/   # 定时任务
```

上述目录结构与职责在代码中均有对应实现，便于从模块层面理解系统边界与依赖关系。【F:cmd/nova/main.go†L1-L60】【F:cmd/agent/main.go†L1-L76】【F:internal/api/server.go†L1-L41】【F:internal/grpc/server.go†L1-L70】【F:internal/executor/executor.go†L1-L54】【F:internal/pool/pool.go†L1-L83】【F:internal/firecracker/vm.go†L1-L85】【F:internal/store/postgres.go†L1-L28】
