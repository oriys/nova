# Nova Agent 架构详解

## 概述

Nova Agent（`cmd/agent/main.go`）是运行在每个 Firecracker microVM 内部的守护进程。它作为宿主机与用户函数之间的桥梁，负责接收执行请求、运行用户代码、返回执行结果。

## 系统架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Host (宿主机)                                   │
│                                                                             │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────────────────────┐   │
│  │   CLI       │     │   Pool      │     │   Firecracker Manager       │   │
│  │ nova invoke │────►│   (VM池)    │────►│   (VM 生命周期管理)          │   │
│  └─────────────┘     └─────────────┘     └─────────────────────────────┘   │
│         │                                              │                    │
│         │                                              │ 创建 VM            │
│         │                                              ▼                    │
│         │            ┌──────────────────────────────────────────────────┐  │
│         │            │                    microVM                        │  │
│         │            │  ┌────────────────────────────────────────────┐  │  │
│         │   vsock    │  │              Nova Agent (PID 1)            │  │  │
│         └───────────►│  │                                            │  │  │
│              :9999   │  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  │  │  │
│                      │  │  │ 消息处理  │  │ 进程管理  │  │ 运行时   │  │  │  │
│                      │  │  │ Handler  │  │ Process  │  │ Runtime  │  │  │  │
│                      │  │  └──────────┘  └──────────┘  └──────────┘  │  │  │
│                      │  │                     │                      │  │  │
│                      │  │                     ▼                      │  │  │
│                      │  │  ┌────────────────────────────────────┐   │  │  │
│                      │  │  │         用户函数进程                 │   │  │  │
│                      │  │  │   Python / Go / Rust / WASM        │   │  │  │
│                      │  │  └────────────────────────────────────┘   │  │  │
│                      │  └────────────────────────────────────────────┘  │  │
│                      │                                                  │  │
│                      │  ┌─────────────┐         ┌─────────────────┐    │  │
│                      │  │   /code     │         │   Alpine Linux  │    │  │
│                      │  │  (vdb 盘)   │         │   (rootfs)      │    │  │
│                      │  └─────────────┘         └─────────────────┘    │  │
│                      └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Agent 生命周期

### 1. 启动阶段

当 Firecracker 启动 microVM 时，Agent 作为 init 进程（PID 1）运行：

```go
func main() {
    fmt.Println("[agent] Nova guest agent starting...")

    // 1. 挂载代码盘
    mountCodeDrive()

    // 2. 监听 vsock
    listener, err := listen(VsockPort)  // 端口 9999

    // 3. 接受连接
    for {
        conn, err := listener.Accept()
        go handleConnection(conn)
    }
}
```

### 2. 代码盘挂载

用户的函数代码通过 ext4 文件系统挂载到 `/code` 目录：

```go
func mountCodeDrive() {
    os.MkdirAll(CodeMountPoint, 0755)  // /code

    cmd := exec.Command("mount", "-t", "ext4", "-o", "ro", "/dev/vdb", CodeMountPoint)
    if out, err := cmd.CombinedOutput(); err != nil {
        // 无法访问代码，必须退出
        os.Exit(1)
    }
}
```

挂载结构：
```
/code/
└── handler          # 用户函数（Python/Go/Rust 可执行文件）
```

### 3. 通信监听

Agent 支持两种监听模式：

```go
func listen(port int) (net.Listener, error) {
    if runtime.GOOS == "linux" {
        // 生产环境：使用 AF_VSOCK
        l, err := vsock.Listen(uint32(port), nil)
        if err == nil {
            return l, nil
        }
    }

    // 开发环境：回退到 Unix socket
    sockPath := fmt.Sprintf("/tmp/nova-agent-%d.sock", port)
    return net.Listen("unix", sockPath)
}
```

| 模式 | 使用场景 | 地址 |
|------|---------|------|
| AF_VSOCK | Firecracker VM 内部 | vsock://2:9999 |
| Unix Socket | 本地开发测试 | /tmp/nova-agent-9999.sock |

## 消息协议

### 消息格式

Agent 使用长度前缀的二进制协议：

```
┌────────────────────────────────────────────────────────┐
│  4 bytes (Big Endian)  │  JSON Payload                 │
│  Message Length        │  {"type":1,"payload":{...}}   │
└────────────────────────────────────────────────────────┘
```

### 消息类型

```go
const (
    MsgTypeInit = 1  // 初始化函数配置
    MsgTypeExec = 2  // 执行函数
    MsgTypeResp = 3  // 响应消息
    MsgTypePing = 4  // 健康检查
    MsgTypeStop = 5  // 停止 Agent
)
```

### 消息结构

```go
type Message struct {
    Type    int             `json:"type"`
    Payload json.RawMessage `json:"payload"`
}
```

### 读写消息

```go
// 读取消息
func readMessage(conn net.Conn) (*Message, error) {
    // 1. 读取 4 字节长度
    lenBuf := make([]byte, 4)
    io.ReadFull(conn, lenBuf)

    // 2. 读取消息体
    data := make([]byte, binary.BigEndian.Uint32(lenBuf))
    io.ReadFull(conn, data)

    // 3. 解析 JSON
    var msg Message
    json.Unmarshal(data, &msg)
    return &msg, nil
}

// 写入消息
func writeMessage(conn net.Conn, msg *Message) error {
    data, _ := json.Marshal(msg)

    // 1. 写入长度
    lenBuf := make([]byte, 4)
    binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
    conn.Write(lenBuf)

    // 2. 写入消息体
    conn.Write(data)
    return nil
}
```

## 消息处理详解

### 1. Init 消息 (MsgTypeInit)

初始化函数运行环境：

```go
type InitPayload struct {
    Runtime string            `json:"runtime"`   // "python", "go", "rust", "wasm"
    Handler string            `json:"handler"`   // "main.handler"
    EnvVars map[string]string `json:"env_vars"`  // 环境变量
    Mode    ExecutionMode     `json:"mode"`      // "process" 或 "persistent"
}
```

处理流程：

```go
func (a *Agent) handleInit(payload json.RawMessage) (*Message, error) {
    var init InitPayload
    json.Unmarshal(payload, &init)

    // 默认使用 process 模式
    if init.Mode == "" {
        init.Mode = ModeProcess
    }

    a.function = &init

    // 如果是 persistent 模式，立即启动进程
    if init.Mode == ModePersistent {
        a.startPersistentProcess()
    }

    return &Message{
        Type:    MsgTypeResp,
        Payload: json.RawMessage(`{"status":"initialized"}`),
    }, nil
}
```

### 2. Exec 消息 (MsgTypeExec)

执行函数调用：

```go
type ExecPayload struct {
    RequestID string          `json:"request_id"`  // 请求唯一标识
    Input     json.RawMessage `json:"input"`       // 函数输入
    TimeoutS  int             `json:"timeout_s"`   // 超时时间
}

type RespPayload struct {
    RequestID  string          `json:"request_id"`
    Output     json.RawMessage `json:"output"`       // 函数输出
    Error      string          `json:"error"`        // 错误信息
    DurationMs int64           `json:"duration_ms"`  // 执行耗时
}
```

处理流程：

```go
func (a *Agent) handleExec(payload json.RawMessage) (*Message, error) {
    var req ExecPayload
    json.Unmarshal(payload, &req)

    start := time.Now()
    output, execErr := a.executeFunction(req.Input)
    duration := time.Since(start).Milliseconds()

    resp := RespPayload{
        RequestID:  req.RequestID,
        DurationMs: duration,
    }

    if execErr != nil {
        resp.Error = execErr.Error()
    } else {
        resp.Output = output
    }

    respData, _ := json.Marshal(resp)
    return &Message{Type: MsgTypeResp, Payload: respData}, nil
}
```

### 3. Ping 消息 (MsgTypePing)

健康检查：

```go
case MsgTypePing:
    return &Message{
        Type:    MsgTypeResp,
        Payload: json.RawMessage(`{"status":"ok"}`),
    }, nil
```

### 4. Stop 消息 (MsgTypeStop)

优雅关闭：

```go
if msg.Type == MsgTypeStop {
    fmt.Println("[agent] Received stop, shutting down...")
    writeMessage(conn, &Message{
        Type:    MsgTypeResp,
        Payload: json.RawMessage(`{"status":"stopping"}`),
    })
    os.Exit(0)
}
```

## 函数执行模式

### Process 模式（默认）

每次调用创建新进程，提供完全隔离：

```
请求 1 ──► fork() ──► python3 handler.py ──► 输出 ──► 进程退出
请求 2 ──► fork() ──► python3 handler.py ──► 输出 ──► 进程退出
请求 3 ──► fork() ──► python3 handler.py ──► 输出 ──► 进程退出
```

```go
func (a *Agent) executeFunction(input json.RawMessage) (json.RawMessage, error) {
    // 1. 写入输入文件
    os.WriteFile("/tmp/input.json", input, 0644)

    // 2. 根据运行时构建命令
    var cmd *exec.Cmd
    switch a.function.Runtime {
    case "python":
        cmd = exec.Command("python3", CodePath, "/tmp/input.json")
    case "go", "rust":
        cmd = exec.Command(CodePath, "/tmp/input.json")
    case "wasm":
        cmd = exec.Command("wasmtime", CodePath, "--", "/tmp/input.json")
    }

    // 3. 设置环境变量
    cmd.Env = append(os.Environ(), "NOVA_CODE_DIR="+CodeMountPoint)
    for k, v := range a.function.EnvVars {
        cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
    }

    // 4. 执行并获取输出
    output, err := cmd.Output()

    // 5. 验证并返回 JSON
    if json.Valid(output) {
        return output, nil
    }
    result, _ := json.Marshal(string(output))
    return result, nil
}
```

**优点**：
- 完全隔离，每次调用互不影响
- 内存自动释放
- 简单可靠

**缺点**：
- 每次调用都有进程启动开销
- 无法复用数据库连接等资源

### Persistent 模式

保持函数进程存活，通过 stdin/stdout 通信：

```
                    ┌─────────────────────────────────┐
请求 1 ──► stdin ──►│                                 │──► stdout ──► 响应 1
请求 2 ──► stdin ──►│   Long-running Python Process  │──► stdout ──► 响应 2
请求 3 ──► stdin ──►│   (保持数据库连接)               │──► stdout ──► 响应 3
                    └─────────────────────────────────┘
```

#### 启动持久进程

```go
func (a *Agent) startPersistentProcess() error {
    var cmd *exec.Cmd
    switch a.function.Runtime {
    case "python":
        cmd = exec.Command("python3", "-u", CodePath, "--persistent")
    case "go", "rust":
        cmd = exec.Command(CodePath, "--persistent")
    default:
        return fmt.Errorf("persistent mode not supported for: %s", a.function.Runtime)
    }

    // 设置环境变量
    cmd.Env = append(os.Environ(), "NOVA_MODE=persistent")
    for k, v := range a.function.EnvVars {
        cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
    }

    // 获取 stdin/stdout 管道
    stdin, _ := cmd.StdinPipe()
    stdout, _ := cmd.StdoutPipe()
    cmd.Stderr = os.Stderr

    cmd.Start()

    a.persistentProc = cmd
    a.persistentIn = stdin
    a.persistentOutRaw = stdout
    a.persistentOut = bufio.NewReader(stdout)

    return nil
}
```

#### 执行请求

```go
func (a *Agent) executePersistent(input json.RawMessage) (json.RawMessage, error) {
    // 协议: {"input": ...}\n -> {"output": ...}\n

    // 1. 发送请求
    req := map[string]json.RawMessage{"input": input}
    reqBytes, _ := json.Marshal(req)
    reqBytes = append(reqBytes, '\n')

    if _, err := a.persistentIn.Write(reqBytes); err != nil {
        // 进程可能已死，尝试重启
        a.stopPersistentProcess()
        a.startPersistentProcess()
        a.persistentIn.Write(reqBytes)
    }

    // 2. 读取响应
    line, err := a.persistentOut.ReadBytes('\n')

    // 3. 解析响应
    var resp struct {
        Output json.RawMessage `json:"output"`
        Error  string          `json:"error,omitempty"`
    }
    json.Unmarshal(line, &resp)

    if resp.Error != "" {
        return nil, fmt.Errorf("%s", resp.Error)
    }
    return resp.Output, nil
}
```

#### 停止持久进程

```go
func (a *Agent) stopPersistentProcess() {
    if a.persistentProc != nil {
        a.persistentIn.Close()
        a.persistentOutRaw.Close()
        a.persistentProc.Process.Kill()
        a.persistentProc.Wait()

        a.persistentProc = nil
        a.persistentIn = nil
        a.persistentOut = nil
        a.persistentOutRaw = nil
    }
}
```

**优点**：
- 可复用数据库连接、TCP 连接等资源
- 避免重复初始化开销
- 适合需要状态保持的场景

**缺点**：
- 内存不会自动释放
- 需要函数代码支持持久模式
- 潜在的内存泄漏风险

## 运行时支持

### Python 运行时

```go
// Process 模式
cmd = exec.Command("python3", CodePath, "/tmp/input.json")

// Persistent 模式
cmd = exec.Command("python3", "-u", CodePath, "--persistent")
```

函数代码示例：
```python
#!/usr/bin/env python3
import json
import sys

def handler(event):
    return {"message": f"Hello, {event.get('name', 'World')}!"}

if __name__ == "__main__":
    if "--persistent" in sys.argv:
        # 持久模式：从 stdin 读取 JSON 行
        for line in sys.stdin:
            req = json.loads(line)
            result = handler(req.get("input", {}))
            print(json.dumps({"output": result}), flush=True)
    else:
        # 进程模式：从文件读取输入
        with open(sys.argv[1]) as f:
            event = json.load(f)
        print(json.dumps(handler(event)))
```

### Go 运行时

```go
// Process 模式
cmd = exec.Command(CodePath, "/tmp/input.json")

// Persistent 模式
cmd = exec.Command(CodePath, "--persistent")
```

函数代码示例：
```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

type Event struct {
    Name string `json:"name"`
}

type Response struct {
    Message string `json:"message"`
}

func handler(event Event) Response {
    name := event.Name
    if name == "" {
        name = "World"
    }
    return Response{Message: fmt.Sprintf("Hello, %s!", name)}
}

func main() {
    if len(os.Args) > 1 && os.Args[1] == "--persistent" {
        // 持久模式
        scanner := bufio.NewScanner(os.Stdin)
        for scanner.Scan() {
            var req struct {
                Input Event `json:"input"`
            }
            json.Unmarshal(scanner.Bytes(), &req)
            result := handler(req.Input)
            output, _ := json.Marshal(map[string]interface{}{"output": result})
            fmt.Println(string(output))
        }
    } else {
        // 进程模式
        data, _ := os.ReadFile(os.Args[1])
        var event Event
        json.Unmarshal(data, &event)
        result := handler(event)
        output, _ := json.Marshal(result)
        fmt.Println(string(output))
    }
}
```

### Rust 运行时

```go
cmd = exec.Command(CodePath, "/tmp/input.json")
```

### WASM 运行时

```go
cmd = exec.Command("wasmtime", CodePath, "--", "/tmp/input.json")
```

## 数据流示意

### 完整请求流程

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│   用户    │    │   CLI    │    │  Host    │    │  Agent   │
└────┬─────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
     │               │               │               │
     │ nova invoke   │               │               │
     │ hello -p {}   │               │               │
     │──────────────►│               │               │
     │               │               │               │
     │               │ GetVM()       │               │
     │               │──────────────►│               │
     │               │               │               │
     │               │               │ vsock connect │
     │               │               │──────────────►│
     │               │               │               │
     │               │               │ Init Message  │
     │               │               │──────────────►│
     │               │               │               │
     │               │               │ Init Response │
     │               │               │◄──────────────│
     │               │               │               │
     │               │               │ Exec Message  │
     │               │               │──────────────►│
     │               │               │               │
     │               │               │               │ fork()
     │               │               │               │──────┐
     │               │               │               │      │
     │               │               │               │ exec │
     │               │               │               │      │
     │               │               │               │◄─────┘
     │               │               │               │
     │               │               │ Exec Response │
     │               │               │◄──────────────│
     │               │               │               │
     │               │ InvokeResponse│               │
     │               │◄──────────────│               │
     │               │               │               │
     │ Output        │               │               │
     │◄──────────────│               │               │
     │               │               │               │
```

### 时序说明

1. **用户发起调用** - CLI 解析参数
2. **获取 VM** - 从池中获取或创建新 VM
3. **建立连接** - 通过 vsock 连接到 Agent
4. **初始化** - 发送 Init 消息配置函数
5. **执行** - 发送 Exec 消息
6. **运行函数** - Agent fork 进程执行用户代码
7. **返回结果** - 响应通过 vsock 返回宿主机
8. **输出显示** - CLI 展示结果

## Agent 状态机

```
                    ┌───────────────┐
                    │   Starting    │
                    └───────┬───────┘
                            │
                            │ mountCodeDrive()
                            │ listen()
                            ▼
                    ┌───────────────┐
          ┌────────│   Listening   │◄────────┐
          │        └───────┬───────┘         │
          │                │                 │
          │                │ Accept()        │
          │                ▼                 │
          │        ┌───────────────┐         │
          │        │  Connected    │         │
          │        └───────┬───────┘         │
          │                │                 │
          │     ┌──────────┼──────────┐      │
          │     │          │          │      │
          │     ▼          ▼          ▼      │
          │  ┌─────┐   ┌─────┐   ┌─────┐    │
          │  │Init │   │Exec │   │Ping │    │
          │  └──┬──┘   └──┬──┘   └──┬──┘    │
          │     │         │         │        │
          │     └─────────┴─────────┘        │
          │                │                 │
          │                │ Response        │
          │                └─────────────────┘
          │
          │ Stop Message
          ▼
    ┌───────────────┐
    │   Shutdown    │
    └───────────────┘
```

## 错误处理

### 函数执行错误

```go
output, err := cmd.Output()
if err != nil {
    if exitErr, ok := err.(*exec.ExitError); ok {
        return nil, fmt.Errorf("exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
    }
    return nil, err
}
```

### 连接错误

```go
func handleConnection(conn net.Conn) {
    defer conn.Close()

    for {
        msg, err := readMessage(conn)
        if err != nil {
            if err != io.EOF {
                fmt.Fprintf(os.Stderr, "[agent] Read error: %v\n", err)
            }
            return  // 连接关闭，退出处理循环
        }
        // ...
    }
}
```

### 持久进程错误恢复

```go
if _, err := a.persistentIn.Write(reqBytes); err != nil {
    // 进程可能已死，尝试重启
    a.stopPersistentProcess()
    if err := a.startPersistentProcess(); err != nil {
        return nil, fmt.Errorf("restart persistent process: %w", err)
    }
    // 重试写入
    if _, err := a.persistentIn.Write(reqBytes); err != nil {
        return nil, fmt.Errorf("write to persistent process: %w", err)
    }
}
```

## 安全考虑

### 代码盘只读挂载

```go
cmd := exec.Command("mount", "-t", "ext4", "-o", "ro", "/dev/vdb", CodeMountPoint)
```

用户代码只能读取，无法修改自身。

### 环境隔离

- 每个 VM 有独立的内核和文件系统
- 网络通过 TAP 设备隔离
- vsock 仅允许与宿主机通信

### 资源限制

通过 Firecracker 配置限制：
- vCPU 数量
- 内存大小
- 磁盘 IOPS
- 网络带宽

## 调试技巧

### 查看 Agent 日志

Agent 的日志输出到 stderr，会被 Firecracker 捕获：

```bash
# 查看 VM 日志
cat /tmp/nova/vm-<id>/firecracker.log
```

### 本地开发测试

不在 VM 内时，Agent 会回退到 Unix socket：

```bash
# 启动 Agent（本地测试）
./nova-agent

# 连接测试
nc -U /tmp/nova-agent-9999.sock
```

### 手动发送消息

```python
import socket
import struct
import json

sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.connect("/tmp/nova-agent-9999.sock")

def send_message(msg):
    data = json.dumps(msg).encode()
    sock.send(struct.pack(">I", len(data)) + data)

def recv_message():
    length = struct.unpack(">I", sock.recv(4))[0]
    return json.loads(sock.recv(length))

# 发送 Ping
send_message({"type": 4, "payload": {}})
print(recv_message())  # {"type": 3, "payload": {"status": "ok"}}
```

## 性能优化

### 进程复用（Persistent 模式）

对于数据库密集型应用，使用 persistent 模式可显著减少连接建立开销：

| 场景 | Process 模式 | Persistent 模式 |
|------|-------------|-----------------|
| 首次调用 | ~100ms (含连接) | ~100ms (含连接) |
| 后续调用 | ~100ms (每次重连) | ~5ms (复用连接) |

### 预热 VM

通过 `min-replicas` 配置保持预热 VM：

```bash
nova register hello --runtime python --code ./hello.py --min-replicas 2
```

### vsock 性能

vsock 是零拷贝的内核级通信机制，延迟约 10-50μs，远低于网络通信。

## 相关文件

- `cmd/agent/main.go` - Agent 主程序
- `internal/pkg/vsock/` - vsock 库封装
- `internal/firecracker/vm.go` - VM 管理（宿主机侧）
- `internal/executor/executor.go` - 执行器（调用 Agent）
