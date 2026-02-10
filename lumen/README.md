# lumen - frontend for nova

Lumen is the web management console for Nova.

## Development

### Prerequisites

Make sure the `zenith` gateway is running:

```bash
# from the nova project root (recommended 3-service setup)
./bin/nova daemon --http :8081
./bin/comet daemon --grpc :9090
./bin/zenith serve --listen :9000 --nova-url http://127.0.0.1:8081 --comet-grpc 127.0.0.1:9090
```

The frontend uses `localhost:9000` (Zenith) as its backend entrypoint by default.

### Start the dev server

```bash
cd lumen
npm install
npm run dev
```

The frontend listens on `http://localhost:3000` and proxies `/api/*` requests automatically.

## API Endpoints

The frontend uses the following backend APIs:

### Control Plane
- `GET /functions` - list functions
- `POST /functions` - create function
- `GET /functions/{name}` - get function details
- `PATCH /functions/{name}` - update function
- `DELETE /functions/{name}` - delete function
- `GET /runtimes` - list available runtimes

### Data Plane
- `POST /functions/{name}/invoke` - invoke function
- `GET /functions/{name}/logs` - get function logs
- `GET /functions/{name}/metrics` - get function metrics
- `GET /metrics` - global metrics
- `GET /health` - health check

### Snapshots
- `GET /snapshots` - list snapshots
- `POST /functions/{name}/snapshot` - create snapshot
- `DELETE /functions/{name}/snapshot` - delete snapshot

## Tech Stack

- Next.js 15
- React 19
- Tailwind CSS
- Recharts (charts)
- shadcn/ui (component library)
