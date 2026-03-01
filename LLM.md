# Nova LLM Sandbox 调研报告

## 一、什么是 LLM Sandbox

LLM Sandbox 是为 AI Agent/LLM 提供的**隔离代码执行环境**。核心场景：

- **AI Agent 工具调用**：LLM 生成代码并需要安全执行（数据分析、自动化脚本）
- **AI 编码助手**：生成 → 执行 → 观察输出 → 迭代修复的循环工作流
- **自主 Agent**：需要完整虚拟计算机环境（终端、文件系统、包管理）

**与传统 Serverless 的核心区别是：有状态 + 交互式**

| 维度 | 传统 Serverless (Nova 现在) | LLM Sandbox |
|------|---------------------------|-------------|
| 会话时长 | 秒级（单次调用） | 分钟到小时级（多轮交互） |
| 状态 | 无状态 | 有状态：变量、文件系统、已安装包跨调用保持 |
| 交互模式 | 单次 request/response | 多轮：执行 → 观察 → 再执行 |
| Shell 访问 | 无 | 完整 Bash/终端 |
| 包安装 | 预打包在部署时 | 运行时 `pip install`、`apt-get` |
| 网络模型 | 默认开放 | 默认限制，白名单制 |

---

## 二、竞品分析

| 平台 | 隔离技术 | 冷启动 | 开源 | 特点 |
|------|---------|--------|------|------|
| **E2B** | Firecracker microVM | ~150ms | Apache-2.0 | 金标准，~8900 star，Manus/Open R1 在用 |
| **Daytona** | Docker/Kata | <90ms | Apache-2.0 | 2026.2 融 $24M，最快冷启动 |
| **Modal** | gVisor | 亚秒级 | 否 | Python-first，Meta FAIR 在用 |
| **Together** | Firecracker | 500ms (snapshot) | 否 | 基于 CodeSandbox |
| **Cloudflare** | Containers | 快（边缘） | 是 | MCP Code Mode |
| **Google Agent Sandbox** | gVisor/Kata (K8s) | 亚秒级 (warm pool) | 是 | K8s CRD 原生 |

**E2B 是最直接的对标对象**——同样基于 Firecracker，架构理念最接近 Nova。

---

## 三、Nova 的天然优势

Nova 现有架构**高度契合** LLM Sandbox 需求：

| LLM Sandbox 需求 | Nova 现有能力 | 差距 |
|-----------------|-------------|------|
| 硬件级隔离 | Firecracker microVM | **已具备** |
| 多语言支持 | 20+ runtime | **已具备** |
| 资源限制 | ResourceLimits (CPU/Memory/IO) | **已具备** |
| 网络隔离 | NetworkPolicy + TAP/netns | **已具备** |
| 流式输出 | MsgTypeStream (type=7) | **已具备** |
| VM 快照/恢复 | Snapshot 系统 (.snap/.mem/.meta) | **已具备** |
| VM 池管理 | Pool (warm pool, LIFO, singleflight) | **已具备** |
| 多后端支持 | Backend 接口 (FC/Docker/WASM/K8s/libkrun) | **已具备** |
| 持久会话 | persistent 模式 | 需扩展 |
| Shell/终端访问 | 无 | **需新增** |
| 文件读写 API | 仅 code drive 注入 | **需新增** |
| 包安装能力 | rootfs 只读 | **需新增** |
| 多轮交互 | 单次 Exec | **需新增** |
| WebSocket 终端 | 无 | **需新增** |

---

## 四、设计方案

### 4.1 核心概念：Sandbox = 长生命周期 VM 会话

```
┌─────────────────────────────────────────────────────────┐
│  LLM / AI Agent                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │ SDK (Python / TypeScript / Go)                    │  │
│  └──────────────────────┬────────────────────────────┘  │
└─────────────────────────┼───────────────────────────────┘
                          │ REST + WebSocket
                          ▼
┌─────────────────────────────────────────────────────────┐
│  Zenith Gateway (port 9000)                             │
│  /sandboxes/*  ──────────────────────►  Sandbox API     │
│  /functions/*  ──────────────────────►  Existing API    │
└──────────────────────────┬──────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│  Sandbox Manager (新模块)                                │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Session Pool │  │ File Manager │  │ Process Mgr   │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
└──────────────────────────┬──────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│  Existing Nova Infra                                    │
│  ┌──────────┐  ┌──────────┐  ┌────────────────────┐    │
│  │ Pool     │  │ Backend  │  │ Firecracker/Docker  │    │
│  └──────────┘  └──────────┘  └────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

### 4.2 Vsock 协议扩展

在现有消息类型基础上新增 Sandbox 专用消息：

```go
const (
    // 现有消息类型 1-14 不变

    // Sandbox 扩展
    MsgTypeShellExec      = 20  // 执行 shell 命令（单次）
    MsgTypeShellStream    = 21  // 开启交互式 shell 会话
    MsgTypeShellInput     = 22  // 向 shell 会话写入 stdin
    MsgTypeShellResize    = 23  // 终端窗口大小变化
    MsgTypeFileRead       = 30  // 读取文件
    MsgTypeFileWrite      = 31  // 写入文件
    MsgTypeFileList       = 32  // 列出目录
    MsgTypeFileDelete     = 33  // 删除文件
    MsgTypeFileResp       = 34  // 文件操作响应
    MsgTypeProcessList    = 40  // 列出进程
    MsgTypeProcessKill    = 41  // 杀死进程
    MsgTypePackageInstall = 50  // 安装包 (pip/npm/apt)
)
```

### 4.3 Agent 改造

Agent 需要新增一个 **Sandbox Mode**（区别于现有的 process/persistent/durable）：

```go
// Sandbox mode: Agent 不再等待 Init+Exec，而是进入通用命令处理循环
func (a *Agent) runSandboxMode() {
    for {
        msg := readMessage(conn)
        switch msg.Type {
        case MsgTypeShellExec:
            // exec.Command("bash", "-c", cmd) — 捕获 stdout/stderr
        case MsgTypeShellStream:
            // 启动 PTY，通过 vsock 双向传输
        case MsgTypeFileRead:
            // os.ReadFile(path)
        case MsgTypeFileWrite:
            // os.WriteFile(path, data, perm)
        case MsgTypeExec:
            // 兼容现有代码执行流程
        }
    }
}
```

关键改造点：

- **可写 rootfs**：Sandbox 需要完整 Linux 环境（apt-get 需要写入 /usr），使用 overlay filesystem（只读 base + 可写 upper layer）
- **PTY 支持**：交互式终端需要伪终端，通过 vsock 双向传输
- **进程管理**：跟踪所有子进程，支持列举和 kill

### 4.4 API 设计

```
# Sandbox 生命周期
POST   /sandboxes                    # 创建 sandbox
GET    /sandboxes/{id}               # 获取状态
DELETE /sandboxes/{id}               # 销毁
POST   /sandboxes/{id}/pause         # 快照暂停
POST   /sandboxes/{id}/resume        # 从快照恢复
PATCH  /sandboxes/{id}/keepalive     # 续期

# 代码执行
POST   /sandboxes/{id}/exec          # 执行 shell 命令
POST   /sandboxes/{id}/code          # 执行代码片段 (指定语言)
WS     /sandboxes/{id}/terminal      # WebSocket 交互终端

# 文件操作
GET    /sandboxes/{id}/files?path=   # 列出/读取文件
PUT    /sandboxes/{id}/files?path=   # 写入/上传文件
DELETE /sandboxes/{id}/files?path=   # 删除文件

# 进程管理
GET    /sandboxes/{id}/processes     # 列出进程
DELETE /sandboxes/{id}/processes/{pid} # 终止进程

# 网络
POST   /sandboxes/{id}/ports         # 暴露端口（获取公网 URL）
```

创建请求示例：

```json
{
  "template": "python",
  "memory_mb": 512,
  "vcpus": 1,
  "timeout_s": 3600,
  "network_policy": "restricted",
  "env_vars": {"API_KEY": "..."},
  "on_idle_s": 300
}
```

### 4.5 Sandbox rootfs 设计

现有 rootfs（如 python.ext4）是精简的，不含包管理器。Sandbox 需要更完整的环境：

```
sandbox-python.ext4
├── /usr/bin/python3, pip3
├── /usr/bin/bash, coreutils
├── /usr/bin/apt-get (可选)
├── /usr/lib/python3/...
├── /home/sandbox/          # 工作目录
└── /init (nova-agent)
```

使用 overlay 机制：

```
lower = sandbox-python.ext4 (只读, 共享)
upper = per-sandbox ext4    (可写, 独立)
```

这样多个 sandbox 可以共享同一个 base rootfs，大幅减少存储开销。

### 4.6 WebSocket 终端实现

```
Client ──WebSocket──► Zenith ──vsock──► Agent (PTY)
  │                                        │
  ├─ stdin (key input)  ──────────────►    │
  │                                        │
  ◄─ stdout/stderr      ◄──────────────   │
  ◄─ resize events       ◄──────────────  │
```

Agent 端使用 `github.com/creack/pty` 启动 PTY，通过 vsock 转发 IO。Zenith 在 HTTP 层做 WebSocket ↔ vsock 桥接。

---

## 五、实现路径

### Phase 1：MVP（核心可用）

1. 新增 `internal/sandbox/` 包 — Sandbox Manager（会话管理、超时/空闲清理）
2. 扩展 Agent — 新增 sandbox mode，支持 ShellExec + FileRead/Write
3. 制作 `sandbox-python.ext4` — 包含 Python + pip + bash
4. 在 Zenith 添加 `/sandboxes/*` 路由
5. API：创建/销毁/执行命令/读写文件

### Phase 2：交互增强

1. WebSocket 终端（PTY over vsock）
2. 快照/恢复（复用现有 snapshot 系统）
3. 多语言 sandbox 模板（node, ubuntu, go）
4. 空闲自动暂停 + 按需恢复

### Phase 3：生态集成

1. Python/TypeScript SDK
2. MCP Server 支持（让 Claude/GPT 直接调用）
3. LangChain / LlamaIndex 集成
4. Warm pool 优化（预创建 sandbox 模板 VM）

### Phase 4：高级特性

1. 端口暴露（公网 URL 映射到 sandbox 内端口）
2. 自定义镜像支持（用户带自己的 Docker image）
3. GPU 支持（libkrun / Kata + GPU passthrough）
4. 多租户隔离与计费

---

## 六、与 E2B 的差异化定位

| 维度 | E2B | Nova Sandbox |
|------|-----|-------------|
| 定位 | 纯 sandbox 服务 | 函数平台 + sandbox 一体化 |
| 部署 | 托管为主，自建较复杂 | 自建优先，单二进制部署 |
| 函数支持 | 无（只有 sandbox） | 原生支持 serverless function |
| 调度编排 | 无 | 内置 cron (Corona) + event bus (Nebula) |
| 可观测性 | 基础 | 内置 Aurora (Prometheus/SLO) |
| 多后端 | 仅 Firecracker | FC/Docker/WASM/K8s/libkrun/Kata |

Nova 的优势在于**一站式**：同一平台既能跑 serverless function，又能提供 LLM sandbox，还有调度、事件、可观测性，适合自建 AI 基础设施的团队。
