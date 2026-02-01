# Linux Quickstart（Firecracker / microVM 模式）

本指南用于在 **Linux x86_64** 上以 **KVM + Firecracker microVM** 的方式运行 Nova（可执行所有 VM runtimes）。

> 说明：
> - `nova daemon` 需要做网络桥接/TAP（`ip link`），建议用 `root` 启动。
> - `--config` 目前读取的是 **JSON**（见 `internal/config/config.go`），仓库里的 `configs/nova.yaml` 仅作参考。

## 0) 前置条件

- Linux x86_64
- CPU/系统开启虚拟化，且存在 `/dev/kvm`
- `Go >= 1.24`
- 基础工具：`git curl tar unzip e2fsprogs`（提供 `mkfs.ext4` / `debugfs`）

Debian/Ubuntu：

```bash
sudo apt-get update
sudo apt-get install -y git curl tar unzip e2fsprogs
```

RHEL/CentOS/Amazon Linux：

```bash
sudo yum install -y git curl tar unzip e2fsprogs
```

## 1) 构建 Nova（二进制）

```bash
git clone https://github.com/oriys/nova.git
cd nova

make build
```

产物：
- `bin/nova`
- `bin/nova-agent`（会被注入到 rootfs 的 `/init`）

## 2) 安装到 `/opt/nova/bin`

```bash
sudo install -Dm755 bin/nova /opt/nova/bin/nova
sudo install -Dm755 bin/nova-agent /opt/nova/bin/nova-agent
```

把 `nova` 放到 PATH（推荐，方便用 `sudo` 执行）：

```bash
sudo ln -sf /opt/nova/bin/nova /usr/local/bin/nova
```

## 3) 安装 Firecracker / Kernel / Rootfs / Postgres

> 重要：先把 `nova-agent` 放到 `/opt/nova/bin/nova-agent`（上一步已完成），再运行安装脚本，rootfs 才会包含正确的 `/init`。

```bash
sudo bash scripts/install.sh
```

脚本会生成：
- `/opt/nova/bin/firecracker`（指向版本化的 Firecracker 二进制）
- `/opt/nova/kernel/vmlinux`
- `/opt/nova/rootfs/*.ext4`
- 本地 Postgres（db/user/password 均为 `nova`，DSN 默认 `postgres://nova:nova@localhost:5432/nova?sslmode=disable`）

## 4) 启动 Nova Daemon（HTTP API）

```bash
sudo /opt/nova/bin/nova daemon --http :9000
```

健康检查：

```bash
curl -fsSL http://127.0.0.1:9000/health
```

## 5) 注册并调用一个 Hello（通过 HTTP API）

注册（写入 Postgres）：

```bash
nova register hello-python --runtime python --code "$(pwd)/examples/hello.py"
```

调用（推荐走 daemon 的 HTTP API，才能复用 warm VM）：

```bash
curl -fsSL \
  -H 'content-type: application/json' \
  -d '{"name":"World"}' \
  http://127.0.0.1:9000/functions/hello-python/invoke
```

> 也可以用 CLI 直接执行 VM（一次性创建 pool，不保证 warm 复用）：`sudo nova invoke hello-python --payload '{"name":"World"}'`

## 6) 一键 smoke test：所有 runtime

准备并测试所有 runtime 的 hello（会自动跳过缺少工具链的 runtime）：

```bash
sudo ./examples/test_all_runtimes.sh
```

可选工具链（没装也能跑，只是会跳过对应 runtime）：
- Rust（musl）：`rustup target add x86_64-unknown-linux-musl`
- Rust（WASI）：`rustup target add wasm32-wasip1`（或旧版 `wasm32-wasi`）
- Java：`javac` + `jar`
- .NET：`dotnet`

## 7) （可选）不通过 loop-mount 重新构建 rootfs

如果你想在本机用“无挂载”的方式重建 rootfs（`mkfs.ext4 -d`），可以运行：

```bash
sudo ./scripts/build_rootfs.sh --agent /opt/nova/bin/nova-agent --out-dir /opt/nova/rootfs
```

脚本支持用环境变量 pin 版本（例如 Deno / Bun / Wasmtime / .NET / Alpine minirootfs）。

## 8) 常见问题

- **没有 `/dev/kvm`**：检查 BIOS/虚拟化开关，或云主机实例类型是否支持 KVM。
- **权限问题（网络/bridge）**：请用 `sudo` 启动 `nova daemon`，或给二进制配置 capabilities（进阶玩法）。
- **`debugfs` / `mkfs.ext4` 找不到**：安装 `e2fsprogs`。
- **Firecracker 路径不对**：确认 `/opt/nova/bin/firecracker` 存在且可执行。
