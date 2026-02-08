import { DocsShell } from "@/components/docs/docs-shell"
import { EndpointSpecCard, type EndpointSpec } from "@/components/docs/api-spec"

const specs: EndpointSpec[] = [
  {
    id: "health",
    title: "Health (Detailed)",
    method: "GET",
    path: "/health",
    summary: "Return aggregate service health with component-level status.",
    auth: "Typically public, depending on auth public_paths configuration.",
    responseFields: [
      { name: "status", type: "ok|degraded", required: true, description: "Overall service health." },
      { name: "components.postgres", type: "boolean", required: true, description: "Postgres connectivity." },
      { name: "components.pool.active_vms", type: "number", required: true, description: "Active VM count." },
      { name: "components.pool.total_pools", type: "number", required: true, description: "Function pool count." },
    ],
    successCodes: [{ code: 200, meaning: "Health payload returned" }],
    errorCodes: [],
    responseExample: `{
  "status": "ok",
  "components": {
    "postgres": true,
    "pool": {
      "active_vms": 2,
      "total_pools": 4
    }
  }
}`,
  },
  {
    id: "health-probes",
    title: "Health Probes",
    method: "GET",
    path: "/health/live | /health/ready | /health/startup",
    summary: "Kubernetes-style probe endpoints for liveness, readiness, and startup checks.",
    auth: "Usually configured as public paths.",
    responseFields: [
      { name: "status", type: "string", required: true, description: "Probe status value." },
      { name: "error", type: "string", required: false, description: "Failure reason for non-ready states." },
    ],
    successCodes: [
      { code: 200, meaning: "Probe passed" },
      { code: 503, meaning: "Readiness/startup not yet satisfied" },
    ],
    errorCodes: [],
    notes: [
      "/health/live checks process liveness.",
      "/health/ready and /health/startup include Postgres checks.",
    ],
    responseExample: `{
  "status": "ready"
}`,
    curlExample: `curl -s http://localhost:9000/health/live
curl -s http://localhost:9000/health/ready
curl -s http://localhost:9000/health/startup`,
  },
  {
    id: "global-timeseries",
    title: "Global Metrics Time Series",
    method: "GET",
    path: "/metrics/timeseries",
    summary: "Bucketed global invocation/error/latency metrics for dashboards.",
    auth: "Public or authenticated based on deployment policy.",
    queryFields: [
      { name: "range", type: "1h|6h|24h|7d|30d", required: false, description: "Window selector." },
    ],
    responseFields: [
      { name: "[]timestamp", type: "string(RFC3339)", required: true, description: "Bucket start time." },
      { name: "[]invocations", type: "number", required: true, description: "Invocation count." },
      { name: "[]errors", type: "number", required: true, description: "Error count." },
      { name: "[]avg_duration", type: "number", required: true, description: "Average duration in ms." },
    ],
    successCodes: [{ code: 200, meaning: "OK" }],
    errorCodes: [{ code: 500, meaning: "Metrics query failure" }],
    responseExample: `[
  {
    "timestamp": "2026-02-08T16:00:00Z",
    "invocations": 44,
    "errors": 2,
    "avg_duration": 18.6
  },
  {
    "timestamp": "2026-02-08T16:05:00Z",
    "invocations": 39,
    "errors": 1,
    "avg_duration": 16.3
  }
]`,
    orbitExample: "orbit metrics global --range 1h",
  },
  {
    id: "function-logs",
    title: "Function Logs",
    method: "GET",
    path: "/functions/{name}/logs",
    summary: "Read recent logs for a function or retrieve by request_id.",
    auth: "Same auth model as function APIs.",
    pathFields: [{ name: "name", type: "string", required: true, description: "Function name." }],
    queryFields: [
      { name: "tail", type: "number", required: false, description: "Last N entries (default 10)." },
      { name: "request_id", type: "string", required: false, description: "Exact request log lookup." },
    ],
    responseFields: [
      { name: "[]request_id", type: "string", required: true, description: "Invocation request ID." },
      { name: "[]status", type: "string", required: true, description: "Invocation result status." },
      { name: "[]duration_ms", type: "number", required: true, description: "Execution duration." },
      { name: "[]error", type: "string", required: false, description: "Error detail." },
      { name: "[]created_at", type: "string(RFC3339)", required: true, description: "Log entry time." },
    ],
    successCodes: [{ code: 200, meaning: "OK" }],
    errorCodes: [
      { code: 404, meaning: "Function or request log not found" },
      { code: 500, meaning: "Log query failure" },
    ],
    responseExample: `[
  {
    "request_id": "req_123",
    "status": "succeeded",
    "duration_ms": 12,
    "created_at": "2026-02-08T16:11:00Z"
  }
]`,
    orbitExample: "orbit functions logs hello-python --tail 20",
  },
  {
    id: "list-invocations",
    title: "List Recent Invocations",
    method: "GET",
    path: "/invocations",
    summary: "List recent invocation logs across functions in active scope.",
    auth: "Same as function APIs.",
    queryFields: [
      { name: "limit", type: "number", required: false, description: "Result cap (max 500)." },
    ],
    responseFields: [
      { name: "[]function_name", type: "string", required: true, description: "Function name." },
      { name: "[]request_id", type: "string", required: true, description: "Request ID." },
      { name: "[]duration_ms", type: "number", required: true, description: "Invocation duration." },
      { name: "[]cold_start", type: "boolean", required: true, description: "Cold start marker." },
      { name: "[]created_at", type: "string(RFC3339)", required: true, description: "Invocation time." },
    ],
    successCodes: [{ code: 200, meaning: "OK" }],
    errorCodes: [{ code: 500, meaning: "Query failure" }],
    responseExample: `[
  {
    "function_name": "hello-python",
    "request_id": "req_123",
    "duration_ms": 12,
    "cold_start": false,
    "created_at": "2026-02-08T16:11:00Z"
  }
]`,
    orbitExample: "orbit invocations --limit 100",
  },
]

export default function DocsAPIOperationsPage() {
  return (
    <DocsShell
      current="api"
      activeHref="/docs/api/operations"
      title="Operations API"
      description="Operational observability and probe endpoints for health, metrics, logs, and recent invocations."
      toc={[
        { id: "coverage", label: "Coverage" },
        { id: "health", label: "Health" },
        { id: "health-probes", label: "Health Probes" },
        { id: "global-timeseries", label: "Global Timeseries" },
        { id: "function-logs", label: "Function Logs" },
        { id: "list-invocations", label: "Recent Invocations" },
      ]}
    >
      <section id="coverage" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Coverage</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          This section documents runtime health and observability endpoints used by dashboards and SRE workflows.
        </p>
      </section>

      {specs.map((spec) => (
        <EndpointSpecCard key={spec.id} spec={spec} />
      ))}
    </DocsShell>
  )
}
