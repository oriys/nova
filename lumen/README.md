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

## Internationalization (i18n)

Lumen supports multiple languages via [next-intl](https://next-intl.dev/). The default language is **English**.

### Supported Languages

| Code    | Language           |
|---------|--------------------|
| `en`    | English (default)  |
| `zh-CN` | 简体中文           |
| `zh-TW` | 繁體中文           |
| `ja`    | 日本語             |
| `fr`    | Français           |

### How it works

- Translation files are in `messages/` (`en.json`, `zh-CN.json`, etc.)
- Locale is determined by the `NEXT_LOCALE` cookie, or the browser `Accept-Language` header
- Users can switch languages via the globe icon in the header toolbar
- The i18n configuration lives in `i18n/config.ts` (shared constants) and `i18n/request.ts` (server-side locale resolution)

### Adding a new language

1. Create a new JSON file in `messages/` (e.g. `ko.json`) following the structure of `en.json`
2. Add the locale code to the `locales` array and `localeNames` map in `i18n/config.ts`

## API Endpoints

The frontend uses the following backend APIs:

### Control Plane
- `GET /functions` - list functions
- `POST /functions` - create function
- `GET /functions/{name}` - get function details
- `PATCH /functions/{name}` - update function
- `DELETE /functions/{name}` - delete function
- `GET /runtimes` - list available runtimes
- `GET /api-keys` - list API keys
- `POST /api-keys` - create API key
- `DELETE /api-keys/{id}` - delete API key
- `GET /secrets` - list secrets
- `POST /secrets` - create secret
- `DELETE /secrets/{name}` - delete secret
- `GET /workflows` - list workflows
- `POST /workflows` - create workflow
- `GET /workflows/{name}` - get workflow details
- `DELETE /workflows/{name}` - delete workflow
- `POST /workflows/{name}/trigger` - trigger workflow run
- `GET /events/topics` - list event topics
- `GET /gateway/routes` - list gateway routes

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

- Next.js 16
- React 19
- Tailwind CSS 4
- Recharts (charts)
- shadcn/ui (component library)
- next-intl (internationalization)
- Monaco Editor (code editing)
- XYFlow (workflow visualization)
