# Nova 后端技术文档

## 目录

- [项目概述](#项目概述)
- [功能特性](#功能特性)
- [系统架构](#系统架构)
- [技术选型](#技术选型)
- [核心模块](#核心模块)
- [关键技术点](#关键技术点)
- [API 接口](#api-接口)
- [数据库设计](#数据库设计)
- [配置说明](#配置说明)
- [部署指南](#部署指南)

---

## 项目概述

Nova 是一个轻量级 Serverless 平台，通过 Firecracker microVM（或 Docker 容器）实现函数级别的隔离执行。支持 20+ 语言运行时，宿主机与 VM 之间通过 vsock 使用长度前缀 JSON 协议通信。

系统由三个组件构成：

| 组件 | 说明 | 入口 |
|------|------|------|
| Nova Daemon | 宿主机守护进程，提供 HTTP/gRPC API | `cmd/nova/` |
| Agent | VM 内 PID 1 进程，执行用户函数 | `cmd/agent/` |
| Lumen | Next.js 15 Web 管理面板 | `lumen/` |

---

## 功能特性

### 函数管理
- 函数注册、更新、删除、查询
- 内联代码（`code` 字段）和文件路径（`code_path`）两种提交方式
- 多文件函数支持（`function_files` 表存储多个文件）
- 函数版本管理（不可变快照）与别名（alias）路由
- 流量分割 / 金丝雀发布（`TrafficSplit`）

### 多运行时
- 编译型：Go、Rust、Zig、Swift、Java、.NET、WASM
- 解释型：Python、Node.js、Ruby、PHP、Deno、Bun、Lua
- 自定义运行时：`custom` / `provided`（用户自带 bootstrap）

### 执行模式
- **Process 模式**：每次调用 fork 新进程，隔离性强
- **Persistent 模式**：长驻进程复用连接，通过 stdin/stdout JSON 通信，适合高频调用

### VM 池化
- 按函数维度池化 VM，空闲 TTL 自动回收（默认 60s）
- MinReplicas 预热，MaxReplicas 并发限制
- 实例级并发控制（`InstanceConcurrency`）
- Singleflight 去重，防止冷启动雷群效应

### 快照加速
- 对冷启动 VM 创建 Firecracker 快照（state + memory）
- 后续冷启动从快照恢复，速度提升约 50%
- 代码变更时自动使快照失效

### 安全
- JWT + API Key 双认证模式
- Secret 管理（AES-GCM 加密，`$SECRET:name` 引用注入环境变量）
- 基于令牌桶的速率限制（多 Tier 支持）

### 可观测性
- OpenTelemetry 分布式追踪（W3C Trace Context 穿透 VM 边界）
- Prometheus 指标导出（调用延迟直方图、冷启动计数等）
- 调用日志批量持久化（500ms / 100 条批次刷盘）
- 按小时时序聚合（24 桶滚动）

### 调度
- Cron 式定时调用（`@every`、`@hourly`、`@daily`）

---

## 系统架构

### 总体调用流程

```
客户端 HTTP 请求
    |
    v
API Server (:9000)
    |-- 认证中间件 (JWT / API Key)
    |-- 限流中间件 (Token Bucket)
    |-- 追踪中间件 (OpenTelemetry)
    |
    v
Executor
    |-- 加载函数元数据 + 代码 (Store)
    |-- 解析 $SECRET: 引用 (Secrets Resolver)
    |-- 获取 VM (Pool)
    |       |-- 命中暖池 -> 直接复用
    |       |-- 冷启动:
    |       |       |-- 有快照 -> 恢复快照
    |       |       |-- 无快照 -> 创建 VM
    |       |       |       |-- 分配 CID + IP
    |       |       |       |-- 创建 TAP 设备
    |       |       |       |-- 构建代码磁盘 (debugfs)
    |       |       |       |-- 启动 Firecracker 进程
    |       |       |       +-- 等待 Agent 就绪
    |       |       +-- 发送 Init 消息
    |       +-- Singleflight 去重并发请求
    |
    v
Agent (VM 内, vsock:9999)
    |-- 接收 Exec 消息
    |-- Process 模式: fork 进程执行
    |-- Persistent 模式: stdin/stdout JSON 交互
    |-- 返回 Resp 消息 (output/error/duration)
    |
    v
Executor
    |-- 记录指标 (Metrics)
    |-- 批量写入调用日志 (Log Batcher -> Postgres)
    |-- 归还 VM 到池（重置空闲计时器）
    |
    v
返回 InvokeResponse (output, error, duration_ms, cold_start)
```

### 模块依赖关系

```
cmd/nova (CLI + Daemon)
    |
    +-- api/server
    |       |-- api/controlplane  (函数 CRUD / 运行时 / 快照 / 配置)
    |       +-- api/dataplane     (调用 / 日志 / 指标 / 健康检查)
    |
    +-- executor                  (调用编排)
    |       |-- store             (元数据 + 代码)
    |       |-- pool              (VM 池)
    |       |-- secrets           (密钥解密)
    |       +-- metrics           (指标采集)
    |
    +-- pool                      (VM 生命周期管理)
    |       +-- backend           (抽象接口)
    |               |-- firecracker  (microVM 后端)
    |               +-- docker       (容器后端)
    |
    +-- config                    (配置加载)
    +-- scheduler                 (定时调度)
```

### 双磁盘 VM 架构

```
Firecracker VM
    |
    +-- /dev/vda (rootfs, 只读, 按运行时共享)
    |       例: python.ext4, node.ext4, base.ext4
    |
    +-- /dev/vdb (代码磁盘, 只读, 每 VM 独立, 16MB)
    |       /code/handler  (用户代码)
    |       /code/...      (多文件函数的附加文件)
    |
    +-- /tmp (tmpfs, 64MB, 读写)
    |
    +-- Agent (PID 1, vsock:9999)
```

---

## 技术选型

| 领域 | 技术 | 选型理由 |
|------|------|----------|
| 语言 | Go 1.22+ | 静态编译、并发原语成熟、交叉编译方便 |
| VM 隔离 | Firecracker | 亚秒级启动、极低内存开销、KVM 级隔离 |
| 容器后端 | Docker | 无 KVM 环境的降级方案，开发调试用 |
| 数据库 | PostgreSQL | JSONB 灵活存储函数配置，成熟稳定 |
| 数据库驱动 | pgx/v5 | 纯 Go 实现，连接池内建，性能优于 database/sql |
| CLI | Cobra | Go 生态标准 CLI 框架 |
| VM 通信 | vsock (AF_VSOCK) | 无需网络栈、低延迟，Firecracker 原生支持 |
| 追踪 | OpenTelemetry | 厂商中立，支持 W3C Trace Context |
| 指标 | Prometheus client_golang | 云原生监控事实标准 |
| gRPC | google.golang.org/grpc | 可选的高性能 RPC 接口 |
| HTTP 路由 | Go 1.22 `http.ServeMux` | 原生支持路径参数，无需第三方路由库 |
| 代码注入 | debugfs (e2fsprogs) | 无需 mount 即可向 ext4 镜像写入文件 |
| 去重 | singleflight | 防止同函数并发冷启动 |

---

## 核心模块

### 1. Guest Agent (`cmd/agent/`)

Agent 作为 PID 1 在 VM 内运行，负责接收宿主机指令并执行用户代码。

**文件结构：**
- `main.go` (790 行) - 主逻辑：vsock 监听、消息分发、进程执行
- `bootstraps.go` (349 行) - 内嵌的各语言 bootstrap 脚本
- `mount_linux.go` (69 行) - 磁盘挂载（devtmpfs、/code、/tmp）

**消息协议：**
- 传输层：4 字节 BigEndian 长度前缀 + JSON 载荷
- 最大消息：8MB（可配置）

| MsgType | 值 | 方向 | 说明 |
|---------|----|------|------|
| Init | 1 | Host -> VM | 初始化函数（运行时、处理器、环境变量） |
| Exec | 2 | Host -> VM | 执行调用（输入、超时、trace context） |
| Resp | 3 | VM -> Host | 返回结果（输出、错误、耗时） |
| Ping | 4 | Host -> VM | 健康检查 |
| Stop | 5 | Host -> VM | 关闭 VM |
| Reload | 6 | Host -> VM | 热更新代码 |

**Process 模式执行流程：**
1. 将输入 JSON 写入 `/tmp/input.json`
2. 执行命令：`<runtime> /code/handler /tmp/input.json`
3. 捕获 stdout/stderr
4. 从 stdout 解析 JSON 输出

**Persistent 模式执行流程：**
1. 启动长驻进程，绑定 stdin/stdout
2. 发送：`{"input": {...}, "context": {...}}\n`
3. 读取：`{"output": ...}\n` 或 `{"error": ...}\n`
4. 进程崩溃时自动重启

**注入的环境变量：**
`NOVA_REQUEST_ID`、`NOVA_FUNCTION_NAME`、`NOVA_FUNCTION_VERSION`、`NOVA_MEMORY_LIMIT_MB`、`NOVA_TIMEOUT_S`、`NOVA_RUNTIME`、`NOVA_CODE_DIR`、`NOVA_MODE`，以及运行时特定变量（`PYTHONPATH`、`NODE_PATH` 等）。

### 2. Executor (`internal/executor/`)

调用编排器，串联函数查找、代码加载、VM 获取、执行、指标记录的完整流程。

```go
type Executor struct {
    store           *store.Store
    pool            *pool.Pool
    secretsResolver *secrets.Resolver
    logBatcher      *invocationLogBatcher  // 异步批量写日志
    inflight        sync.WaitGroup
    closing         atomic.Bool
}
```

**日志批处理器配置：**
- 批次大小：100 条
- 缓冲通道：1000 条
- 刷盘间隔：500ms
- 写入超时：5s

### 3. VM Pool (`internal/pool/`)

按函数维度管理 VM 池，核心结构：

```go
Pool
├── pools sync.Map[funcID -> functionPool]
├── backend Backend (Firecracker / Docker)
└── group singleflight.Group

functionPool
├── vms []*PooledVM
├── maxReplicas atomic
├── codeHash atomic      // 检测代码变更
├── cond *sync.Cond      // 等待可用 VM
└── mu sync.Mutex
```

**后台任务：**
- 每 10s 清理超过 IdleTTL 的空闲 VM + 代码 hash 过期的 VM
- 每 30s 对空闲 VM 发送 Ping 健康检查，移除无响应实例
- Daemon 启动后周期性调用 `EnsureReady()` 预热 MinReplicas 数量的 VM

### 4. Firecracker Backend (`internal/firecracker/`)

VM 全生命周期管理，是默认的执行后端。

**VM 创建流程：**
1. 分配 CID（Context ID）和 IP（172.30.0.x/24）
2. 创建 TAP 设备，接入 `novabr0` 网桥
3. 通过 `debugfs` 构建 ext4 代码磁盘（默认 16MB，最小 4MB）
4. 启动 Firecracker 进程（Unix Socket API）
5. 轮询 VM 状态，等待启动完成（超时 10s）

**关键路径：**
- 内核：`/opt/nova/kernel/vmlinux`
- Rootfs 目录：`/opt/nova/rootfs/`（python.ext4、node.ext4、base.ext4 等）
- 快照目录：`/opt/nova/snapshots/`
- Socket 目录：`/tmp/nova/sockets/`
- 日志目录：`/tmp/nova/logs/`

**网络架构：**
- 网桥 `novabr0`，子网 `172.30.0.0/24`
- 每个 VM 分配独立 TAP 设备和 IP
- 宿主机通过 CID + port 9999 建立 vsock 连接

**VM 关闭流程：**
1. 发送 Stop 消息 → 2. 等待 Agent 响应 → 3. Kill Firecracker 进程 → 4. 清理 TAP/socket/代码磁盘 → 5. 释放 CID + IP

### 5. Docker Backend (`internal/docker/`)

Firecracker 的降级替代方案，无需 KVM，适合 macOS 开发调试。

- 通过 TCP（host:port -> container:9999）与 Agent 通信
- 代码目录挂载到容器 `/code`
- 支持 CPU/内存限制

### 6. Store (`internal/store/`)

PostgreSQL 存储层，所有元数据持久化。

**接口分组：**

| 分组 | 方法 |
|------|------|
| 函数 | SaveFunction, GetFunction, GetFunctionByName, DeleteFunction, ListFunctions, UpdateFunction |
| 版本 | PublishVersion, GetVersion, ListVersions, DeleteVersion |
| 别名 | SetAlias, GetAlias, ListAliases, DeleteAlias |
| 日志 | SaveInvocationLog, SaveInvocationLogs (批量), ListInvocationLogs, GetInvocationLog, GetFunctionTimeSeries, GetGlobalTimeSeries |
| 运行时 | SaveRuntime, GetRuntime, ListRuntimes, DeleteRuntime |
| 配置 | GetConfig, SetConfig |
| 认证 | SaveAPIKey, GetAPIKeyByHash/Name, ListAPIKeys, DeleteAPIKey |
| 密钥 | SaveSecret, GetSecret, DeleteSecret, ListSecrets, SecretExists |
| 限流 | CheckRateLimit (令牌桶) |
| 代码 | SaveFunctionCode, GetFunctionCode, UpdateFunctionCode, UpdateCompileResult, DeleteFunctionCode |
| 多文件 | SaveFunctionFiles, GetFunctionFiles, ListFunctionFiles, DeleteFunctionFiles, HasFunctionFiles |

### 7. Metrics (`internal/metrics/`)

**全局指标：** TotalInvocations, SuccessInvocations, FailedInvocations, ColdStarts, WarmStarts, TotalLatencyMs, MinLatencyMs, MaxLatencyMs, VMsCreated, VMsStopped, VMsCrashed, SnapshotsHit

**按函数维度：** 调用数、成功/失败、冷/暖启动、延迟统计

**Prometheus 导出：** 延迟直方图（可配桶）、冷启动计数器、错误计数器、活跃 VM Gauge

**时序数据：** 24 个小时桶滚动，记录每小时调用数、错误数、延迟

### 8. Config (`internal/config/`)

配置加载优先级：CLI 标志 > 环境变量（`NOVA_*` 前缀）> 配置文件（YAML/JSON）

主要配置段：

| 配置段 | 关键字段 |
|--------|----------|
| firecracker | backend, bin, kernel, rootfs_dir, snapshot_dir, bridge, subnet, boot_timeout, code_drive_size_mb, vsock_port, max_vsock_message_mb |
| docker | code_dir, agent_path, image_prefix, network, port_range, cpu_limit |
| postgres | dsn |
| pool | idle_ttl, cleanup_interval, health_check_interval, max_pre_warm_workers |
| executor | log_batch_size, log_buffer_size, log_flush_interval, log_timeout |
| daemon | http_addr, log_level |
| tracing | enabled, exporter, endpoint, service_name, sample_rate |
| metrics | enabled, namespace, histogram_buckets |
| logging | level, format (text/json), include_trace_id |
| grpc | enabled, addr (:9090) |
| auth | enabled, jwt (algorithm/secret/issuer), api_keys (enabled/static_keys), public_paths |
| rate_limit | enabled, tiers (rps/burst), default_tier |
| secrets | enabled, master_key (hex), master_key_file |

---

## 关键技术点

### 1. 代码变更检测

函数元数据保存 `CodeHash`（SHA256）。每次调用时 Executor 重新计算代码哈希，若与池中 VM 的哈希不一致，则驱逐该函数的所有 VM 并强制重建，同时使快照失效。

### 2. 热更新（解释型语言）

对于 Python、Node.js 等解释型语言，代码更新时向已有 VM 发送 `Reload` 消息（MsgType=6）：
1. Agent 将 `/code` 重新挂载为读写
2. 清空 `/code` 并写入新文件
3. 重启 persistent 进程（如适用）
4. 重新挂载为只读

避免了销毁/重建 VM 的开销。

### 3. 异步编译

编译型语言（Go、Rust、Java 等）的代码提交后由独立 goroutine 异步编译。编译状态通过 `compile_status` 字段追踪（pending -> compiling -> success/failed）。调用时若编译未完成则阻塞等待。

### 4. Singleflight 冷启动去重

同一函数的并发冷启动请求通过 `singleflight.Group` 去重——第一个请求创建 VM，后续请求复用结果，防止雷群效应。

### 5. 令牌桶限流

```
tokens += (now - last_refill) * refill_rate
tokens = min(tokens, burst_size)
if tokens >= 1: allow, tokens--
else: deny (429)
```

按 API Key 的 Tier 划分不同的 rps/burst 配额，存储在 `rate_limit_buckets` 表。

### 6. Secret 注入

函数环境变量中的 `$SECRET:name` 引用在调用时由 Secrets Resolver 解析。密钥以 AES-GCM 加密存储在 `secrets` 表，通过 Master Key 解密后注入为环境变量。

### 7. W3C Trace Context 穿透

请求的 `traceparent` 和 `tracestate` 头通过 Exec 消息传入 VM，Agent 注入为环境变量 `NOVA_TRACE_PARENT`，实现跨 VM 边界的分布式追踪链路关联。

### 8. 代码磁盘构建

使用 `debugfs`（e2fsprogs 工具集）在宿主机上直接向 ext4 镜像注入文件，无需 mount/umount 操作，避免了权限和并发问题：

```bash
# 创建空 ext4 镜像
mkfs.ext4 -F -m0 code.ext4

# 注入文件
debugfs -w code.ext4 -R "write /host/path/handler /code/handler"
```

### 9. 双磁盘架构

- **rootfs（/dev/vda）**：按运行时共享的只读系统盘，包含语言运行时和依赖
- **代码磁盘（/dev/vdb）**：每 VM 独立，仅包含用户代码，默认 16MB

这种设计将运行时环境与用户代码解耦，rootfs 可被同运行时的所有 VM 共享，显著降低磁盘开销。

---

## API 接口

所有接口默认监听 `:9000`。

### Control Plane

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/functions` | 创建函数 |
| GET | `/functions` | 列出所有函数 |
| GET | `/functions/{name}` | 获取函数详情 |
| PATCH | `/functions/{name}` | 更新函数（代码、配置、环境变量） |
| DELETE | `/functions/{name}` | 删除函数 |
| GET | `/runtimes` | 列出可用运行时 |
| GET | `/snapshots` | 列出快照 |
| POST | `/functions/{name}/snapshot` | 创建快照 |
| GET | `/config` | 获取系统配置 |
| POST | `/config` | 更新系统配置 |

### Data Plane

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/functions/{name}/invoke` | 调用函数 |
| GET | `/functions/{name}/logs` | 函数调用日志 |
| GET | `/functions/{name}/metrics` | 函数维度指标 |
| GET | `/invocations` | 全局调用日志 |
| GET | `/stats` | 池统计（活跃 VM 数、池数量） |
| GET | `/metrics` | JSON 格式全局指标 |
| GET | `/metrics/prometheus` | Prometheus 格式指标 |
| GET | `/metrics/timeseries` | 全局时序数据 |

### 健康检查（Kubernetes 兼容）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 详细状态（Postgres + 池统计） |
| GET | `/health/live` | Liveness（始终 200） |
| GET | `/health/ready` | Readiness（Postgres 连通性） |
| GET | `/health/startup` | Startup（Postgres 可达） |

### 运行时命令映射

| 运行时 | Rootfs | 执行命令 |
|--------|--------|----------|
| Go / Rust / Zig | base.ext4 | `/code/handler input.json` |
| Python | python.ext4 | `python3 /code/handler input.json` |
| Node.js | node.ext4 | `node /code/handler input.json` |
| Ruby | ruby.ext4 | `ruby /code/handler input.json` |
| Java | java.ext4 | `java -jar /code/handler input.json` |
| PHP | php.ext4 | `php /code/handler input.json` |
| .NET | dotnet.ext4 | `/code/handler input.json` |
| Deno | deno.ext4 | `deno run --allow-read /code/handler input.json` |
| Bun | bun.ext4 | `bun run /code/handler input.json` |
| WASM | wasm.ext4 | `wasmtime /code/handler -- input.json` |

---

## 数据库设计

PostgreSQL，函数配置以 JSONB 存储，便于灵活扩展字段。

```sql
-- 函数元数据
functions (
    id          UUID PRIMARY KEY,
    name        TEXT UNIQUE,
    data        JSONB           -- Function 结构体序列化
);
CREATE INDEX idx_functions_runtime ON functions ((data->>'runtime'));

-- 函数版本（不可变）
function_versions (
    function_id UUID,
    version     INT,
    data        JSONB,
    created_at  TIMESTAMPTZ,
    PRIMARY KEY (function_id, version)
);

-- 函数别名
function_aliases (
    function_id UUID,
    name        TEXT,
    data        JSONB,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    PRIMARY KEY (function_id, name)
);

-- 调用日志
invocation_logs (
    id              UUID PRIMARY KEY,
    function_id     UUID,
    function_name   TEXT,
    runtime         TEXT,
    duration_ms     BIGINT,
    cold_start      BOOLEAN,
    success         BOOLEAN,
    error_message   TEXT,
    input_size      BIGINT,
    output_size     BIGINT,
    input           JSONB,
    output          JSONB,
    stdout          TEXT,
    stderr          TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_invocation_logs_function_id ON invocation_logs (function_id);
CREATE INDEX idx_invocation_logs_created_at ON invocation_logs (created_at DESC);
CREATE INDEX idx_invocation_logs_func_time ON invocation_logs (function_id, created_at DESC);

-- 运行时定义
runtimes (
    id              TEXT PRIMARY KEY,
    name            TEXT,
    version         TEXT,
    status          TEXT,
    image_name      TEXT,
    entrypoint      TEXT[],
    file_extension  TEXT,
    env_vars        JSONB
);

-- 键值配置
config (
    key   TEXT PRIMARY KEY,
    value TEXT
);

-- API 密钥
api_keys (
    name        TEXT PRIMARY KEY,
    key_hash    TEXT UNIQUE,
    tier        TEXT,
    enabled     BOOLEAN,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ
);
CREATE INDEX idx_api_keys_hash ON api_keys (key_hash);

-- 加密密钥
secrets (
    name       TEXT PRIMARY KEY,
    value      TEXT,       -- AES-GCM 加密
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

-- 限流桶
rate_limit_buckets (
    key         TEXT PRIMARY KEY,
    tokens      DOUBLE PRECISION,
    last_refill TIMESTAMPTZ
);

-- 函数代码
function_code (
    function_id     UUID PRIMARY KEY,
    source_code     TEXT,
    compiled_binary BYTEA,
    source_hash     TEXT,
    binary_hash     TEXT,
    compile_status  TEXT,    -- pending/compiling/success/failed/not_required
    compile_error   TEXT,
    created_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ
);

-- 多文件支持
function_files (
    id          UUID PRIMARY KEY,
    function_id UUID,
    file_path   TEXT,
    content     BYTEA,
    size        BIGINT,
    created_at  TIMESTAMPTZ
);
```

---

## 配置说明

配置文件示例（YAML）：

```yaml
firecracker:
  backend: firecracker          # firecracker 或 docker
  firecracker_bin: /opt/nova/bin/firecracker
  kernel_path: /opt/nova/kernel/vmlinux
  rootfs_dir: /opt/nova/rootfs
  snapshot_dir: /opt/nova/snapshots
  socket_dir: /tmp/nova/sockets
  vsock_dir: /tmp/nova/vsock
  log_dir: /tmp/nova/logs
  bridge_name: novabr0
  subnet: 172.30.0.0/24
  boot_timeout: 10s
  code_drive_size_mb: 16
  vsock_port: 9999
  max_vsock_message_mb: 8

docker:
  code_dir: /tmp/nova/code
  image_prefix: nova-runtime-
  network: nova-net
  port_range_min: 10000
  port_range_max: 20000
  cpu_limit: 1.0

postgres:
  dsn: postgres://nova:nova@localhost:5432/nova?sslmode=disable

pool:
  idle_ttl: 60s
  cleanup_interval: 10s
  health_check_interval: 30s
  max_pre_warm_workers: 8

executor:
  log_batch_size: 100
  log_buffer_size: 1000
  log_flush_interval: 500ms
  log_timeout: 5s

daemon:
  http_addr: ":9000"
  log_level: info

tracing:
  enabled: false
  exporter: otlp
  endpoint: localhost:4317
  service_name: nova
  sample_rate: 1.0

metrics:
  enabled: true
  namespace: nova
  histogram_buckets: [5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000]

logging:
  level: info
  format: text                  # text 或 json
  include_trace_id: true

grpc:
  enabled: false
  addr: ":9090"

auth:
  enabled: false
  jwt:
    algorithm: HS256
    secret: ""
    issuer: nova
  api_keys:
    enabled: false

rate_limit:
  enabled: false
  default_tier: standard
  tiers:
    standard:
      requests_per_second: 100
      burst_size: 200
    premium:
      requests_per_second: 1000
      burst_size: 2000

secrets:
  enabled: false
  master_key: ""                # 32 字节 hex 编码
```

环境变量覆盖（`NOVA_` 前缀）：
- `NOVA_PG_DSN` - 数据库连接串
- `NOVA_HTTP_ADDR` - HTTP 监听地址
- `NOVA_LOG_LEVEL` - 日志级别
- `NOVA_IDLE_TTL` - VM 空闲超时

---

## 部署指南

### 环境要求

**完整模式（Firecracker）：**
- Linux x86_64，内核支持 KVM（`/dev/kvm`）
- Firecracker 二进制文件
- e2fsprogs（提供 `mkfs.ext4` 和 `debugfs`）
- PostgreSQL 14+
- 运行时 rootfs 镜像（python.ext4、node.ext4 等）
- Linux 内核镜像（vmlinux）

**Docker 模式：**
- Docker Engine
- PostgreSQL 14+
- 无需 KVM

### 构建

```bash
# 构建 nova (本机) + nova-agent (linux/amd64)
make build

# 交叉编译 nova + agent 为 linux/amd64
make build-linux

# 清理构建产物
make clean
```

产物输出到 `bin/` 目录。Agent 始终交叉编译为 `linux/amd64`（运行在 VM 内）。所有 Go 构建使用 `CGO_ENABLED=0` 确保静态链接。

### 生产部署

```bash
# 1. 交叉编译
make build-linux

# 2. 部署到服务器（SCP 方式）
make deploy SERVER=root@your-server

# 3. 服务器上的目录结构
/opt/nova/
├── bin/
│   ├── nova              # 主程序
│   └── nova-agent        # VM 内 Agent
├── kernel/
│   └── vmlinux           # Linux 内核
├── rootfs/
│   ├── base.ext4         # 编译型语言
│   ├── python.ext4       # Python 运行时
│   ├── node.ext4         # Node.js 运行时
│   └── ...               # 其他运行时
└── snapshots/            # VM 快照存储

# 4. 启动守护进程
nova daemon --http :9000 --pg-dsn "postgres://nova:nova@localhost:5432/nova"
```

### Docker Compose 开发环境

```bash
# 启动全部服务 (postgres + nova + lumen)
docker compose up -d

# 重新构建并启动
docker compose up -d --build

# 查看日志
docker compose logs -f nova
docker compose logs -f lumen
```

服务端口：
- Nova API：`:9000`
- Lumen Dashboard：`:3000`
- PostgreSQL：`:5432`

### 健康检查

部署后验证：

```bash
# 基础健康检查
curl http://localhost:9000/health/live

# 完整状态（含 Postgres 连通性和池统计）
curl http://localhost:9000/health

# Prometheus 指标
curl http://localhost:9000/metrics/prometheus
```

### Kubernetes 部署

健康探针配置：
- **Liveness**：`GET /health/live`（始终 200）
- **Readiness**：`GET /health/ready`（检查 Postgres 连通性）
- **Startup**：`GET /health/startup`（检查 Postgres 可达）
