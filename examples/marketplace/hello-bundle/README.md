# Hello World Bundle

This is a simple example bundle containing a single Python function.

## Installation

Using Orbit CLI:
```bash
orbit store install hello-world --version 1.0.0 --namespace my-namespace
```

Using HTTP API:
```bash
curl -X POST http://localhost:9000/store/installations \
  -H "Content-Type: application/json" \
  -d '{
    "app_slug": "hello-world",
    "version": "1.0.0",
    "install_name": "my-hello",
    "namespace": "default"
  }'
```

## Testing

After installation, invoke the function:
```bash
curl -X POST http://localhost:9000/functions/hello/invoke \
  -H "Content-Type: application/json" \
  -d '{"name": "Nova"}'
```

Expected response:
```json
{
  "output": {"message": "Hello, Nova!"},
  "duration_ms": 50,
  "cold_start": true
}
```
