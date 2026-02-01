# Nova Examples

测试用例覆盖 CPU 计算、超时、网络、磁盘 I/O 四种场景（目前提供 Python / Go / Rust 三种语言实现）。

同时提供 **Hello** 用例覆盖 Nova 当前支持的全部 VM runtime：
`python, go, rust, wasm, node, ruby, java, php, dotnet, deno, bun`。

## 快速测试

```bash
# 构建所有 runtime 的 hello 产物（输出到 examples/build/<runtime>/handler）
./build_runtime_fixtures.sh

# 测试所有运行时 (python, go, rust, wasm, node, ruby, java, php, dotnet, deno, bun)
./test_all_runtimes.sh

# 仅测试 Python 和 Go
./test_hello.sh
```

## 用例列表

| 场景 | Python | Go | Rust |
|------|--------|----|----- |
| CPU 计算 | `cpu_intensive.py` | `cpu_intensive.go` | `cpu_intensive.rs` |
| 超时测试 | `timeout_test.py` | `timeout_test.go` | `timeout_test.rs` |
| 网络请求 | `network_test.py` | `network_test.go` | `network_test.rs` |
| 磁盘 I/O | `disk_test.py` | `disk_test.go` | `disk_test.rs` |
| Hello World | `hello.py` | `hello.go` | `hello.rs` |

## Hello（全运行时）

| Runtime | 代码 |
|--------|------|
| python | `hello.py` |
| go | `hello.go` |
| rust | `hello.rs` |
| wasm | `hello_wasm.rs` |
| node | `hello_node.js` |
| ruby | `hello_ruby.rb` |
| java | `hello_java/Main.java` |
| php | `hello_php.php` |
| dotnet | `hello_dotnet/Program.cs` |
| deno | `hello_deno.js` |
| bun | `hello_bun.js` |

## 编译 Go 示例

```bash
# 编译为静态二进制（Linux amd64）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cpu_intensive cpu_intensive.go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o timeout_test timeout_test.go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o network_test network_test.go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o disk_test disk_test.go
```

## 编译 Rust 示例

```bash
# 添加 musl target
rustup target add x86_64-unknown-linux-musl

# 编译（需要 Cargo.toml 配置依赖）
cargo build --release --target x86_64-unknown-linux-musl
```

## 编译 WASM 示例

WASM runtime 使用 WASI（配合 rootfs 里的 `wasmtime`）。

```bash
# 新版 Rust（推荐）
rustup target add wasm32-wasip1

# 或旧版 target
rustup target add wasm32-wasi

# 编译
cargo build --release --target wasm32-wasip1 --bin hello-wasm
```

生成的 `.wasm` 文件可直接用于 `nova register --runtime wasm --code ...`（平台会注入到 VM 的 `/code/handler`）。

## 编译 Java 示例

```bash
# 生成可运行 JAR（Main-Class: Main）
mkdir -p build/java-tmp/classes
javac -d build/java-tmp/classes hello_java/Main.java
echo "Main-Class: Main" > build/java-tmp/manifest.mf
jar cfm build/java/handler build/java-tmp/manifest.mf -C build/java-tmp/classes .
```

Rust 依赖 (Cargo.toml):
```toml
[dependencies]
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"
ureq = "2.9"  # 仅 network_test 需要
```

## 注册和调用

### 1. CPU 计算测试

计算质数，测试 CPU 性能和 vCPU 限制。

```bash
# 注册
nova register cpu-test --runtime python --code examples/cpu_intensive.py --vcpus 2

# 调用 - 计算 10000 以内的质数
nova invoke cpu-test --payload '{"limit": 10000}'

# 调用 - 计算 100000 以内的质数（CPU 密集）
nova invoke cpu-test --payload '{"limit": 100000}'
```

预期输出：
```json
{
  "limit": 10000,
  "count": 1229,
  "last_10": [9887, 9901, 9907, 9923, 9929, 9931, 9941, 9949, 9967, 9973],
  "elapsed_ms": 15
}
```

### 2. 超时测试

测试函数执行超时机制。

```bash
# 注册（设置 10 秒超时）
nova register timeout-test --runtime python --code examples/timeout_test.py --timeout 10

# 调用 - 睡眠 3 秒（应成功）
nova invoke timeout-test --payload '{"sleep_seconds": 3}'

# 调用 - 睡眠 15 秒（应超时）
nova invoke timeout-test --payload '{"sleep_seconds": 15}'
```

预期输出（成功）：
```json
{
  "requested_sleep": 3,
  "actual_sleep": 3.0,
  "status": "completed"
}
```

### 3. 网络测试

测试 VM 访问外部网络能力和带宽限制。

```bash
# 注册（限制网络带宽 1MB/s）
nova register network-test \
  --runtime python \
  --code examples/network_test.py \
  --net-rx-bandwidth 1048576 \
  --net-tx-bandwidth 1048576

# 调用 - 请求 httpbin
nova invoke network-test --payload '{"url": "https://httpbin.org/get"}'

# 调用 - 请求 GitHub API
nova invoke network-test --payload '{"url": "https://api.github.com/users/octocat"}'

# 调用 - 测试大文件下载（观察带宽限制效果）
nova invoke network-test --payload '{"url": "https://httpbin.org/bytes/1048576"}'
```

预期输出：
```json
{
  "url": "https://httpbin.org/get",
  "status": 200,
  "elapsed_ms": 342,
  "response": {
    "args": {},
    "headers": {"User-Agent": "Nova/1.0", ...},
    "origin": "x.x.x.x",
    "url": "https://httpbin.org/get"
  }
}
```

### 4. 磁盘 I/O 测试

测试磁盘读写性能和 IOPS 限制。

```bash
# 注册（限制磁盘 IOPS 和带宽）
nova register disk-test \
  --runtime python \
  --code examples/disk_test.py \
  --disk-iops 100 \
  --disk-bandwidth 5242880

# 调用 - 写入 1MB 文件 10 次
nova invoke disk-test --payload '{"size_kb": 1024, "iterations": 10}'

# 调用 - 写入 10MB 文件（观察带宽限制）
nova invoke disk-test --payload '{"size_kb": 10240, "iterations": 5}'
```

预期输出：
```json
{
  "size_kb": 1024,
  "iterations": 10,
  "write_times_ms": [12, 11, 10, 11, 10, 11, 10, 10, 11, 10],
  "read_times_ms": [2, 1, 1, 1, 1, 1, 1, 1, 1, 1],
  "avg_write_ms": 10.6,
  "avg_read_ms": 1.1,
  "write_throughput_mbps": 94.34,
  "read_throughput_mbps": 909.09
}
```

## 批量测试脚本

```bash
#!/bin/bash
# test_all.sh - 批量测试所有场景

set -e

echo "=== Registering functions ==="
nova register cpu-py --runtime python --code examples/cpu_intensive.py --vcpus 2
nova register timeout-py --runtime python --code examples/timeout_test.py --timeout 10
nova register network-py --runtime python --code examples/network_test.py
nova register disk-py --runtime python --code examples/disk_test.py --disk-iops 500

echo ""
echo "=== CPU Test ==="
nova invoke cpu-py --payload '{"limit": 50000}'

echo ""
echo "=== Timeout Test (should succeed) ==="
nova invoke timeout-py --payload '{"sleep_seconds": 2}'

echo ""
echo "=== Network Test ==="
nova invoke network-py --payload '{"url": "https://httpbin.org/ip"}'

echo ""
echo "=== Disk Test ==="
nova invoke disk-py --payload '{"size_kb": 512, "iterations": 5}'

echo ""
echo "=== Cleanup ==="
nova delete cpu-py
nova delete timeout-py
nova delete network-py
nova delete disk-py

echo "Done!"
```

## Go 版本测试

```bash
# 编译
cd examples
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cpu_intensive_go cpu_intensive.go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o disk_test_go disk_test.go

# 注册和测试
nova register cpu-go --runtime go --code examples/cpu_intensive_go --vcpus 2
nova invoke cpu-go --payload '{"limit": 100000}'

nova register disk-go --runtime go --code examples/disk_test_go
nova invoke disk-go --payload '{"size_kb": 2048, "iterations": 5}'
```

## 性能对比

运行相同的 CPU 计算任务（计算 100000 以内质数）：

| 语言 | 平均耗时 | 说明 |
|------|---------|------|
| Go | ~50ms | 静态编译，启动快 |
| Rust | ~45ms | 静态编译，性能最优 |
| Python | ~800ms | 解释执行，启动有 overhead |

> 注：实际性能取决于 vCPU 配置和宿主机硬件
