# Lumen - Nova Frontend

Nova 的 Web 管理界面。

## 开发

### 前提条件

确保 Nova 后端正在运行：

```bash
# 在 nova 根目录
./bin/nova daemon
```

后端默认监听 `localhost:9000`。

### 启动开发服务器

```bash
cd lumen
npm install
npm run dev
```

前端默认监听 `http://localhost:3000`，会自动代理 `/api/*` 请求到后端。

## API 端点

前端使用以下后端 API：

### Control Plane
- `GET /functions` - 列出所有函数
- `POST /functions` - 创建函数
- `GET /functions/{name}` - 获取函数详情
- `PATCH /functions/{name}` - 更新函数
- `DELETE /functions/{name}` - 删除函数
- `GET /runtimes` - 获取可用运行时

### Data Plane
- `POST /functions/{name}/invoke` - 调用函数
- `GET /functions/{name}/logs` - 获取函数日志
- `GET /functions/{name}/metrics` - 获取函数指标
- `GET /metrics` - 全局指标
- `GET /health` - 健康检查

### Snapshots
- `GET /snapshots` - 列出快照
- `POST /functions/{name}/snapshot` - 创建快照
- `DELETE /functions/{name}/snapshot` - 删除快照

## 技术栈

- Next.js 15
- React 19
- Tailwind CSS
- Recharts (图表)
- shadcn/ui (组件库)
