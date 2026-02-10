# Backend Split: Nova / Comet / Zenith

后端已经拆成三个可独立运行的子项目（同仓库内多服务）：

- `nova`：控制平面（函数管理、配置、租户、工作流、网关路由管理）
- `comet`：数据平面（执行器、池化、异步队列），仅提供 gRPC 给网关调用
- `zenith`：统一网关入口，供 UI / MCP / CLI 调用

## 目录与入口

- 控制平面：`cmd/nova`
- 数据平面：`cmd/comet`
- 网关：`cmd/zenith`

## 构建

```bash
make build
```

会生成：

- `bin/nova`
- `bin/comet`
- `bin/zenith`

## 运行

1. 启动 Nova（控制平面）

```bash
./bin/nova daemon --http :8081 --config configs/nova.json
```

2. 启动 Comet（数据平面，仅 gRPC）

```bash
./bin/comet daemon --grpc :9090 --config configs/nova.json
```

3. 启动 Zenith（统一入口）

```bash
./bin/zenith serve \
  --listen :8080 \
  --nova-url http://127.0.0.1:8081 \
  --comet-grpc 127.0.0.1:9090
```

## 请求路由规则（Zenith）

- `POST /functions/{name}/invoke`：走 gRPC 调用 Comet
- 数据面路径（如 `invoke-async` / logs / metrics / heatmap / invocations / async-invocations）：通过 gRPC `ProxyHTTP` 转发到 Comet 内部数据面处理器
- 其余管理类路径：反向代理到 Nova HTTP

## Docker 启动（3 容器）

`docker-compose.yml` 已配置为三服务独立容器启动：

- `nova` 使用 `Dockerfile` 的 `nova-runtime` target
- `comet` 使用 `Dockerfile` 的 `comet-runtime` target
- `zenith` 使用 `Dockerfile` 的 `zenith-runtime` target

## 默认接入地址

- Lumen 默认通过 `BACKEND_URL`（compose 中已设为 `http://zenith:9000`）访问 Zenith
- Atlas 使用 `ZENITH_URL`，默认 `http://localhost:9000`
- Orbit 使用 `--server` 或 `ZENITH_URL`

## 健康检查

Zenith 提供统一健康探针：

- `GET /health/live`
- `GET /health/startup`
- `GET /health`
- `GET /health/ready`

`/health` 和 `/health/ready` 会同时检查 Nova 与 Comet 的可用性。
