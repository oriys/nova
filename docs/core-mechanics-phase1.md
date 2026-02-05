# 阶段一：核心性能与机制增强（Core Mechanics）

本文档定义 Nova 第一阶段的核心机制升级方案，目标是显著降低冷启动时延、解耦运行时生态、以及建立可控的并发与扩缩容策略。

## 目标与成功指标

- **冷启动**：MicroVM 冷启动从当前约 `100ms~500ms` 降到 `<10ms`（P50），P95 明显下降。
- **运行时解耦**：从“平台硬编码运行时”升级为“用户自带引导程序（bootstrap）”。
- **资源效率**：实现 Scale-to-Zero，空闲时资源回收；高峰可按策略扩容。

---

## 1) Firecracker 内存快照（Snapshotting & Restore）

### 现状

当前调用路径以“创建新 VM 或从普通 warm 池复用”为主，仍存在明显冷启动开销。

### 方案

引入 **Init 后快照**：

1. 启动 VM，完成运行时初始化（例如 import 常用库、加载 handler 元信息）。
2. VM 进入“可执行但未处理业务请求”的就绪态。
3. 调用 Firecracker snapshot API，生成：
   - 内存快照（memory file）
   - VM 状态快照（vmstate）
4. 后续冷启动优先从快照恢复，而不是全量启动。

### 设计要点

- **快照粒度**：按 `runtime + function version + memory size` 分层缓存，避免错配。
- **失效策略**：代码版本变更、依赖更新、runtime 镜像更新后自动失效并重建快照。
- **存储管理**：快照落盘在本地高速盘，配合 TTL + LRU 回收。

### 关键挑战与处理建议

- **随机数状态复用风险**：
  - 恢复后在 guest 侧强制触发熵池刷新（例如读取 `getrandom`，必要时注入 entropy）。
- **时钟漂移**：
  - 恢复后立即重新同步 monotonic/time 基准，避免 timeout 误判。
- **连接类资源失效（DB/Socket）**：
  - 将连接初始化移到“请求处理路径”并实现惰性重连；快照中不保留活动连接。

### 验收标准

- 基准函数（hello + 中等依赖）恢复路径 P50 `<10ms`。
- 连续压测 10k 次恢复无状态串扰。
- 快照失效与重建逻辑可观测（日志 + 指标）。

---

## 2) 自定义 Runtime API（Custom Runtime / BYOL）

### 现状

当前 runtime 执行逻辑由 agent/后端内置分支维护，语言扩展成本高。

### 方案

实现类似 AWS Lambda Custom Runtime 的 **bootstrap 协议**：

- 平台只负责：
  - 投递事件
  - 回收结果
  - 超时与隔离
- 用户包中包含可执行文件 `bootstrap`，由其自行解析事件并执行业务逻辑。

### 建议协议（本地 HTTP 或 vsock）

- `GET /runtime/invocation/next`：拉取下一条事件
- `POST /runtime/invocation/{request_id}/response`：返回结果
- `POST /runtime/invocation/{request_id}/error`：返回错误

### 兼容策略

- 保留官方 runtime（python/node/go...）作为“平台托管模板”。
- 新增 `runtime=custom` 或 `runtime=provided`，启用 bootstrap 模式。
- 通过层（layer）或镜像模板提供常见 bootstrap 脚手架。

### 安全边界

- bootstrap 仍运行在 microVM 隔离内。
- 限制只读代码盘、受控环境变量注入、网络策略与 syscall 白名单保持不变。

### 验收标准

- 平台在不改 agent 代码的前提下，成功运行 Bash/C++/Nim 等非内置语言样例。
- 文档化协议并提供最小 demo（10 行级别 bootstrap 示例）。

---

## 3) 并发控制与扩缩容策略（Concurrency & Scaling）

### 现状

当前并发策略偏向“一请求一实例”或简单池化，资源利用率与时延弹性仍有限。

### 方案

#### A. Scale-to-Zero

- 函数在空闲窗口（例如 5~15 分钟）后自动回收实例与快照缓存（可配）。
- 新请求触发快速恢复（优先快照）。

#### B. Instance Concurrency

- **Docker 模式**：允许单实例多并发（适合 I/O 密集型场景）。
- **MicroVM 模式**：默认单实例单请求，保持最强隔离与可预测性能。

#### C. 扩缩容控制面

- 指标驱动：QPS、排队时延、CPU/内存水位。
- 决策参数：`min_replicas`、`max_replicas`、`target_inflight`、`cooldown`。
- 抖动抑制：采用滞后窗口与步进扩缩，防止频繁抖动。

### 验收标准

- 空闲自动归零生效，且首次请求可在目标冷启动预算内返回。
- Docker 模式下多并发吞吐明显提升，MicroVM 模式保持隔离不回退。
- 扩缩容事件全链路可观测（metrics + structured logs）。

---

## 实施里程碑（建议）

1. **M1：快照 PoC**
   - 跑通 init→snapshot→restore 主流程与基准测试。
2. **M2：Custom Runtime Alpha**
   - 发布 bootstrap 协议与 `runtime=custom`，提供官方示例。
3. **M3：扩缩容 Beta**
   - 完成 scale-to-zero + docker 并发开关 + 指标面板。
4. **M4：稳定性与回归**
   - 异常恢复、压测、故障注入、观测体系完善。

## 风险与回滚

- 快照机制异常时，自动降级为常规启动路径。
- Custom Runtime 协议变更需版本化（`/runtime/v1/...`）。
- 并发策略默认保守，按函数逐步灰度放量。
