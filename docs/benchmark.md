# Nova Function Performance Benchmark

> **Date**: 2026-02-27  
> **Platform**: Docker Compose (macOS host → linux/amd64 containers)  
> **Services**: Zenith (gateway :9000) → Comet (gRPC :9090), Nova (control :9001), Aurora (:9002)  
> **Method**: Sequential & concurrent HTTP invocations via `POST /functions/{name}/invoke`  
> **Warmup**: 2 calls discarded per function, then 15 measured iterations

---

## 1. Latency by Runtime

### Server-Side Execution Time

Measured inside Comet (excludes network/gateway overhead). Sorted by warm average.

| # | Runtime | Cold Start | Warm Avg | Warm P50 | Warm P95 | Min | Max |
|---|---------|-----------|----------|----------|----------|-----|-----|
| 1 | **Rust** | 191 ms | **3.7 ms** | 4 ms | 4 ms | 3 ms | 4 ms |
| 2 | **C** | 157 ms | **5.1 ms** | 5 ms | 6 ms | 5 ms | 6 ms |
| 3 | **C++** | 189 ms | **7.5 ms** | 7 ms | 8 ms | 7 ms | 8 ms |
| 4 | **Bun** | 509 ms | **7.6 ms** | 7 ms | 10 ms | 7 ms | 10 ms |
| 5 | **PHP** | 630 ms | **8.3 ms** | 8 ms | 13 ms | 6 ms | 13 ms |
| 6 | **Python** | 454 ms | **9.4 ms** | 9 ms | 13 ms | 8 ms | 13 ms |
| 7 | **Lua** | 401 ms | **10.0 ms** | 7 ms | 47 ms | 3 ms | 47 ms |
| 8 | **Node.js** | 516 ms | **10.7 ms** | 10 ms | 13 ms | 9 ms | 13 ms |
| 9 | **Deno** | 2,446 ms | **16.9 ms** | 16 ms | 23 ms | 13 ms | 23 ms |
| 10 | **Kotlin** | 183 ms | **21.3 ms** | 21 ms | 24 ms | 19 ms | 24 ms |
| 11 | **Ruby** | 267 ms | **24.5 ms** | 23 ms | 38 ms | 21 ms | 38 ms |
| 12 | **Java** | 471 ms | **26.6 ms** | 23 ms | 44 ms | 17 ms | 44 ms |
| 13 | **Go** | 186 ms | **30.9 ms** | 19 ms | 103 ms | 16 ms | 103 ms |
| 14 | **GraalVM** | 1,235 ms | **50.0 ms** | 46 ms | 81 ms | 44 ms | 81 ms |
| 15 | **Scala** | 291 ms | **157.9 ms** | 122 ms | 411 ms | 71 ms | 411 ms |

### Wall Clock Time (includes gateway + network)

| # | Runtime | Cold Wall | Warm Avg | Warm P50 | Warm P95 |
|---|---------|----------|----------|----------|----------|
| 1 | **Rust** | 208 ms | **13.1 ms** | 13.0 ms | 15.7 ms |
| 2 | **C** | 170 ms | **14.1 ms** | 14.0 ms | 17.6 ms |
| 3 | **C++** | 203 ms | **16.9 ms** | 16.2 ms | 23.4 ms |
| 4 | **Bun** | 520 ms | **16.5 ms** | 16.2 ms | 19.3 ms |
| 5 | **PHP** | 642 ms | **17.8 ms** | 17.1 ms | 26.3 ms |
| 6 | **Python** | 490 ms | **18.3 ms** | 17.8 ms | 25.3 ms |
| 7 | **Lua** | 423 ms | **40.7 ms** | 33.9 ms | 141.0 ms |
| 8 | **Node.js** | 525 ms | **19.7 ms** | 18.7 ms | 25.0 ms |
| 9 | **Deno** | 2,459 ms | **26.9 ms** | 26.1 ms | 37.8 ms |
| 10 | **Kotlin** | 198 ms | **35.4 ms** | 36.4 ms | 40.9 ms |
| 11 | **Ruby** | 277 ms | **35.7 ms** | 34.4 ms | 48.1 ms |
| 12 | **Java** | 489 ms | **44.7 ms** | 40.6 ms | 71.6 ms |
| 13 | **Go** | 202 ms | **51.4 ms** | 32.7 ms | 207.7 ms |
| 14 | **GraalVM** | 1,298 ms | **71.0 ms** | 67.9 ms | 103.8 ms |
| 15 | **Scala** | 305 ms | **176.2 ms** | 135.6 ms | 432.2 ms |

### Gateway Overhead

Difference between wall clock and server-side execution (Zenith routing + HTTP serialization).

| Runtime | Overhead |
|---------|----------|
| Rust | ~9 ms |
| C | ~9 ms |
| C++ | ~9 ms |
| Bun | ~9 ms |
| PHP | ~10 ms |
| Python | ~9 ms |
| Node.js | ~9 ms |
| Deno | ~10 ms |
| Kotlin | ~14 ms |
| Ruby | ~11 ms |
| Java | ~18 ms |
| Go | ~20 ms |
| GraalVM | ~21 ms |
| Scala | ~18 ms |
| Lua | ~31 ms |

**Average gateway overhead: ~13 ms** (mostly consistent across runtimes).

---

## 2. Cold Start Analysis

Cold start = first invocation of a function (container/process spin-up).

| Runtime | Cold Start (server) | Cold Start (wall) | Category |
|---------|--------------------|--------------------|----------|
| **C** | 157 ms | 170 ms | ⚡ Fast |
| **Kotlin** | 183 ms | 198 ms | ⚡ Fast |
| **Go** | 186 ms | 202 ms | ⚡ Fast |
| **C++** | 189 ms | 203 ms | ⚡ Fast |
| **Rust** | 191 ms | 208 ms | ⚡ Fast |
| **Ruby** | 267 ms | 277 ms | 🟡 Medium |
| **Scala** | 291 ms | 305 ms | 🟡 Medium |
| **Lua** | 401 ms | 423 ms | 🟡 Medium |
| **Python** | 454 ms | 490 ms | 🟡 Medium |
| **Java** | 471 ms | 489 ms | 🟡 Medium |
| **Bun** | 509 ms | 520 ms | 🟡 Medium |
| **Node.js** | 516 ms | 525 ms | 🟡 Medium |
| **PHP** | 630 ms | 642 ms | 🟠 Slow |
| **GraalVM** | 1,235 ms | 1,298 ms | 🔴 Very Slow |
| **Deno** | 2,446 ms | 2,459 ms | 🔴 Very Slow |

**Key insight**: Compiled binaries (C, Go, Rust, C++, Kotlin) cold-start under 200ms. Deno has the slowest cold start at ~2.4s due to its runtime initialization.

---

## 3. Throughput Tests

### Sequential Throughput (single caller, 50 calls)

| Runtime | Success | Requests/sec | Avg Latency | Total Time |
|---------|---------|-------------|-------------|------------|
| **Node.js** | 50/50 | **48.3 req/s** | 10.4 ms | 1.0 s |
| **Python** | 50/50 | **46.5 req/s** | 11.8 ms | 1.1 s |
| **Go** | 50/50 | **35.6 req/s** | 15.7 ms | 1.4 s |

### Concurrent Throughput (hello-python, 30 calls)

| Concurrency | Success | Requests/sec | Wall Avg | Wall P95 | Server Avg |
|-------------|---------|-------------|----------|----------|------------|
| 1 | 30/30 | **49.5 req/s** | 19.6 ms | 20.1 ms | 10.2 ms |
| 5 | 30/30 | **38.2 req/s** | 113.2 ms | 775.3 ms | 103.5 ms |
| 10 | 30/30 | **96.2 req/s** | 72.9 ms | 276.0 ms | 58.9 ms |
| 20 | 30/30 | **39.7 req/s** | 236.1 ms | 716.5 ms | 219.5 ms |

### Cross-Runtime Concurrent (10 runtimes, 10 concurrent)

| Runtime | Calls | Wall Avg |
|---------|-------|----------|
| C++ | 3 | 33.2 ms |
| Rust | 3 | 34.9 ms |
| C | 3 | 37.3 ms |
| Bun | 3 | 41.5 ms |
| PHP | 3 | 44.5 ms |
| Node.js | 3 | 49.3 ms |
| Python | 3 | 53.4 ms |
| Java | 3 | 55.9 ms |
| Deno | 3 | 115.6 ms |
| Ruby | 3 | 138.8 ms |

**Total: 30/30 ok, 86.7 req/s across 10 runtimes concurrently.**

### Sustained Load (hello-node, 100 calls, concurrency=10)

| Metric | Wall Clock | Server-Side |
|--------|-----------|-------------|
| **Avg** | 91.9 ms | 75.1 ms |
| **P50** | 38.1 ms | 20.0 ms |
| **P95** | 677.0 ms | 659.0 ms |
| **P99** | 823.7 ms | 802.0 ms |
| **Throughput** | **106.7 req/s** | — |
| **Success Rate** | 100/100 (100%) | — |

---

## 4. Payload Size Impact

Tested with Python echo function (returns input as-is).

| Payload Size | Avg Latency |
|-------------|-------------|
| 100 bytes | 9.6 ms |
| 1 KB | 9.2 ms |
| 10 KB | 9.4 ms |
| 50 KB | 11.4 ms |

**Conclusion**: Payload size has negligible impact up to 50 KB (~2 ms increase).

---

## 5. Runtime Tier Summary

Based on warm execution performance:

### 🏆 Tier 1 — Sub-10ms (Compiled Native)

| Runtime | Warm Avg | Best For |
|---------|----------|----------|
| Rust | 3.7 ms | Systems programming, high-perf compute |
| C | 5.1 ms | Low-level processing, algorithms |
| C++ | 7.5 ms | Compute-intensive tasks |
| Bun | 7.6 ms | Fast JS/TS execution |
| PHP | 8.3 ms | Web/string processing |

### 🥈 Tier 2 — 10-25ms (Interpreted/Lightweight)

| Runtime | Warm Avg | Best For |
|---------|----------|----------|
| Python | 9.4 ms | Data science, ML, general scripting |
| Lua | 10.0 ms | Lightweight scripting, embedding |
| Node.js | 10.7 ms | I/O-heavy tasks, npm ecosystem |
| Deno | 16.9 ms | Secure TypeScript execution |
| Kotlin | 21.3 ms | JVM with modern syntax |
| Ruby | 24.5 ms | Web tasks, text processing |

### 🥉 Tier 3 — 25ms+ (JVM / Heavy Runtimes)

| Runtime | Warm Avg | Best For |
|---------|----------|----------|
| Java | 26.6 ms | Enterprise workloads |
| Go | 30.9 ms | Concurrent/network tasks |
| GraalVM | 50.0 ms | Polyglot, native-image potential |
| Scala | 157.9 ms | Functional programming, Spark |

---

## 6. Key Findings

1. **Rust is the fastest runtime** at 3.7ms warm average — 42x faster than Scala.
2. **Gateway overhead is consistent** at ~9-20ms regardless of runtime, suggesting the Zenith proxy is well-optimized.
3. **Cold starts vary dramatically**: C at 157ms vs Deno at 2,446ms (15x difference).
4. **Bun outperforms Node.js** in both warm execution (7.6ms vs 10.7ms) and is competitive with compiled languages.
5. **Payload size has minimal impact** — 50KB payloads add only ~2ms over 100-byte payloads.
6. **Concurrency sweet spot is ~10 threads** — achieved 96-107 req/s. Higher concurrency (20) shows diminishing returns due to container resource contention.
7. **100% success rate** across all tests — zero failed invocations under load.
8. **Scala is an outlier** with high variance (71-411ms) likely due to JVM warmup and garbage collection.

---

## Appendix: Test Environment

```
Host:       macOS (Apple Silicon, Docker Desktop)
Containers: linux/amd64 (emulated)
Docker:     Docker Compose with 7 services
Gateway:    Zenith on port 9000
Executor:   Comet (gRPC, Docker backend)
Database:   PostgreSQL 15
Functions:  159 deployed across 15 runtimes
Test Tool:  Python 3 + curl (sequential & concurrent.futures)
```

> **Note**: Performance on native Linux with KVM/Firecracker backend would be significantly faster due to elimination of Docker Desktop emulation overhead and near-native VM execution.
