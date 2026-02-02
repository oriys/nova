# Nova 项目 Review 与开发指南

本文件定义了 Nova 项目的代码质量、性能优化及架构设计的核心准则。

---

## 1. Golang 代码 Review 指南（通用）

### A. 正确性与可维护性 (Blocker / Major)
*   **业务语义：** 确保代码逻辑清晰，命名需表达领域含义（严禁 a/b/c、tmp 等无意义命名）。
*   **领域规则：** 业务逻辑应位于 `internal/domain` 或 `internal/service`，避免侵入 handler、repo 或 util。
*   **错误处理：**
    *   严禁丢失原始 err，使用 `fmt.Errorf("...: %w", err)`。
    *   使用 `errors.Is`/`As` 识别错误。
    *   除非初始化或不可恢复，严禁使用 `panic` 控制流程。
*   **边界条件：** 统一 nil/空 slice/map 的处理逻辑（尤其是 JSON 序列化）。
*   **并发安全：**
    *   注意 map 的并发读写冲突。
    *   确保 goroutine 生命周期可控，防止泄漏。
    *   使用 `go test -race` 检查 data race。

### B. Go 语言惯用法 (Major / Minor)
*   **Context 透传：** 所有 RPC/DB/HTTP 调用必须透传 `context`。`context.Background()` 仅限入口。
*   **接口设计：** 接口应由使用方定义，保持最小方法集。避免过度 mock 接口。
*   **选项模式：** 对于复杂结构体的可选配置，优先使用 Functional Options 模式。
*   **日志规范：** 不要同时 `log` 且 `return err`。日志需带 trace ID 和业务 ID。

### C. 工程化与测试 (Major)
*   **依赖管理：** 严禁引入过重或功能重复的第三方库。
*   **测试覆盖：** 核心逻辑必须有单测，边界逻辑需集成测。避免测试中的 `time.Sleep`。
*   **可观测性：** 关键路径必须包含 metrics、trace 和结构化日志。

---

## 2. 性能 Review 指南

### A. 算法与复杂度 (Blocker / Major)
*   避免热点路径的 O(n²) 逻辑。
*   解决 DB/Redis 的 N+1 查询问题，优先使用 Batch 操作。

### B. 内存与分配 (Major)
*   **预分配：** 循环内 append 必须预估容量 `make([]T, 0, n)`，map 同理。
*   **减少复制：** 频繁字符串拼接使用 `strings.Builder`。
*   **对象复用：** 对于高频大对象，考虑使用 `sync.Pool`。

### C. 并发与锁 (Major)
*   减小锁粒度，读多写少场景优先使用 `RWMutex`。
*   控制 goroutine 数量，使用 worker pool 或 `errgroup`。

### D. I/O 与调用 (Blocker / Major)
*   **超时：** 所有外部调用必须设置超时。
*   **重试：** 重试需具备上限、退避（backoff）和抖动（jitter）。
*   **复用：** 确保 HTTP client 和连接池被正确复用。

---

## 3. 整洁架构 (Clean Architecture) 指南

### A. 依赖方向 (Blocker)
*   **Domain 层：** 绝对不准依赖数据库、框架、日志实现等外部组件。
*   **依赖倒置：** 外层认识内层，内层通过接口定义契约。

### B. 领域建模 (Major)
*   **聚合根：** 确保聚合边界清晰，不变量由聚合内部维护。
*   **隔离：** 跨域交互通过领域事件或防腐层 (ACL) 进行。

### C. 事务与一致性 (Blocker / Major)
*   明确事务边界，严禁在事务内执行耗时外部调用（如 HTTP）。
*   重要事件投递优先考虑 Outbox 模式。

### D. 读写分离 (Major)
*   DTO 仅限边界层使用，不应穿透至 Domain 层。
*   允许为查询需求建立特定的读模型，避免污染核心领域模型。

---

## 结论分级标准
1.  **Blocker**: 线上错误、数据一致性风险、安全问题、不可回滚。
2.  **Major**: 维护成本高、隐患大、性能显著风险。
3.  **Minor / Nit**: 风格建议、可读性改进、一致性微调。
