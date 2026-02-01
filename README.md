# Nova

Nova 是一个极简的 Serverless 平台，基于 [Firecracker](https://github.com/firecracker-microvm/firecracker) microVM 实现函数级别的隔离执行。每次函数调用都运行在独立的轻量虚拟机中，支持 Python、Go、Rust、WASM 四种运行时。

## 它是怎么工作的

```
用户 (CLI)                    宿主机                         microVM
   |                            |                              |
   |--- nova invoke hello --->  |                              |
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

1. `nova register` 注册函数（名称、运行时、代码路径）到 Postgres（元数据），并使用 Redis 做缓存/限流/日志等
2. `nova invoke` 触发执行：从 VM 池获取或创建 microVM
3. 宿主机通过 vsock 向 VM 内的 agent 发送执行指令
4. agent 运行用户代码，返回 JSON 结果
5. VM 执行完毕后保留在池中，60 秒内可复用（warm start），超时销毁

## 架构

```
nova/
├── cmd/
│   ├── nova/main.go          # CLI 入口 (cobra)
│   └── agent/main.go         # VM 内的 guest agent（编译为 /init）
├── internal/
│   ├── domain/function.go    # 数据模型：Function, Runtime, InvokeRequest/Response
│   ├── store/postgres.go     # Postgres 存储：函数元数据/版本/别名
│   ├── store/redis.go        # Redis：日志/限流/API Keys/Secrets 等
│   ├── store/store.go        # 组合存储（Postgres + Redis）
│   ├── firecracker/vm.go     # VM 生命周期：创建、API配置、快照、停止
│   ├── pool/pool.go          # VM 池：复用、TTL清理、预热、singleflight
│   └── executor/executor.go  # 调用编排：查函数 → 获取VM → 执行 → 释放
├── scripts/
│   ├── install.sh            # Linux 服务器一键部署
│   └── deploy.sh             # macOS → Linux 交叉部署
├── examples/                 # 示例函数 (Python/Go/Rust)
├── configs/nova.yaml         # 配置模板
└── Makefile
```

### 每个运行时怎么执行

| 运行时 | rootfs 镜像 | 执行方式 | 说明 |
|---------|-------------|----------|------|
| Go | `base.ext4` (32MB) | `/code/handler input.json` | 静态编译二进制，直接执行 |
| Rust | `base.ext4` (32MB) | `/code/handler input.json` | 同 Go |
| Python | `python.ext4` (256MB) | `python3 /code/handler input.json` | 需要解释器 |
| WASM | `wasm.ext4` (256MB) | `wasmtime /code/handler -- input.json` | 需要 wasmtime |

### 双磁盘架构

每个 VM 挂载两个磁盘：

- **Drive 0 (rootfs)**: 只读，按运行时共享（`base.ext4` / `python.ext4` / `wasm.ext4`）
- **Drive 1 (code)**: 只读，16MB ext4，每个 VM 独立，包含用户函数代码

代码注入通过 `debugfs` 完成，不需要 root 权限或 mount 操作。

### 通信协议

宿主机和 VM 之间通过 [vsock](https://man7.org/linux/man-pages/man7/vsock.7.html) 通信，使用长度前缀 + JSON 的二进制协议：

```
[4 bytes: 消息长度 BigEndian] [JSON payload]
```

消息类型：

| Type | 值 | 方向 | 用途 |
|------|---|------|------|
| Init | 1 | Host → VM | 初始化函数（运行时、handler、环境变量） |
| Exec | 2 | Host → VM | 执行函数（request_id、input、timeout） |
| Resp | 3 | VM → Host | 返回结果（output、error、duration_ms） |
| Ping | 4 | Host → VM | 健康检查 |
| Stop | 5 | Host → VM | 优雅停机 |

## 环境要求

- **开发机**: macOS 或 Linux（编写代码、交叉编译）
- **运行服务器**: Linux x86_64，需要 KVM 支持（`/dev/kvm`）
- **依赖**: Postgres、Redis、Firecracker、e2fsprogs（`mkfs.ext4`、`debugfs`）

## 快速开始

Linux（KVM + Firecracker microVM 模式）：见 `docs/quickstart-linux.md`。

### 本地开发：docker-compose 启动 Postgres/Redis

```bash
docker compose up -d postgres redis

# Nova 默认读取环境变量（也可用 CLI flag --pg-dsn / --redis 覆盖）
export NOVA_PG_DSN="postgres://nova:nova@localhost:5432/nova?sslmode=disable"
export NOVA_REDIS_ADDR="localhost:6379"

# 如果本机端口已被占用，可自定义映射端口：
# NOVA_PG_PORT=5433 NOVA_REDIS_PORT=6380 docker compose up -d postgres redis
# export NOVA_PG_DSN="postgres://nova:nova@localhost:5433/nova?sslmode=disable"
# export NOVA_REDIS_ADDR="localhost:6380"
```

### 1. 准备 Linux 服务器

在 Linux 服务器上执行一键安装（安装 Firecracker、内核、rootfs、Postgres、Redis）：

```bash
# 在服务器上执行
sudo bash scripts/install.sh
```

安装完成后目录结构：

```
/opt/nova/
├── kernel/vmlinux              # Linux 内核
├── rootfs/
│   ├── base.ext4               # Go/Rust 运行时 (32MB)
│   ├── python.ext4             # Python 运行时 (256MB)
│   └── wasm.ext4               # WASM 运行时 (256MB)
└── bin/                        # 放编译好的二进制
```

### 2. 编译

```bash
# 本机编译（macOS）
make build

# 交叉编译 Linux 二进制
make build-linux
```

产物在 `bin/` 目录：

| 文件 | 说明 |
|------|------|
| `bin/nova` | macOS CLI（本地调试用） |
| `bin/nova-linux` | Linux CLI |
| `bin/nova-agent` | VM 内的 guest agent（Linux amd64，静态编译） |

### 3. 部署到服务器

```bash
# 一键部署（编译 + 传输 + 安装）
make deploy SERVER=root@your-server
```

或者手动：

```bash
scp bin/nova-linux bin/nova-agent root@server:/opt/nova/bin/
ssh root@server 'mv /opt/nova/bin/nova-linux /opt/nova/bin/nova'
```

### 4. 注册函数

```bash
# 注册一个 Python 函数
nova register hello-python \
  --runtime python \
  --handler main.handler \
  --code /path/to/hello.py \
  --memory 128 \
  --timeout 30
```

参数说明：

| 参数 | 缩写 | 默认值 | 说明 |
|------|------|--------|------|
| `--runtime` | `-r` | (必填) | 运行时：`python`、`go`、`rust`、`wasm` |
| `--code` | `-c` | (必填) | 代码文件路径 |
| `--handler` | `-H` | `main.handler` | Handler 名称 |
| `--memory` | `-m` | `128` | 内存 (MB) |
| `--timeout` | `-t` | `30` | 超时 (秒) |
| `--min-replicas` | | `0` | 最小预热 VM 数量 |
| `--env` | `-e` | | 环境变量 `KEY=VALUE`（可多次指定） |

### 5. 调用函数

```bash
# 调用函数
nova invoke hello-python --payload '{"name": "World"}'
```

输出：

```
Request ID: a1b2c3d4
Cold Start: true
Duration:   42 ms
Output:
{
  "message": "Hello, World!",
  "runtime": "python"
}
```

第二次调用会复用 VM（warm start），`Cold Start: false`，延迟更低。

### 6. 其他命令

```bash
# 列出所有函数
nova list

# 查看函数详情
nova get hello-python

# 删除函数
nova delete hello-python

# 守护进程模式（维护 VM 池、预热 min-replicas）
nova daemon --idle-ttl 60s
```

## 编写函数

函数代码遵循统一约定：从命令行参数指定的文件读取 JSON 输入，处理后将 JSON 结果输出到 stdout。

### Python

```python
import json, sys

def handler(event):
    name = event.get("name", "Anonymous")
    return {"message": f"Hello, {name}!", "runtime": "python"}

if __name__ == "__main__":
    with open(sys.argv[1]) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
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

编译为静态二进制：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o handler hello.go
```

### Rust

```rust
use serde::{Deserialize, Serialize};
use std::{env, fs};

#[derive(Deserialize)]
struct Event { name: Option<String> }

#[derive(Serialize)]
struct Response { message: String, runtime: String }

fn main() {
    let data = fs::read_to_string(&env::args().nth(1).unwrap()).unwrap();
    let event: Event = serde_json::from_str(&data).unwrap();
    let name = event.name.unwrap_or("Anonymous".into());
    let resp = Response {
        message: format!("Hello, {}!", name),
        runtime: "rust".into(),
    };
    println!("{}", serde_json::to_string(&resp).unwrap());
}
```

编译为静态二进制：

```bash
cargo build --release --target x86_64-unknown-linux-musl
```

### 函数约定

- **输入**: 从 `argv[1]` 指定的文件读取 JSON
- **输出**: JSON 格式输出到 stdout
- **退出码**: 0 表示成功，非 0 表示失败
- Go/Rust 必须编译为**静态链接**的 Linux amd64 二进制

## 全局参数

```bash
nova --pg-dsn "postgres://nova:nova@localhost:5432/nova?sslmode=disable"  # Postgres DSN
nova --redis localhost:6379      # Redis 地址（默认 localhost:6379）
nova --redis-pass secret         # Redis 密码
nova --redis-db 0                # Redis 数据库编号
```

## 配置文件

参考 `configs/nova.yaml`：

```yaml
postgres:
  dsn: "postgres://nova:nova@localhost:5432/nova?sslmode=disable"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0

firecracker:
  binary: "/usr/local/bin/firecracker"
  kernel: "/opt/nova/kernel/vmlinux"
  rootfs_dir: "/opt/nova/rootfs"
  socket_dir: "/tmp/nova/sockets"
  vsock_dir: "/tmp/nova/vsock"
  log_dir: "/tmp/nova/logs"
  boot_timeout: "10s"
  bridge_name: "novabr0"           # Network bridge
  subnet: "172.30.0.0/24"          # VM subnet

pool:
  idle_ttl: "60s"
```

## 资源限制

Nova 支持对每个函数设置资源限制：

| 限制类型 | CLI 参数 | 默认值 | 说明 |
|---------|---------|--------|------|
| **vCPU** | `--vcpus` | 1 | vCPU 数量 (1-32) |
| **内存** | `--memory` | 128 | 内存大小 (MB) |
| **执行超时** | `--timeout` | 30 | 函数执行超时 (秒) |
| **磁盘 IOPS** | `--disk-iops` | 0 (无限) | 磁盘每秒操作数 |
| **磁盘带宽** | `--disk-bandwidth` | 0 (无限) | 磁盘读写带宽 (bytes/s) |
| **网络入站** | `--net-rx-bandwidth` | 0 (无限) | 网络接收带宽 (bytes/s) |
| **网络出站** | `--net-tx-bandwidth` | 0 (无限) | 网络发送带宽 (bytes/s) |

### 使用示例

```bash
# 注册一个资源受限的计算密集型函数
nova register compute-heavy \
  --runtime go \
  --code ./handler \
  --memory 512 \
  --vcpus 2 \
  --timeout 120 \
  --disk-iops 1000 \
  --disk-bandwidth 10485760 \
  --net-rx-bandwidth 5242880 \
  --net-tx-bandwidth 5242880
```

**限制说明：**

- 带宽单位为 bytes/s（如 `10485760` = 10MB/s）
- IOPS 为每秒磁盘操作数
- 所有限制 `0` 表示不限制
- 限制基于 Firecracker rate limiter，采用令牌桶算法

## 网络架构

每个 VM 通过 TAP 设备和网桥连接到宿主机网络：

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

**特性：**

- 自动创建网桥和 TAP 设备
- 自动分配 VM IP (172.30.0.2+)
- NAT 出站流量（VM 可访问外网）
- 支持网络带宽限速（rx/tx rate limiter）
- VM 间二层隔离（需手动配置 iptables 放行跨 VM 通信）

**VM 内网络配置：**

- IP 通过内核参数自动配置 (`ip=<guest_ip>::<gateway>:255.255.255.0::eth0:off`)
- 网关：172.30.0.1
- DNS：继承宿主机 `/etc/resolv.conf`

### 在函数中访问网络

Python 示例（调用外部 API）：

```python
import json, sys, urllib.request

def handler(event):
    url = "https://api.github.com/users/octocat"
    with urllib.request.urlopen(url) as resp:
        data = json.loads(resp.read())
    return {"login": data["login"], "name": data["name"]}

if __name__ == "__main__":
    with open(sys.argv[1]) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
```

## 关键设计决策

**为什么用 Firecracker 而不是容器？**
Firecracker 提供硬件级隔离（KVM），启动速度 <125ms，内存开销 <5MB。适合多租户场景，安全性远高于容器。

**为什么用 vsock 而不是网络？**
vsock 用于宿主机↔VM 控制通道（执行指令、返回结果），延迟更低，配置简单。网络用于 VM 访问外部服务，两者互补。

**为什么用双磁盘？**
rootfs 只读共享避免了每次复制整个文件系统。代码盘 16MB 足够放任何单个函数，通过 `debugfs` 注入不需要 root 权限。

**为什么 agent 是 /init？**
Firecracker VM 不需要完整 OS。agent 直接作为 PID 1 运行，省去 systemd/init 开销，启动速度最快。

## License

MIT
