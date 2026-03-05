# Nova 无服务器平台 — 技术架构审核报告

**审核日期**: 2026-03-04  
**审核范围**: Nova 全系统（Zenith 网关 / Nova 控制面 / Comet 执行面 / Corona 调度 / Nebula 事件总线 / Aurora 可观测 / Lumen 前端 / Agent 客户端）

---

## 第一阶段：资产盘点与上下文对齐

### 1.1 项目背景

Nova 是一个自研的轻量级无服务器（Serverless/FaaS）平台，以 Firecracker 微虚拟机为核心隔离单元，支持 20+ 语言运行时。项目定位为**私有化部署的 Lambda 替代方案**，面向需要函数级隔离但不想依赖公有云的团队。

**当前阶段判断**: 项目处于"活下来 → 抗住流量"的过渡期。核心隔离执行链路已相当成熟（多后端抽象、快照恢复、自适应弹性伸缩），但生产化所需的安全加固、多实例高可用、运维可操作性仍有显著缺口。

### 1.2 技术栈全景

| 层次 | 选型 |
|------|------|
| 语言 | Go 1.24 (CGO_ENABLED=0) |
| 数据库 | PostgreSQL 16 (pgx/v5 + pgxpool) |
| 缓存 | 本地 sync.Map（60s TTL），可选 Redis |
| 消息 | 数据库优先（Outbox/Inbox 模式），可选 Kafka/RabbitMQ/Redis Streams |
| 通信 | 内部 gRPC，外部 HTTP（Zenith 网关统一入口） |
| 隔离 | Firecracker / Docker / Kata / Libkrun / WASM / K8s / Apple VZ |
| 前端 | Next.js 16 + shadcn/ui + i18n |
| 追踪 | OpenTelemetry (OTLP-HTTP) + W3C propagation |
| 指标 | 自研 time-series + Prometheus 双通道 |
| CI/CD | Makefile 驱动；无正式测试 / Lint 流水线 |

### 1.3 系统上下文图（逻辑视图）

```
                    ┌──────────────┐
                    │  Lumen (UI)  │ ──── Next.js :3000
                    └──────┬───────┘
                           │ HTTP
                    ┌──────▼───────┐
 外部用户/CLI ─────▶│   Zenith     │ ──── 唯一 HTTP 入口 :9000
                    │  (Gateway)   │
                    └──┬───┬───┬───┘
            gRPC       │   │   │
         ┌─────────────┘   │   └─────────────┐
         ▼                 ▼                  ▼
   ┌──────────┐     ┌──────────┐       ┌──────────┐
   │  Nova    │     │  Comet   │       │  Aurora   │
   │ 控制面   │     │ 执行面   │       │ 可观测   │
   │  :9001   │     │  :9090   │       │  :9002   │
   └────┬─────┘     └────┬─────┘       └──────────┘
        │                 │
        ▼                 ▼
   ┌─────────┐     ┌──────────────────┐
   │Postgres │◄────│  VM Pool         │
   │         │     │ (Firecracker/    │
   └─────────┘     │  Docker/K8s/...) │
                   └──────┬───────────┘
                          │ vsock / TCP
                   ┌──────▼───────┐
                   │    Agent     │
                   │  (Guest VM)  │
                   └──────────────┘
   
   辅助服务:
   ┌──────────┐     ┌──────────┐
   │ Corona   │     │ Nebula   │
   │ 调度器   │     │ 事件总线 │
   │  :9003   │     │  :9004   │
   └──────────┘     └──────────┘
```

### 1.4 核心调用链路（函数调用时序）

```
Client → Zenith(HTTP) → Comet(gRPC) → Executor
  → Pool.Acquire() → [cold: Firecracker.CreateVM | warm: 取空闲 VM]
    → vsock 4B-len-prefix + JSON → Agent.Exec(handler, input)
    → Agent 返回结果
  → Pool.Release(VM)
  → 写 Metrics / InvocationLog (异步)
← Zenith 返回 HTTP Response
```

---

## 第二阶段：核心架构"切片"审查

### 2.1 应用架构层：服务拆分与交互

#### ✅ 亮点

- **五面体架构清晰**：控制面 (Nova)、执行面 (Comet)、调度 (Corona)、事件 (Nebula)、可观测 (Aurora) 职责边界明确，Zenith 作为唯一 HTTP 入口实现了流量归一。
- **多后端抽象优秀**：`Backend` 接口 + `Router` 工厂模式 + 自动检测 (`detect.go`)，支持 7 种隔离后端无缝切换。
- **Pool Key 设计巧妙**：配置相同的函数共享同一 VM 池（按 runtime/memory/env hash），减少资源浪费。
- **Singleflight 去重冷启动**：并发请求只触发一次 VM 创建，有效避免惊群效应。
- **Transactional Outbox + Inbox**：事件总线使用标准 Outbox 模式确保消息不丢，Inbox 实现消费幂等。

#### ⚠️ 问题

| # | 问题 | 严重度 | 详情 |
|---|------|--------|------|
| A1 | **同步调用链中未配置端到端 Timeout** | P1 | Zenith→Comet gRPC 调用依赖 `X-Nova-Timeout-S` header 注入 context timeout，但无全局最大超时兜底。如果客户端不传 timeout，理论上请求可无限等待。 |
| A2 | **同步 Invoke 端点不支持幂等** | P1 | `POST /functions/{name}/invoke` 同步调用无 `Idempotency-Key` 支持（仅异步调用有），网络抖动重试可能导致重复执行。 |
| A3 | **API 缺少版本控制** | P2 | 无 Accept-Version / URL 前缀版本化，破坏性变更将难以平滑迁移。 |
| A4 | **输入校验不足** | P2 | 函数名无正则限制、payload 无 schema 验证、代码大小无 handler 级限制（仅网关 10MB body limit）。 |

---

### 2.2 数据架构层：存储与流转

#### ✅ 亮点

- **Schema 设计规范**：16+ 核心表，复合索引覆盖高频查询，外键 ON DELETE CASCADE 防止孤儿记录。
- **Advisory Lock 保护关键操作**：Schema 迁移用 `pg_advisory_xact_lock(0x6e6f7661)`，删除操作用独立锁键，防止并发冲突。
- **60s TTL 本地缓存**：`CachedMetadataStore` 用 sync.Map + 写穿透失效，热路径查询效率高。

#### ⚠️ 问题

| # | 问题 | 严重度 | 详情 |
|---|------|--------|------|
| D1 | **无正式迁移框架** | P1 | Schema 变更嵌入 Go 代码 (`ensureSchema()` 每次启动执行)，依赖 `IF NOT EXISTS` 幂等性。无版本追踪、无回滚能力。一旦迁移失败，需人工介入。 |
| D2 | **多实例缓存一致性窗口 60s** | P2 | 在多 Nova/Comet 实例部署下，一个实例更新函数配置后，其他实例最长 60s 内仍使用旧缓存。高频更新场景可能导致行为不一致。 |
| D3 | **连接池未显式配置** | P2 | pgxpool 使用默认配置（max = 4×CPU），在多租户高并发场景下可能不足。DSN 中无 `pool_max_conns` 参数。 |
| D4 | **异步任务状态竞争** | P1 | `async_invocations` 表的状态更新无乐观锁（版本号/CAS），多 worker 可能竞争同一任务导致状态覆盖。虽然有 lease-based 锁，但 lease 过期后仍存在窗口。 |

---

### 2.3 部署与物理架构层：隔离与容灾

#### ✅ 亮点

- **多层资源隔离**：Firecracker VM 级别的 CPU/内存/磁盘 IOPS/网络带宽限制全部通过 Firecracker API 原生支持。
- **网络命名空间隔离**：支持 netns 模式下的严格出站/入站 iptables 规则（strict / egress-only 策略）。
- **资源池化**：CID（4096）和 IP 地址使用 O(1) LIFO 栈分配/回收。

#### ⚠️ 问题

| # | 问题 | 严重度 | 详情 |
|---|------|--------|------|
| I1 | **Zenith 网关是全系统单点故障 (SPOF)** | **P0** | 单实例 Zenith，无内建负载均衡，每个后端服务仅一个 gRPC 连接。Zenith 宕机 = 全系统不可用。无 TLS、无 keepalive、无连接池。 |
| I2 | **无 IP/CID 资源回收机制** | P1 | 如果 VM 在 monitor goroutine 清理前崩溃（极端情况），IP 和 CID 泄漏直到进程重启。无后台扫描回收器。IP 池仅 ~252 地址，泄漏几十个即耗尽。 |
| I3 | **gRPC 连接无 TLS 加密** | **P0** | Zenith→所有后端服务全部使用 `insecure.NewCredentials()`。内网流量明文传输，含 service token、用户 payload、密文数据。 |
| I4 | **集群无共识机制** | P1 | `internal/cluster/` 基于心跳 + 内存 registry，无分布式一致性（etcd/Raft）。多实例部署存在脑裂风险，节点状态可能不一致。 |
| I5 | **快照 code drive 累积** | P2 | `PreserveCodeDrive=true` 标记在快照场景下阻止代码盘删除，持续累积。无自动清理策略。 |

---

## 第三阶段：非功能性 (NFR) 压力探测

### 3.1 稳定性

**直击灵魂之问："如果 Comet 执行面变慢了 10 倍，其他服务几分钟内会崩？"**

| 维度 | 现状 | 风险评估 |
|------|------|----------|
| **熔断** | ✅ 有。Per-function 滑动窗口熔断器，支持 Closed→Open→HalfOpen 三态，可配置错误率阈值。 | 低风险 |
| **超时** | ⚠️ 部分。函数执行有 `TimeoutS`，但 Zenith→Comet gRPC 无全局超时兜底，客户端不传则无限等待。 | **高风险** |
| **重试** | ⚠️ 部分。Gateway 层支持 per-route 重试（最多 5 次 + 退避），但 Zenith→后端 gRPC 无重试策略。 | 中风险 |
| **降级** | ❌ 无。Zenith 收到 gRPC Unavailable 直接返回 503，无降级缓存或备用响应。 | 高风险 |
| **背压/过载保护** | ✅ 有。Pool 层有 `ErrConcurrencyLimit` / `ErrQueueFull` / `ErrQueueWaitTimeout` / `ErrGlobalVMLimit`，函数级队列深度可配置。 | 低风险 |
| **优雅关闭** | ✅ 有。Executor 设置 `closing` flag → 等待 in-flight drain → 关闭 pool（10s 超时并行停止所有 VM）。 | 低风险 |

### 3.2 安全性

**直击灵魂之问："如果一个普通用户篡改了 API Key 中的 tenant_id，他能操作别人的函数吗？"**

| 维度 | 现状 | 风险评估 |
|------|------|----------|
| **认证** | ✅ JWT (HS256/RS256) + API Key (SHA256 hash 存储，constant-time 比较)。 | 低风险 |
| **授权** | ⚠️ RBAC + DENY-first 策略，**但默认无策略时回退到 `RoleViewer`（有读权限）**。违反最小权限原则。 | **高风险** |
| **租户隔离** | ⚠️ 查询级 `WHERE tenant_id = $X` 强制隔离。**但 API Key/JWT 无 AllowedScopes 时默认不限制**，可跨租户访问。 | **致命风险** |
| **密钥管理** | ⚠️ AES-256-GCM 加密存储。**但 Master Key 硬编码在 docker-compose.yml 和 configs/，无轮换机制。** | **致命风险** |
| **传输加密** | ❌ 所有内部 gRPC 使用 insecure credentials，HTTP 无 TLS。 | **致命风险** |
| **幂等防重放** | ⚠️ 幂等性模块为**纯内存**，服务重启后丢失。无法真正保证 exactly-once。 | 高风险 |

### 3.3 可观测性

**直击灵魂之问："如果今晚函数调用成功率从 99.9% 掉到 80%，你们需要多久定位到根因？"**

| 维度 | 现状 | 风险评估 |
|------|------|----------|
| **分布式追踪** | ✅ OpenTelemetry + W3C traceparent 贯穿 HTTP→gRPC→vsock。可串联到 VM 内部。 | 低风险 |
| **指标** | ✅ 双通道：自研 JSON (轻量 Dashboard) + Prometheus（运维级）。23+ 指标覆盖调用量、延迟、冷启动、VM 生命周期。 | 低风险 |
| **SLO 自动评估** | ✅ 窗口级 SLO 评估（成功率 / P95 / 冷启动率），支持 webhook/Slack 告警 + 自动扩容自愈。 | 低风险 |
| **结构化日志** | ✅ slog + JSON/text handler，注入 traceID/spanID。 | 低风险 |
| **审计日志** | ❌ 无。认证/授权决策无审计记录，安全事件不可追溯。 | 高风险 |

**评价**: 可观测性是本项目做得最好的维度之一。指标、追踪、日志三支柱齐全，SLO 自动评估 + 自愈是亮眼设计。主要缺口在安全审计。

### 3.4 扩展性

**直击灵魂之问："如果下个月流量翻十倍，哪些组件会先爆？"**

| 维度 | 现状 | 风险评估 |
|------|------|----------|
| **无状态横向扩容** | ⚠️ Zenith/Nova/Comet 理论上可多实例部署，但 Zenith 无负载均衡、集群无共识协议。 | 高风险 |
| **数据库** | ⚠️ 单 PostgreSQL 实例，无读写分离、无分库分表规划。`invocation_logs` 表随调用量线性增长，无自动归档。 | 高风险 |
| **VM 资源** | ⚠️ IP 池仅 252 地址（/24 子网），CID 池 4096。单节点 VM 上限受此约束。 | 中风险 |
| **自适应伸缩** | ✅ AIMD 自适应队列 + 反应式/预测式弹性伸缩 + 季节性模型。设计前瞻。 | 低风险 |
| **配置** | ⚠️ 大量配置有合理默认值，但部分关键参数（pool size、连接池、超时）未暴露为可配置项。 | 中风险 |

---

## 第四阶段：评审结论与行动计划

### P0 / 致命风险 — 必须立即修复

| 编号 | 问题 | 影响 | 建议措施 |
|------|------|------|----------|
| **S1** | **Master Key 硬编码在版本控制中** | 所有加密密钥实质暴露。任何能读仓库的人可解密全部 secrets。 | ① 立即从 docker-compose.yml / configs/ 移除 key；② 引入 `.env` + Docker secrets / Vault；③ 轮换现有 key + 重加密所有 secret。 |
| **S2** | **租户隔离默认不限制** | API Key/JWT 无 AllowedScopes 时可跨租户操作，等同于权限逃逸。 | ① `auth.go` 默认 scope 改为空集（deny-all）；② 强制所有 identity 必须绑定明确的 tenant/namespace。 |
| **S3** | **内部通信无 TLS** | gRPC 全链路明文，含 service token + 用户数据。内网嗅探即可获取全部数据。 | ① Zenith→backend 启用 mTLS；② gRPC server 启用 TLS（已有 `--grpc-cert/key` flag，但客户端未对接）。 |
| **I1** | **Zenith 网关单点故障** | 全系统唯一 HTTP 入口，宕机 = 全部不可用。 | ① 部署至少 2 个 Zenith 实例；② 前置 LB（nginx/HAProxy/K8s Ingress）；③ gRPC 客户端启用 keepalive + round-robin。 |

### P1 / 高风险技术债务 — 随业务增长必爆

| 编号 | 问题 | 建议措施 |
|------|------|----------|
| **S4** | 默认授权回退到 `RoleViewer` | 改为 deny-all + 强制绑定角色。 |
| **S5** | 幂等性模块纯内存 | 迁移到 PostgreSQL-backed 存储，重启后状态不丢失。 |
| **S6** | 无密钥轮换机制 | Master Key 引入版本化 envelope encryption，支持不停机轮换。 |
| **A1** | Zenith→Comet 无全局超时兜底 | 在 Zenith 侧注入全局最大超时（如 300s），防止无限等待。 |
| **D1** | 无正式数据库迁移框架 | 引入 golang-migrate 或 Atlas，带版本追踪和回滚。 |
| **D4** | 异步任务状态无乐观锁 | `async_invocations` 增加 `version` 列，UPDATE 时带 WHERE version 条件。 |
| **I2** | IP/CID 无资源回收 | 增加后台扫描线程，定期比对 pool 内活跃 VM 与资源池，回收孤儿资源。 |
| **I4** | 集群无共识机制 | 引入 etcd / Raft 或至少 PostgreSQL-based leader election。 |
| **A2** | 同步 Invoke 无幂等支持 | 扩展 `Idempotency-Key` 到同步调用端点。 |

### P2 / 优化建议

| 编号 | 问题 | 建议措施 |
|------|------|----------|
| **D2** | 多实例缓存一致性 60s 窗口 | 引入 Redis pub/sub 或 PostgreSQL LISTEN/NOTIFY 做缓存失效广播。 |
| **D3** | 连接池未显式配置 | DSN 中增加 `pool_max_conns=20&pool_min_conns=5` 等参数，暴露为配置项。 |
| **A3** | 无 API 版本控制 | 在路由中增加 `/v1/` 前缀或 Accept-Version header 支持。 |
| **A4** | 输入校验不足 | 函数名增加正则校验、payload 增加大小限制、runtime 做枚举验证。 |
| **I5** | 快照 code drive 累积 | 增加定期清理策略，删除超过 N 天未使用的快照 code drive。 |
| **O1** | 无安全审计日志 | 认证/授权/密钥操作增加结构化审计日志，输出到独立 sink。 |
| **E1** | 数据库无归档策略 | `invocation_logs` 增加 TTL + 分区表或定期归档到冷存储。 |
| **E2** | 单 PostgreSQL 无读写分离 | 规划 streaming replication + 只读副本用于查询密集操作（logs/metrics）。 |

### 妥协声明 (Trade-offs)

以下是当前架构**有意识的取舍**，在现阶段业务规模下合理，但需要业务方知悉：

| 取舍 | 牺牲了什么 | 换取了什么 | 何时需要重新评估 |
|------|-----------|-----------|----------------|
| **数据库优先，不依赖外部消息队列** | 吞吐量上限受 DB poll 频率限制（~500ms 默认，Redis 通知可降至亚毫秒） | 零外部依赖，部署极简 | 异步任务 TPS > 1000 或事件延迟要求 < 100ms |
| **本地 sync.Map 缓存而非 Redis** | 多实例间存在 60s 一致性窗口 | 无 Redis 运维成本 | 多实例部署 + 频繁配置变更 |
| **无正式 CI/CD 测试套件** | 回归风险高，重构成本大 | 快速迭代 | 团队扩大到 3+ 人或开始接入生产流量 |
| **手写 JWT 解析而非标准库** | 更大的攻击面、更高的维护成本 | 零依赖 | 建议尽早替换为 `golang-jwt/jwt` |
| **per-node autoscaler 而非中央调度** | 全局最优资源分配受限 | 无需中央调度器 | 集群节点 > 3 |

---

## 附录：架构优秀实践清单

以下是审核过程中发现的**值得保留和推广**的设计：

1. **Singleflight 冷启动去重** — 并发请求仅触发一次 VM 创建，显著降低冷启动开销
2. **三级 VM 生命周期** (Active→Idle→Suspended→Destroyed) — 优雅的资源回收策略
3. **Health Check Loop** — 30s 间隔 ping 空闲 VM，自动驱逐无响应实例
4. **AIMD 自适应并发控制** — 类 TCP 拥塞控制算法动态调整 worker 数、batch size、poll interval
5. **DENY-first 授权评估** — DENY 策略优先于 ALLOW，防止特权提升
6. **Transactional Outbox + Inbox** — 事件不丢 + 消费幂等的经典模式
7. **DAG Workflow 引擎** — Kahn 拓扑排序验证 + dependency_count 就绪检测
8. **SLO 自动自愈** — 延迟/冷启动 SLO 违约自动触发 min_replicas 扩容
9. **Vsock 重试 + 重连** — 3 次指数退避重试，断连自动重拨 + 重初始化
10. **多后端 Backend 接口** — 7 种隔离后端统一抽象，运行时自动检测最优选项

---

*本报告基于代码静态分析生成，未包含负载测试数据。建议在修复 P0 问题后，补充性能基准测试验证。*
