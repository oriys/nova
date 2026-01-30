# microVM 原理

## 什么是 microVM

microVM 是专门为运行单一任务（如一个函数）设计的超轻量级虚拟机，和传统 VM 的核心区别是：

| | 传统 VM | microVM (Firecracker) |
|---|---------|----------------------|
| 启动时间 | 30秒+ | <125ms |
| 内存开销 | 512MB+ | <5MB |
| 设备模拟 | 完整 (显卡/USB/声卡) | 最小化 (virtio 块/网络/vsock) |
| Guest OS | 完整 Linux 发行版 | 最小内核 + init 进程 |
| 用途 | 通用服务器 | 单函数/容器隔离 |

## 核心技术栈

```
┌─────────────────────────────────────────────────────┐
│                    用户空间                          │
│  ┌─────────────────────────────────────────────┐   │
│  │  Firecracker VMM (用户态进程)                │   │
│  │  - virtio-blk (块设备)                       │   │
│  │  - virtio-net (网络)                         │   │
│  │  - virtio-vsock (宿主机↔VM 通信)             │   │
│  │  - 串口 (ttyS0)                              │   │
│  └─────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────┤
│                    KVM (内核)                        │
│  - vCPU 虚拟化 (VT-x/AMD-V)                         │
│  - 内存虚拟化 (EPT/NPT)                             │
│  - 中断注入 (APIC)                                  │
└─────────────────────────────────────────────────────┘
```

## 关键原理

### 1. KVM 硬件虚拟化

Linux 内核的 KVM 模块利用 CPU 的硬件虚拟化扩展（Intel VT-x / AMD-V）：

```c
// 创建 VM
int vm_fd = ioctl(kvm_fd, KVM_CREATE_VM, 0);

// 创建 vCPU
int vcpu_fd = ioctl(vm_fd, KVM_CREATE_VCPU, 0);

// 分配 Guest 内存
void *mem = mmap(...);
ioctl(vm_fd, KVM_SET_USER_MEMORY_REGION, &region);

// 运行 vCPU
while (1) {
    ioctl(vcpu_fd, KVM_RUN, 0);
    // 处理 VM Exit (I/O, MMIO, 中断等)
}
```

**优势**：Guest 代码直接在物理 CPU 上执行（不是模拟），性能接近裸机。

### 2. virtio 半虚拟化设备

传统虚拟化需要模拟完整硬件（如 IDE 硬盘），性能损失大。virtio 是专门设计的虚拟化设备协议：

```
Guest (VM 内核)              Host (Firecracker)
┌─────────────────┐         ┌─────────────────┐
│ virtio-blk 驱动 │◄──共享──►│ virtio 后端     │
│                 │  内存   │                 │
│ vring (环形队列)│         │ 事件处理        │
└─────────────────┘         └─────────────────┘
```

- **vring**：Guest 和 Host 共享的内存队列
- Guest 写请求到 vring，通知 Host（eventfd）
- Host 处理后写结果到 vring，通知 Guest

### 3. vsock (VM Socket)

传统 VM 网络需要配置 TAP/Bridge/IP。vsock 提供更简单的 Guest↔Host 通信：

```
宿主机                              Guest
┌──────────────────────┐        ┌──────────────────────┐
│ connect(vsock, CID)  │        │ listen(vsock)        │
│         │            │        │        │             │
│   Unix Socket        │◄──────►│  AF_VSOCK            │
│   /tmp/vm.vsock      │  虚拟  │  port 9999           │
└──────────────────────┘  设备  └──────────────────────┘
```

CID (Context ID) 标识每个 VM，类似 IP 地址。

### 4. 最小化启动

Firecracker 跳过传统 VM 启动的大部分步骤：

| 传统 VM 启动 | Firecracker 启动 |
|-------------|-----------------|
| BIOS/UEFI | 跳过 |
| Bootloader (GRUB) | 跳过 |
| 内核解压缩 | 直接加载未压缩内核 |
| initramfs | 跳过 |
| systemd/init 服务 | 直接运行 /init (单进程) |

```
内核启动 → 挂载 rootfs → 执行 /init (nova-agent)
         (约 100ms)
```

## 隔离机制

```
┌─────────────────────────────────────────────────────┐
│                    宿主机 Linux                      │
│                                                     │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐       │
│  │ VM 1      │  │ VM 2      │  │ VM 3      │       │
│  │ (函数 A)  │  │ (函数 B)  │  │ (函数 C)  │       │
│  │           │  │           │  │           │       │
│  │ 独立内核  │  │ 独立内核  │  │ 独立内核  │       │
│  │ 独立内存  │  │ 独立内存  │  │ 独立内存  │       │
│  │ 独立文件系统│ │ 独立文件系统│ │ 独立文件系统│      │
│  └───────────┘  └───────────┘  └───────────┘       │
│       │              │              │               │
│       └──────────────┼──────────────┘               │
│                      │                              │
│                ┌─────┴─────┐                        │
│                │ Firecracker │                      │
│                │ (用户态 VMM)│                      │
│                └─────┬─────┘                        │
│                      │                              │
│                ┌─────┴─────┐                        │
│                │    KVM     │                       │
│                │  /dev/kvm  │                       │
│                └───────────┘                        │
└─────────────────────────────────────────────────────┘
```

**隔离边界**：

- **CPU**：每个 VM 有独立的虚拟 CPU，通过 KVM 硬件隔离
- **内存**：EPT/NPT 页表隔离，VM 无法访问其他 VM 或宿主机内存
- **文件系统**：独立的 rootfs 镜像
- **网络**：独立的 TAP 设备 + iptables 规则
- **设备**：只暴露必要的 virtio 设备

## Firecracker vs 容器

```
容器 (Docker/containerd)           microVM (Firecracker)
┌──────────────────────┐         ┌──────────────────────┐
│ 容器进程              │         │ VM 内核              │
│    │                 │         │    │                │
│ cgroups (资源限制)    │         │ KVM vCPU/内存隔离    │
│ namespaces (隔离视图) │         │ 独立内核态           │
│ seccomp (系统调用过滤)│         │ virtio 设备          │
│    │                 │         │    │                │
│ 共享宿主机内核        │         │ 独立 Guest 内核      │
└──────────────────────┘         └──────────────────────┘
```

**容器逃逸风险**：容器和宿主机共享内核，内核漏洞可能导致逃逸。

**microVM 优势**：每个 VM 有独立内核，即使 Guest 内核被攻破，也无法影响宿主机。AWS Lambda、Cloudflare Workers 等都使用这种架构。

## 性能代价

| 指标 | 容器 | microVM |
|------|------|---------|
| 启动时间 | ~100ms | ~125ms |
| 内存开销 | ~10MB | ~5MB (Firecracker 优化) |
| CPU 开销 | <1% | <1% (VT-x 直接执行) |
| 冷启动延迟 | 低 | 稍高 |
| 安全隔离 | 中 | 高 |

Firecracker 通过极简设计将 microVM 开销降到接近容器水平，同时保持硬件级隔离。

## Nova 中的实现

### VM 生命周期

```go
// 1. 启动 Firecracker 进程
cmd := exec.Command("firecracker", "--api-sock", socketPath)
cmd.Start()

// 2. 通过 HTTP API 配置 VM
// 设置内核
PUT /boot-source {"kernel_image_path": "/opt/nova/kernel/vmlinux", ...}

// 设置磁盘
PUT /drives/rootfs {"path_on_host": "/opt/nova/rootfs/base.ext4", ...}
PUT /drives/code {"path_on_host": "/tmp/nova/vm-code.ext4", ...}

// 设置网络
PUT /network-interfaces/eth0 {"host_dev_name": "nova-abc123", ...}

// 设置 vsock
PUT /vsock {"guest_cid": 100, "uds_path": "/tmp/nova/vm.vsock"}

// 设置资源限制
PUT /machine-config {"vcpu_count": 1, "mem_size_mib": 128}

// 3. 启动 VM
PUT /actions {"action_type": "InstanceStart"}

// 4. 通过 vsock 与 Guest Agent 通信
conn, _ := net.Dial("unix", "/tmp/nova/vm.vsock")
conn.Write(execRequest)
conn.Read(execResponse)

// 5. 停止 VM
syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)
```

### 双磁盘架构

```
┌─────────────────────────────────────────┐
│               microVM                    │
│                                          │
│  /dev/vda (rootfs)     /dev/vdb (code)  │
│  ┌──────────────┐     ┌──────────────┐  │
│  │ base.ext4    │     │ code.ext4    │  │
│  │ (只读, 共享) │     │ (只读, 独立) │  │
│  │              │     │              │  │
│  │ /init        │     │ /handler     │  │
│  │ (nova-agent) │     │ (用户代码)   │  │
│  └──────────────┘     └──────────────┘  │
│         │                    │          │
│         └────────────────────┘          │
│                    │                    │
│         mount /dev/vdb → /code          │
│         exec /code/handler              │
└─────────────────────────────────────────┘
```

**优势**：

- rootfs 只读共享，节省存储
- 代码盘通过 debugfs 注入，无需 root 权限
- 每个 VM 隔离的代码环境

### 资源限制

Nova 支持通过 Firecracker API 设置：

```go
// CPU 和内存
PUT /machine-config {
    "vcpu_count": 2,
    "mem_size_mib": 256
}

// 磁盘带宽限制
PUT /drives/code {
    "rate_limiter": {
        "bandwidth": {"size": 10485760, "refill_time": 1000},
        "ops": {"size": 1000, "refill_time": 1000}
    }
}

// 网络带宽限制
PUT /network-interfaces/eth0 {
    "rx_rate_limiter": {"bandwidth": {"size": 5242880, ...}},
    "tx_rate_limiter": {"bandwidth": {"size": 5242880, ...}}
}
```

## 参考资料

- [Firecracker Design](https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md)
- [KVM API Documentation](https://www.kernel.org/doc/Documentation/virtual/kvm/api.txt)
- [virtio Specification](https://docs.oasis-open.org/virtio/virtio/v1.1/virtio-v1.1.html)
- [vsock(7) man page](https://man7.org/linux/man-pages/man7/vsock.7.html)
