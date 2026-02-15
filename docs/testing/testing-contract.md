# Nova Testing Contract

This document defines the testing boundaries, consistency model, idempotency strategy,
and acceptance criteria for the Nova serverless platform.

## 1. Golden Path

The canonical request flow through Nova is:

```
Client Request
  → Zenith (API Gateway, :9000)
    → Nova (Control Plane, :9001)  — Function CRUD, auth, tenant scoping
    → Comet (Data Plane, :9090)    — gRPC invocation, VM/container lifecycle
    → Domain (internal/domain)     — Validation, business rules
    → Store  (internal/store)      — Postgres persistence (functions, events, logs)
    → Outbox (event_outbox table)  — Transactional event publishing
    → Nebula (Event Bus)           — Outbox relay, async delivery, DLQ
    → Downstream (subscriptions)   — Function/workflow invocation via event delivery
```

## 2. Consistency Model

| Boundary | Model | Guarantee |
|---|---|---|
| Function CRUD (Nova → Postgres) | **Strong** | Single-writer, serializable transactions |
| Event publishing (Outbox) | **Strong** | Same-transaction insert: `event_messages` + `event_deliveries` |
| Event delivery (Nebula → functions) | **Eventual** | At-least-once with inbox dedup per (subscription, message) |
| Async invocations | **Eventual** | Lease-based acquisition, retry with exponential backoff |
| Cache (internal/cache) | **Eventual** | TTL-based expiry, invalidated on write operations |

## 3. Idempotency Strategy

| Layer | Mechanism | Storage |
|---|---|---|
| Async invocations | `idempotency_key` column + `EnqueueAsyncInvocationWithIdempotency` | `async_invocations` table, UNIQUE constraint |
| Event publishing | `source_outbox_id` dedup in `PublishEventFromOutbox` | `event_outbox` table |
| Event delivery | `PrepareEventInbox` per (subscription_id, message_id) | `event_inbox_records` table |
| API requests | `request_id` / `X-Request-ID` header propagation | `invocation_logs.id` |

## 4. Acceptance Criteria

### P0 — Must pass before merge

- [ ] **Idempotency**: Duplicate requests with the same idempotency key return consistent results, no duplicate writes
- [ ] **Tenant isolation**: Tenant A cannot read or write Tenant B's resources (functions, secrets, events)
- [ ] **Auth enforcement**: Requests without valid credentials are rejected; role-based access is enforced
- [ ] **Domain validation**: Invalid function names, handlers, memory/timeout ranges are rejected
- [ ] **Outbox reliability**: DB write + outbox insert occur in the same transaction

### P1 — Must pass before release

- [ ] **Concurrent writes**: N goroutines writing the same resource produce exactly 1 success (unique constraint)
- [ ] **Event delivery**: Published events are delivered at-least-once to all active subscriptions
- [ ] **Retry/backoff**: Failed deliveries are retried with exponential backoff; exhausted retries go to DLQ
- [ ] **Cache coherence**: Write operations invalidate cached entries; stale reads do not persist beyond TTL

### P2 — Monitored continuously

- [ ] **Observability**: trace_id, structured logs, and Prometheus metrics are present on all invocations
- [ ] **Timeout/circuit breaker**: Downstream failures trigger circuit breaker; requests respect deadline
- [ ] **Rate limiting**: Per-route and per-tenant rate limits are enforced

## 5. Risk Points

| Risk | Impact | Mitigation |
|---|---|---|
| Outbox relay failure | Events silently lost | Outbox status tracking (pending/sent/failed), DLQ, monitoring |
| Duplicate event delivery | Double execution | Inbox dedup table per (subscription, message) |
| Concurrent function create | Duplicate functions | UNIQUE constraint on `functions.name` |
| Stale cache after write | Serve outdated data | Cache invalidation on Save/Update/Delete + TTL expiry |
| Lease expiry during execution | Duplicate async invocation | Lease renewal, idempotent handlers |
| Cross-tenant data leak | Security breach | Tenant-scoped queries, middleware enforcement, scope tests |

## 6. Test Layers

```
┌─────────────────────────────────────────────────┐
│  E2E (3-5 golden paths)         — make test-e2e │
├─────────────────────────────────────────────────┤
│  Integration (DB + services)    — make test-int  │
├─────────────────────────────────────────────────┤
│  Unit (domain + pure logic)     — make test-unit │
└─────────────────────────────────────────────────┘
```

### Unit tests (`go test -short ./internal/...`)
- Domain model validation (function, permission, workflow, gateway)
- Service layer input validation
- Cache hit/miss/invalidation logic
- Circuit breaker state transitions
- Cost calculation
- Rate limit token bucket

### Integration tests (`go test -run Integration ./internal/...`)
- Store operations against real Postgres
- Outbox + event delivery pipeline
- Async invocation queue with concurrent workers
- Tenant isolation at the query level

### E2E tests
- Create → Invoke → Query logs (full CRUD lifecycle)
- Publish event → Deliver to subscriber
- Concurrent invoke with rate limiting
