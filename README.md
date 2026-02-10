# Nova

Nova 是一个极简的 Serverless 平台，基于 [Firecracker](https://github.com/firecracker-microvm/firecracker) microVM 实现函数级别的隔离执行。每次函数调用都运行在独立的轻量虚拟机中，支持 Python、Go、Rust、WASM、Node.js、Ruby、Java、PHP、.NET、Deno、Bun、Lua 等 20+ 运行时。

## 后端拆分

后端已拆分为三个独立服务：

- `nova`（控制平面）
- `comet`（数据平面，提供 gRPC）
- `zenith`（网关，供 UI/MCP/CLI 统一接入）

详细启动方式见 `BACKEND_SPLIT.md`。

## 目录

- [工作原理](#工作原理)
- [功能特性](#功能特性)
- [系统架构](#系统架构)
- [全组件依赖关系图](#全组件依赖关系图)
- [后端拆分](#后端拆分)
- [快速开始](#快速开始)
- [构建与部署](#构建与部署)
- [编写函数](#编写函数)
- [API 接口](#api-接口)
- [配置](#配置)
- [数据库设计](#数据库设计)
- [核心模块](#核心模块)
- [关键技术点](#关键技术点)
- [与主流平台对比](#与主流平台对比)
- [设计决策](#设计决策)

---

## 工作原理

```
用户 (CLI/HTTP)               宿主机                         microVM
   |                            |                              |
   |--- invoke hello -------->  |                              |
   |                      [1] 从 VM 池获取空闲 VM              |
   |                           (没有？创建新 VM)               |
   |                            |                              |
   |                      [2] Firecracker 启动 microVM         |
   |                            |---- vsock 连接 ----------->  |
   |                            |                        [3] agent 收到请求
   |                            |                            执行 /code/handler
   |                            |<--- 返回 JSON 结果 ------   |
   |<-- 输出结果 -----------    |                              |
   |                      [4] VM 回到池中等待复用              |
   |                           (60秒无调用则销毁)              |
```

**核心流程：**

1. 注册函数（名称、运行时、代码）到 Postgres
2. 调用触发：从 VM 池获取或创建 microVM
3. 宿主机通过 vsock 向 VM 内的 agent 发送执行指令
4. agent 运行用户代码，返回 JSON 结果
5. VM 保留在池中，60 秒内可复用（warm start），超时销毁

---

## 功能特性

### 函数管理
- 函数注册、更新、删除、查询
- 内联代码（`code` 字段）和文件路径（`code_path`）两种提交方式
- 多文件函数支持
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
- 后续冷启动从快照恢复，代码变更时自动使快照失效

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

### 资源限制

| 限制类型 | CLI 参数 | 默认值 | 说明 |
|---------|---------|--------|------|
| vCPU | `--vcpus` | 1 | vCPU 数量 (1-32) |
| 内存 | `--memory` | 128 | 内存大小 (MB) |
| 执行超时 | `--timeout` | 30 | 超时 (秒) |
| 磁盘 IOPS | `--disk-iops` | 0 (无限) | 每秒操作数 |
| 磁盘带宽 | `--disk-bandwidth` | 0 (无限) | bytes/s |
| 网络入站 | `--net-rx-bandwidth` | 0 (无限) | bytes/s |
| 网络出站 | `--net-tx-bandwidth` | 0 (无限) | bytes/s |

---

## 系统架构

系统按“接入层 -> 网关 -> 控制平面/数据平面 -> 执行后端 -> 存储”组织：

| 组件 | 说明 | 入口 |
|------|------|------|
| Zenith Gateway | 统一 HTTP 入口，转发到 Nova/Comet | `cmd/zenith/` |
| Nova Control Plane | 控制平面（函数、租户、配置、工作流、路由管理） | `cmd/nova/` |
| Comet Data Plane | 数据平面（执行、池化、异步），仅提供 gRPC | `cmd/comet/` |
| Agent | VM/容器内执行进程，负责运行用户函数 | `cmd/agent/` |
| Lumen / Atlas / Orbit | UI / MCP / CLI 调用方，统一访问 Zenith | `lumen/` / `atlas/` / `orbit/` |

### 全组件依赖关系图

```mermaid
flowchart LR
    subgraph "Clients"
        lumen["Lumen (UI)"]
        atlas["Atlas (MCP)"]
        orbit["Orbit (CLI)"]
        ext["External HTTP Clients"]
    end

    subgraph "Gateway Layer"
        zenith["Zenith (HTTP Gateway)"]
    end

    subgraph "Control Plane"
        nova["Nova (HTTP Control Plane)"]
        scheduler["Scheduler"]
        workflow["Workflow Engine"]
        compiler["Compiler / Function Service"]
    end

    subgraph "Data Plane"
        comet["Comet (gRPC Data Plane)"]
        executor["Executor"]
        pool["Worker / VM Pool"]
        asyncq["Async Queue Workers"]
        eventbus["Event Bus Workers"]
    end

    subgraph "Runtime Backends"
        fc["Firecracker Backend"]
        docker["Docker Backend"]
        agent["nova-agent (in VM/container)"]
        assets["Rootfs / Code / Snapshot Assets"]
    end

    subgraph "State / Infra"
        pg["PostgreSQL"]
        obs["Metrics / Tracing / Logs"]
    end

    lumen --> zenith
    atlas --> zenith
    orbit --> zenith
    ext --> zenith

    zenith -->|"HTTP management routes"| nova
    zenith -->|"gRPC invoke/proxy routes"| comet

    nova --> scheduler
    nova --> workflow
    nova --> compiler
    nova --> pg
    nova --> obs

    comet --> executor
    comet --> asyncq
    comet --> eventbus
    comet --> pg
    comet --> obs

    executor --> pool
    executor --> pg
    pool --> fc
    pool --> docker
    fc --> agent
    docker --> agent
    fc --> assets
    docker --> assets
```

### 调用流程

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

### 模块依赖

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

每个 VM 挂载两个磁盘：

- **Drive 0 (rootfs)**: 只读，按运行时共享（`base.ext4` / `python.ext4` / `node.ext4` 等）
- **Drive 1 (code)**: 只读，16MB ext4，每个 VM 独立，包含用户函数代码

代码注入通过 `debugfs` 完成，不需要 root 权限或 mount 操作。

```
Firecracker VM
    +-- /dev/vda (rootfs, 只读, 按运行时共享)
    +-- /dev/vdb (代码磁盘, 只读, 每 VM 独立, 16MB)
    |       /code/handler  (用户代码)
    +-- /tmp (tmpfs, 64MB, 读写)
    +-- Agent (PID 1, vsock:9999)
```

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

### 通信协议

宿主机和 VM 之间通过 [vsock](https://man7.org/linux/man-pages/man7/vsock.7.html) 通信，使用长度前缀 + JSON 的二进制协议：

```
[4 bytes: 消息长度 BigEndian] [JSON payload]
```

| Type | 值 | 方向 | 用途 |
|------|---|------|------|
| Init | 1 | Host → VM | 初始化函数（运行时、handler、环境变量） |
| Exec | 2 | Host → VM | 执行函数（request_id、input、timeout） |
| Resp | 3 | VM → Host | 返回结果（output、error、duration_ms） |
| Ping | 4 | Host → VM | 健康检查 |
| Stop | 5 | Host → VM | 优雅停机 |
| Reload | 6 | Host → VM | 热更新代码 |

### 网络架构

```
                   宿主机
┌──────────────────────────────────────────────┐
│  novabr0 (172.30.0.1/24)                     │
│    │                                          │
│    ├─ nova-abc123 (TAP) ← VM1 (172.30.0.2)  │
│    ├─ nova-def456 (TAP) ← VM2 (172.30.0.3)  │
│    └─ nova-ghi789 (TAP) ← VM3 (172.30.0.4)  │
│                                               │
│  iptables NAT (MASQUERADE) → Internet        │
└──────────────────────────────────────────────┘
```

- 自动创建网桥和 TAP 设备，自动分配 VM IP
- NAT 出站流量（VM 可访问外网）
- IP 通过内核参数自动配置，网关 172.30.0.1

### 技术选型

| 领域 | 技术 | 选型理由 |
|------|------|----------|
| 语言 | Go 1.22+ | 静态编译、并发原语成熟、交叉编译方便 |
| VM 隔离 | Firecracker | 亚秒级启动、极低内存开销、KVM 级隔离 |
| 容器后端 | Docker | 无 KVM 环境的降级方案，开发调试用 |
| 数据库 | PostgreSQL | JSONB 灵活存储函数配置，成熟稳定 |
| 数据库驱动 | pgx/v5 | 纯 Go 实现，连接池内建 |
| CLI | Cobra | Go 生态标准 CLI 框架 |
| VM 通信 | vsock (AF_VSOCK) | 无需网络栈、低延迟，Firecracker 原生支持 |
| 追踪 | OpenTelemetry | 厂商中立，支持 W3C Trace Context |
| 指标 | Prometheus client_golang | 云原生监控事实标准 |
| HTTP 路由 | Go 1.22 `http.ServeMux` | 原生支持路径参数，无需第三方路由库 |
| 代码注入 | debugfs (e2fsprogs) | 无需 mount 即可向 ext4 镜像写入文件 |

---

## 快速开始

### 环境要求

**完整模式（Firecracker）：**
- Linux x86_64，内核支持 KVM（`/dev/kvm`）
- Firecracker 二进制、Linux 内核镜像（vmlinux）
- e2fsprogs（`mkfs.ext4`、`debugfs`）
- PostgreSQL 14+
- 运行时 rootfs 镜像

**Docker 模式（无需 KVM，适合本地开发）：**
- Docker Engine + PostgreSQL 14+

### 本地开发

```bash
make dev          # docker compose up --build（Postgres + Nova + Comet + Zenith + Lumen）
make seed         # 注入示例函数
```

服务端口：Zenith API `:9000`、Nova 控制平面 `:9001`、Comet gRPC `:9090`、Lumen Dashboard `:3000`、PostgreSQL `:5432`

> macOS/无 KVM 环境下主要用于跑 API + Lumen Dashboard，不适合跑 Firecracker VM 执行。

### Linux 服务器部署

```bash
# 1. 构建 linux 二进制
make build-linux

# 2. 一键部署（安装依赖、初始化 DB、配置 systemd 并启动服务）
sudo bash scripts/setup.sh
```

安装完成后目录结构：

```
/opt/nova/
├── bin/nova, comet, zenith, nova-agent
├── kernel/vmlinux
├── rootfs/
│   ├── base.ext4          # Go/Rust/Zig
│   ├── python.ext4        # Python
│   ├── node.ext4          # Node.js
│   └── ...                # ruby/java/php/dotnet/deno/bun/wasm
└── snapshots/
```

### 使用

```bash
# 注册函数
nova register hello-python \
  --runtime python \
  --handler main.handler \
  --code /path/to/hello.py \
  --memory 128 \
  --timeout 30

# 调用函数
nova invoke hello-python --payload '{"name": "World"}'

# 其他命令
nova list
nova get hello-python
nova delete hello-python
nova daemon --idle-ttl 60s
```

---

## 构建与部署

运行 `make` 或 `make help` 查看所有可用命令（安装了 [fzf](https://github.com/junegunn/fzf) 时会启动交互式选择器）。

### 后端

```bash
make build          # 构建 nova/comet/zenith (本机) + agent (linux/amd64)
make build-linux    # 交叉编译 nova/comet/zenith + agent 全部为 linux/amd64
make agent          # 仅构建 guest agent
```

### 前端（Lumen Dashboard）

```bash
make frontend       # npm install + npm run build
make frontend-dev   # 启动开发服务器 (localhost:3000)
```

### Docker 镜像

```bash
make docker-backend          # 构建 Nova 后端镜像
make docker-frontend         # 构建 Lumen 前端镜像
make docker-runtimes         # 构建全部运行时镜像
make docker-runtime-python   # 构建单个运行时镜像
```

### VM Rootfs

```bash
make rootfs             # Docker 内构建全部 rootfs 镜像
make download-assets    # 下载 Firecracker 二进制、内核等大文件
```

### 全量构建与清理

```bash
make all        # 后端 + 前端 + 全部 Docker 镜像
make clean      # 清理 bin/
make clean-all  # 清理 bin/ + assets/ + lumen 构建产物
```

### 部署

```bash
make deploy SERVER=root@your-server    # 交叉编译 + SCP 传输
```

产物在 `bin/` 目录。所有 Go 构建使用 `CGO_ENABLED=0` 确保静态链接。Agent 始终交叉编译为 `linux/amd64`。

---

## 编写函数

函数代码遵循统一约定：`handler(event, context)` 签名，与 AWS Lambda 兼容。context 是类实例（Python/Ruby）或对象（Node/Deno/Bun），包含 `function_name`、`request_id`、`memory_limit_in_mb`、`get_remaining_time_in_millis()` 等字段。

### Python

```python
def handler(event, context):
    name = event.get("name", "Anonymous")
    return {
        "message": f"Hello, {name}!",
        "runtime": "python",
        "request_id": context.request_id,
    }
```

### Go

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

type Event struct {
    Name string `json:"name"`
}

func main() {
    data, _ := os.ReadFile(os.Args[1])
    var event Event
    json.Unmarshal(data, &event)

    if event.Name == "" {
        event.Name = "Anonymous"
    }
    result, _ := json.Marshal(map[string]string{
        "message": fmt.Sprintf("Hello, %s!", event.Name),
        "runtime": "go",
    })
    fmt.Println(string(result))
}
```

### Node.js

```javascript
function handler(event, context) {
  const name = event.name || "Anonymous";
  return {
    message: `Hello, ${name}!`,
    runtime: "node",
    requestId: context.requestId,
  };
}

module.exports = { handler };
```

编译：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o handler hello.go`

### Rust

```rust
use serde::{Deserialize, Serialize};
use std::{env, fs};

#[derive(Deserialize)]
struct Event { name: Option<String> }

#[derive(Serialize)]
struct Response { message: String, runtime: String }

fn main() {
    let args: Vec<String> = env::args().collect();
    let data = fs::read_to_string(&args[1]).unwrap();
    let event: Event = serde_json::from_str(&data).unwrap();
    let name = event.name.unwrap_or("Anonymous".into());
    let resp = Response {
        message: format!("Hello, {}!", name),
        runtime: "rust".into(),
    };
    println!("{}", serde_json::to_string(&resp).unwrap());
}
```

编译：`cargo build --release --target x86_64-unknown-linux-musl`

> - 解释型语言（Python/Node/Ruby/PHP/Deno/Bun）：只需定义 `handler(event, context)` 函数，bootstrap 自动包装
> - 编译型语言（Go/Rust）：读取 `argv[1]` 文件，输出 JSON 到 stdout，须编译为**静态链接**的 Linux amd64 二进制

---

## API 接口

所有接口默认监听 `:9000`。

### Control Plane

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/functions` | 创建函数 |
| GET | `/functions` | 列出所有函数 |
| GET | `/functions/{name}` | 获取函数详情 |
| PATCH | `/functions/{name}` | 更新函数 |
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
| GET | `/stats` | 池统计 |
| GET | `/metrics` | JSON 格式全局指标 |
| GET | `/metrics/prometheus` | Prometheus 格式指标 |
| GET | `/metrics/timeseries` | 全局时序数据 |

### 健康检查

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 详细状态（Postgres + 池统计） |
| GET | `/health/live` | Liveness（始终 200） |
| GET | `/health/ready` | Readiness（Postgres 连通性） |
| GET | `/health/startup` | Startup（Postgres 可达） |

---

## 配置

配置加载优先级：CLI 标志 > 环境变量（`NOVA_*` 前缀）> 配置文件（YAML/JSON）

**环境变量：** `NOVA_PG_DSN`、`NOVA_HTTP_ADDR`、`NOVA_LOG_LEVEL`、`NOVA_IDLE_TTL` 等。

最小 JSON 配置：

```json
{
  "postgres": { "dsn": "postgres://nova:nova@localhost:5432/nova?sslmode=disable" },
  "daemon": { "http_addr": ":9000", "log_level": "info" },
  "pool": { "idle_ttl": "60s" }
}
```

<details>
<summary>完整配置示例（YAML）</summary>

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

</details>

---

## 数据库设计

PostgreSQL，函数配置以 JSONB 存储。

<details>
<summary>完整表结构</summary>

```sql
-- 函数元数据
functions (
    id          UUID PRIMARY KEY,
    name        TEXT UNIQUE,
    data        JSONB
);

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
config (key TEXT PRIMARY KEY, value TEXT);

-- API 密钥
api_keys (
    name        TEXT PRIMARY KEY,
    key_hash    TEXT UNIQUE,
    tier        TEXT,
    enabled     BOOLEAN,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ
);

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

</details>

---

## 核心模块

### Guest Agent (`cmd/agent/`)

Agent 作为 PID 1 在 VM 内运行，负责接收宿主机指令并执行用户代码。

- **Process 模式**：将输入写入 `/tmp/input.json` → 执行 `<runtime> /code/handler /tmp/input.json` → 捕获 stdout → 解析 JSON
- **Persistent 模式**：启动长驻进程 → 通过 stdin/stdout JSON 交互 → 崩溃时自动重启

注入的环境变量：`NOVA_REQUEST_ID`、`NOVA_FUNCTION_NAME`、`NOVA_FUNCTION_VERSION`、`NOVA_MEMORY_LIMIT_MB`、`NOVA_TIMEOUT_S`、`NOVA_RUNTIME`、`NOVA_CODE_DIR`、`NOVA_MODE`，以及运行时特定变量（`PYTHONPATH`、`NODE_PATH` 等）。

### Executor (`internal/executor/`)

调用编排器，串联函数查找、代码加载、VM 获取、执行、指标记录的完整流程。异步日志批处理器（100 条/批、500ms 刷盘间隔）。

### VM Pool (`internal/pool/`)

按函数维度管理 VM 池。后台任务：
- 每 10s 清理超过 IdleTTL 的空闲 VM + 代码 hash 过期的 VM
- 每 30s 对空闲 VM 发送 Ping 健康检查，移除无响应实例
- Daemon 启动后周期性预热 MinReplicas 数量的 VM

### Firecracker Backend (`internal/firecracker/`)

VM 全生命周期管理（默认后端）：分配 CID + IP → 创建 TAP 设备 → `debugfs` 构建代码磁盘 → 启动 Firecracker → 等待就绪。

### Docker Backend (`internal/docker/`)

Firecracker 的降级替代方案，无需 KVM，适合 macOS 开发调试。通过 TCP 与 Agent 通信，代码目录挂载到容器 `/code`。

### Store (`internal/store/`)

PostgreSQL 存储层：函数 CRUD、版本管理、别名、调用日志、运行时、配置、API Key、Secret、限流、代码存储、多文件支持。

### Metrics (`internal/metrics/`)

全局指标（调用数、冷/暖启动、延迟、VM 生命周期）+ 按函数维度指标。Prometheus 导出 + 24 小时时序聚合。

---

## 关键技术点

### 代码变更检测
函数元数据保存 `CodeHash`（SHA256）。每次调用时重新计算，若不一致则驱逐所有 VM 并使快照失效。

### 热更新（解释型语言）
代码更新时向已有 VM 发送 `Reload` 消息：Agent 将 `/code` 重新挂载为读写 → 清空并写入新文件 → 重启 persistent 进程 → 重新挂载只读。

### 异步编译
编译型语言的代码提交后由独立 goroutine 异步编译。编译状态：`pending → compiling → success/failed`。调用时若编译未完成则阻塞等待。

### Singleflight 冷启动去重
同一函数的并发冷启动请求通过 `singleflight.Group` 去重，防止雷群效应。

### Secret 注入
环境变量中的 `$SECRET:name` 引用在调用时由 Secrets Resolver 解析，密钥以 AES-GCM 加密存储，通过 Master Key 解密注入。

### W3C Trace Context 穿透
`traceparent` / `tracestate` 通过 Exec 消息传入 VM，Agent 注入为 `NOVA_TRACE_PARENT` 环境变量，实现跨 VM 边界的追踪链路关联。

---

## 与主流平台对比

Nova 是一个面向学习和内部使用的轻量 Serverless 平台。以下是与 AWS Lambda、Cloudflare Workers、Google Cloud Functions 的能力对比。

### 能力矩阵

| 能力 | Nova | AWS Lambda | CF Workers | GCP Cloud Functions |
|------|------|------------|------------|---------------------|
| **VM 级隔离** | Firecracker KVM | Firecracker KVM | V8 Isolate | gVisor |
| **多运行时** | 20+ 语言 | 7 语言 + 自定义 | JS/TS/WASM | 7 语言 |
| **冷启动优化** | 快照恢复 + 模板缓存 | SnapStart (Java) | 无冷启动 (V8) | Min instances |
| **可观测性** | OTEL + Prometheus + 结构化日志 | CloudWatch + X-Ray | Tail Workers + Logpush | Cloud Logging + Trace |
| **版本管理** | 不可变版本 + 别名 + 流量分割 | 版本 + 别名 + 加权路由 | Gradual Rollouts | Traffic splitting |
| **认证** | JWT + API Key + 速率限制 | IAM + 资源策略 | API tokens | IAM + Invoker 角色 |
| **调度** | Cron 定时触发 | EventBridge 规则 | Cron Triggers | Cloud Scheduler |
| **热更新** | 解释型语言原地 Reload | 需重新部署 | 即时生效 | 需重新部署 |
| 事件源 | HTTP + Cron | 30+ 事件源 | HTTP + Queue + CRON + Email | 10+ 事件源 |
| 流式响应 | — | Response Streaming | 原生 ReadableStream | — |
| 错误处理 | 基础日志 | DLQ + 重试 + 目的地路由 | Tail Workers | DLQ + 重试 |
| 自动伸缩 | 手动 Min/Max | 自适应 + 预留并发 | 自动（无限制） | 自适应 + Min instances |
| 持久存储 | 64MB tmpfs（易失） | EFS + S3 + DynamoDB | KV + R2 + D1 + Durable Objects | GCS + Firestore |
| 依赖层 | — | Lambda Layers | 内置 (npm) | — |
| 网络隔离 | 单子网 + NAT | VPC + 安全组 + PrivateLink | — | VPC Connector |
| 边缘部署 | — | Lambda@Edge | 全球 300+ 节点 | — |
| 细粒度权限 | 全局 API Key | 函数级 IAM 角色 | 服务绑定 | 函数级 SA |
| API 网关 | 内置基础 HTTP | API Gateway (REST/HTTP/WS) | 内置 (路由/域名) | API Gateway |
| 容器镜像部署 | Docker 后端（降级方案） | 容器镜像函数 | — | Cloud Run 集成 |

> 粗体行 = Nova 已具备的能力，普通行 = Nova 缺失或薄弱的能力。

### Nova 的核心优势

1. **极致轻量**：整个后端是单二进制 + Postgres，没有分布式依赖
2. **冷启动快**：Firecracker 快照恢复 + 代码磁盘模板缓存 + singleflight 去重
3. **运行时最广**：20+ 语言支持，自定义运行时，handler-only 风格零样板代码
4. **可观测性好**：OpenTelemetry trace 穿透 VM 边界 + Prometheus 指标 + 结构化日志
5. **热更新**：解释型语言代码更新不销毁 VM，原地 Reload

### 待补齐的关键能力

按优先级排序：

**P0 — 生产环境底线**

| 能力 | 说明 | 参考 |
|------|------|------|
| 异步调用 + 事件总线 | 函数可异步触发其他函数，内部消息队列 | Lambda Async Invoke, EventBridge |
| DLQ + 自动重试 | 失败自动重试（指数退避），超限进死信队列 | Lambda DLQ, SQS redrive |
| 响应流式传输 | vsock 协议扩展支持 chunked response | Lambda Response Streaming |

**P1 — 规模化必需**

| 能力 | 说明 | 参考 |
|------|------|------|
| 自动伸缩策略 | 基于请求速率/延迟自适应调整 VM 池大小 | Lambda Provisioned Concurrency |
| 共享依赖层 | 跨函数共享运行时依赖，避免重复打包 | Lambda Layers |
| 持久化存储 | 函数可读写的持久卷或对象存储 | EFS, S3, CF KV/R2 |

**P2 — 企业级特性**

| 能力 | 说明 | 参考 |
|------|------|------|
| 网络隔离 | 安全组、多子网、出入站规则 | VPC, Security Groups |
| 函数级权限 | 每个函数独立角色，最小权限原则 | IAM Role per function |
| API 网关增强 | 自定义域名、WebSocket、请求校验 | API Gateway |
| 边缘部署 | 多区域 + 就近路由 | Lambda@Edge, CF Workers |

---

## 设计决策

**为什么用 Firecracker 而不是容器？**
硬件级隔离（KVM），启动速度 <125ms，内存开销 <5MB。适合多租户场景。

**为什么用 vsock 而不是网络？**
用于宿主机↔VM 控制通道，延迟更低，配置简单。网络用于 VM 访问外部服务，两者互补。

**为什么用双磁盘？**
rootfs 只读共享避免了每次复制文件系统。代码盘 16MB 通过 `debugfs` 注入，不需要 root 权限。

**为什么 agent 是 /init？**
Firecracker VM 不需要完整 OS。agent 直接作为 PID 1 运行，省去 init 开销。

---

## License

MIT
