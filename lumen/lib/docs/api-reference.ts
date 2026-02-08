import type { EndpointSpec } from "@/components/docs/api-spec"

export type ApiDomainKey = "functions" | "workflows" | "events" | "operations"

export interface ApiEndpointDoc {
  slug: string
  spec: EndpointSpec
}

export interface ApiDomainDoc {
  key: ApiDomainKey
  title: string
  description: string
  coverage: string
  endpoints: ApiEndpointDoc[]
}

const functionEndpoints: ApiEndpointDoc[] = [
  {
    slug: "create-function",
    spec: {
      id: "create-function",
      title: "Create Function",
      method: "POST",
      path: "/functions",
      summary: "Register function metadata and source code in the active tenant/namespace.",
      auth: "API key/JWT if auth is enabled; tenant scope headers recommended.",
      requestFields: [
        { name: "name", type: "string", required: true, description: "Unique function name." },
        { name: "runtime", type: "string", required: true, description: "Runtime ID (python, node, go, ...)." },
        { name: "code", type: "string", required: true, description: "Function source code." },
        { name: "handler", type: "string", required: false, description: "Handler name; default main.handler." },
        { name: "memory_mb", type: "number", required: false, description: "Memory in MB; default 128." },
        { name: "timeout_s", type: "number", required: false, description: "Timeout in seconds; default 30." },
        { name: "min_replicas", type: "number", required: false, description: "Warm minimum replicas." },
        { name: "max_replicas", type: "number", required: false, description: "Replica cap (0 = unlimited)." },
        { name: "instance_concurrency", type: "number", required: false, description: "Per-instance in-flight cap." },
        { name: "mode", type: "process|persistent", required: false, description: "Execution mode." },
        { name: "env_vars", type: "object<string,string>", required: false, description: "Environment variable map." },
        { name: "limits", type: "object", required: false, description: "Resource limits (vcpus, disk/network constraints)." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Function ID." },
        { name: "name", type: "string", required: true, description: "Function name." },
        { name: "runtime", type: "string", required: true, description: "Runtime ID." },
        { name: "handler", type: "string", required: true, description: "Resolved handler." },
        { name: "compile_status", type: "string", required: true, description: "Code compile status." },
        { name: "created_at", type: "string(RFC3339)", required: true, description: "Creation time." },
        { name: "updated_at", type: "string(RFC3339)", required: true, description: "Last update time." },
      ],
      successCodes: [{ code: 201, meaning: "Created" }],
      errorCodes: [
        { code: 400, meaning: "Invalid or missing required field" },
        { code: 409, meaning: "Name conflict" },
        { code: 500, meaning: "Store/compiler failure" },
      ],
      requestExample: `{
  "name": "hello-python",
  "runtime": "python",
  "handler": "handler",
  "code": "def handler(event, context):\\n    return {'ok': True, 'echo': event}",
  "memory_mb": 256,
  "timeout_s": 30,
  "min_replicas": 1,
  "max_replicas": 5,
  "instance_concurrency": 10
}`,
      responseExample: `{
  "id": "fn_01",
  "name": "hello-python",
  "runtime": "python",
  "handler": "handler",
  "compile_status": "not_required",
  "created_at": "2026-02-08T12:00:00Z",
  "updated_at": "2026-02-08T12:00:00Z"
}`,
      orbitExample: `orbit functions create \\
  --name hello-python \\
  --runtime python \\
  --handler handler \\
  --code 'def handler(event, context):\\n    return {"ok": True}' \\
  --memory 256 \\
  --timeout 30`,
    },
  },
  {
    slug: "list-functions",
    spec: {
      id: "list-functions",
      title: "List Functions",
      method: "GET",
      path: "/functions",
      summary: "List functions in current tenant/namespace scope.",
      auth: "Same as create function.",
      queryFields: [
        { name: "search", type: "string", required: false, description: "Name substring filter." },
        { name: "limit", type: "number", required: false, description: "Max results." },
      ],
      responseFields: [
        { name: "[]id", type: "string", required: true, description: "Function ID." },
        { name: "[]name", type: "string", required: true, description: "Function name." },
        { name: "[]runtime", type: "string", required: true, description: "Runtime." },
        { name: "[]version", type: "number", required: true, description: "Current version." },
      ],
      successCodes: [{ code: 200, meaning: "OK" }],
      errorCodes: [{ code: 500, meaning: "Store query failure" }],
      responseExample: `[
  {
    "id": "fn_01",
    "name": "hello-python",
    "runtime": "python",
    "version": 1
  }
]`,
      orbitExample: "orbit functions list --search hello --limit 20",
    },
  },
  {
    slug: "update-function",
    spec: {
      id: "update-function",
      title: "Update Function",
      method: "PATCH",
      path: "/functions/{name}",
      summary: "Patch mutable config and/or inline source code.",
      auth: "Same as create function.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Function name." }],
      requestFields: [
        { name: "handler", type: "string", required: false, description: "Handler override." },
        { name: "memory_mb", type: "number", required: false, description: "Memory override." },
        { name: "timeout_s", type: "number", required: false, description: "Timeout override." },
        { name: "min_replicas", type: "number", required: false, description: "Min replica override." },
        { name: "max_replicas", type: "number", required: false, description: "Max replica override." },
        { name: "instance_concurrency", type: "number", required: false, description: "In-flight per instance." },
        { name: "mode", type: "process|persistent", required: false, description: "Execution mode." },
        { name: "env_vars", type: "object<string,string>", required: false, description: "Environment map replacement." },
        { name: "limits", type: "object", required: false, description: "Resource limits replacement." },
        { name: "code", type: "string", required: false, description: "Inline source update." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Function ID." },
        { name: "name", type: "string", required: true, description: "Function name." },
        { name: "updated_at", type: "string(RFC3339)", required: true, description: "Update timestamp." },
      ],
      successCodes: [{ code: 200, meaning: "Updated" }],
      errorCodes: [
        { code: 404, meaning: "Function not found" },
        { code: 500, meaning: "Update failure" },
      ],
      requestExample: `{
  "memory_mb": 512,
  "timeout_s": 60,
  "max_replicas": 20,
  "code": "def handler(event, context):\\n    return {'updated': True}"
}`,
      responseExample: `{
  "id": "fn_01",
  "name": "hello-python",
  "memory_mb": 512,
  "timeout_s": 60,
  "updated_at": "2026-02-08T13:00:00Z"
}`,
      orbitExample: `orbit functions update hello-python \\
  --memory 512 \\
  --timeout 60 \\
  --max-replicas 20`,
    },
  },
  {
    slug: "invoke-sync",
    spec: {
      id: "invoke-sync",
      title: "Invoke Function (Sync)",
      method: "POST",
      path: "/functions/{name}/invoke",
      summary: "Execute function synchronously and return output inline.",
      auth: "Same as create function.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Function name." }],
      requestFields: [
        { name: "(body)", type: "any JSON", required: false, description: "Invocation payload. Defaults to {}." },
      ],
      responseFields: [
        { name: "request_id", type: "string", required: true, description: "Invocation request ID." },
        { name: "output", type: "any JSON", required: true, description: "Function output." },
        { name: "error", type: "string", required: false, description: "Execution error if failed." },
        { name: "duration_ms", type: "number", required: true, description: "Duration in milliseconds." },
        { name: "cold_start", type: "boolean", required: true, description: "Cold start marker." },
      ],
      successCodes: [{ code: 200, meaning: "Executed" }],
      errorCodes: [
        { code: 400, meaning: "Invalid JSON payload" },
        { code: 404, meaning: "Function not found" },
        { code: 429, meaning: "Queue/inflight shed by policy" },
        { code: 503, meaning: "Concurrency or capacity rejection" },
        { code: 504, meaning: "Execution timeout" },
      ],
      responseExample: `{
  "request_id": "req_123",
  "output": {
    "ok": true
  },
  "duration_ms": 14,
  "cold_start": false
}`,
      orbitExample: "orbit functions invoke hello-python --payload '{\"trace\":\"docs\"}'",
    },
  },
  {
    slug: "invoke-async",
    spec: {
      id: "invoke-async",
      title: "Invoke Function (Async)",
      method: "POST",
      path: "/functions/{name}/invoke-async",
      summary: "Queue invocation job with retry/backoff and optional idempotency key.",
      auth: "Same as create function.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Function name." }],
      requestFields: [
        { name: "payload", type: "any JSON", required: false, description: "Invocation payload." },
        { name: "max_attempts", type: "number", required: false, description: "Retry max attempts." },
        { name: "backoff_base_ms", type: "number", required: false, description: "Base retry delay." },
        { name: "backoff_max_ms", type: "number", required: false, description: "Max retry delay." },
        { name: "idempotency_key", type: "string", required: false, description: "Deduplication key." },
        { name: "idempotency_ttl_s", type: "number", required: false, description: "Deduplication TTL seconds." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Async invocation ID." },
        { name: "status", type: "queued|running|succeeded|dlq", required: true, description: "Current state." },
        { name: "attempt", type: "number", required: true, description: "Current attempt." },
        { name: "max_attempts", type: "number", required: true, description: "Retry cap." },
        { name: "next_run_at", type: "string(RFC3339)", required: true, description: "Next scheduled run." },
        { name: "last_error", type: "string", required: false, description: "Last failure message." },
      ],
      successCodes: [
        { code: 202, meaning: "Queued" },
        { code: 200, meaning: "Idempotency hit, existing invocation returned" },
      ],
      errorCodes: [
        { code: 400, meaning: "Invalid payload or idempotency key" },
        { code: 404, meaning: "Function not found" },
        { code: 429, meaning: "Tenant async queue quota exceeded" },
        { code: 500, meaning: "Queue/store failure" },
      ],
      requestExample: `{
  "payload": { "user_id": "u-1" },
  "max_attempts": 5,
  "backoff_base_ms": 1000,
  "backoff_max_ms": 60000,
  "idempotency_key": "task-u-1-001",
  "idempotency_ttl_s": 3600
}`,
      responseExample: `{
  "id": "job_456",
  "function_name": "hello-python",
  "status": "queued",
  "attempt": 0,
  "max_attempts": 5,
  "next_run_at": "2026-02-08T12:00:05Z"
}`,
      orbitExample: "orbit functions invoke-async hello-python --payload '{\"user_id\":\"u-1\"}' --max-attempts 5",
    },
  },
]

const workflowEndpoints: ApiEndpointDoc[] = [
  {
    slug: "create-workflow",
    spec: {
      id: "create-workflow",
      title: "Create Workflow",
      method: "POST",
      path: "/workflows",
      summary: "Create workflow metadata record (name/description).",
      auth: "API key/JWT if enabled; tenant scope headers recommended.",
      requestFields: [
        { name: "name", type: "string", required: true, description: "Workflow unique name." },
        { name: "description", type: "string", required: false, description: "Human-readable description." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Workflow ID." },
        { name: "name", type: "string", required: true, description: "Workflow name." },
        { name: "description", type: "string", required: false, description: "Description." },
        { name: "status", type: "active|inactive|deleted", required: true, description: "Workflow status." },
        { name: "current_version", type: "number", required: true, description: "Published version pointer." },
        { name: "created_at", type: "string(RFC3339)", required: true, description: "Creation time." },
      ],
      successCodes: [{ code: 201, meaning: "Created" }],
      errorCodes: [
        { code: 400, meaning: "Invalid JSON or missing name" },
        { code: 500, meaning: "Service/store failure" },
      ],
      requestExample: `{
  "name": "order-pipeline",
  "description": "Order processing workflow"
}`,
      responseExample: `{
  "id": "wf_001",
  "name": "order-pipeline",
  "description": "Order processing workflow",
  "status": "active",
  "current_version": 0,
  "created_at": "2026-02-08T14:00:00Z"
}`,
      orbitExample: "orbit workflows create --name order-pipeline --description 'Order processing workflow'",
    },
  },
  {
    slug: "publish-workflow-version",
    spec: {
      id: "publish-workflow-version",
      title: "Publish Workflow Version",
      method: "POST",
      path: "/workflows/{name}/versions",
      summary: "Publish immutable workflow DAG definition version.",
      auth: "Same as create workflow.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Workflow name." }],
      requestFields: [
        { name: "nodes", type: "array", required: true, description: "Node definitions." },
        { name: "nodes[].node_key", type: "string", required: true, description: "Logical node key." },
        { name: "nodes[].function_name", type: "string", required: true, description: "Target function name." },
        { name: "nodes[].input_mapping", type: "object", required: false, description: "Input mapping template." },
        { name: "nodes[].retry_policy", type: "object", required: false, description: "Retry policy for node." },
        { name: "nodes[].timeout_s", type: "number", required: false, description: "Node timeout in seconds." },
        { name: "edges", type: "array", required: true, description: "Directed edges." },
        { name: "edges[].from", type: "string", required: true, description: "Upstream node_key." },
        { name: "edges[].to", type: "string", required: true, description: "Downstream node_key." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Workflow version ID." },
        { name: "workflow_id", type: "string", required: true, description: "Workflow ID." },
        { name: "version", type: "number", required: true, description: "Published version number." },
        { name: "created_at", type: "string(RFC3339)", required: true, description: "Publish timestamp." },
      ],
      successCodes: [{ code: 201, meaning: "Version published" }],
      errorCodes: [
        { code: 400, meaning: "Invalid DAG definition" },
        { code: 404, meaning: "Workflow not found" },
        { code: 500, meaning: "Publish failure" },
      ],
      requestExample: `{
  "nodes": [
    {
      "node_key": "validate",
      "function_name": "validate-order",
      "retry_policy": {
        "max_attempts": 3,
        "base_ms": 1000,
        "max_backoff_ms": 10000
      }
    },
    {
      "node_key": "charge",
      "function_name": "charge-order"
    }
  ],
  "edges": [
    {
      "from": "validate",
      "to": "charge"
    }
  ]
}`,
      responseExample: `{
  "id": "wfv_001",
  "workflow_id": "wf_001",
  "version": 1,
  "created_at": "2026-02-08T14:05:00Z"
}`,
      orbitExample: "orbit workflows versions publish order-pipeline --definition-file ./workflow.json",
    },
  },
  {
    slug: "trigger-workflow-run",
    spec: {
      id: "trigger-workflow-run",
      title: "Trigger Workflow Run",
      method: "POST",
      path: "/workflows/{name}/runs",
      summary: "Create one workflow run from current published version.",
      auth: "Same as create workflow.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Workflow name." }],
      requestFields: [
        { name: "input", type: "any JSON", required: true, description: "Workflow input payload. Use {} if empty." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Run ID." },
        { name: "workflow_name", type: "string", required: true, description: "Workflow name." },
        { name: "status", type: "pending|running|succeeded|failed|cancelled", required: true, description: "Run status." },
        { name: "trigger_type", type: "string", required: true, description: "Trigger source (api/schedule/event)." },
        { name: "input", type: "any JSON", required: false, description: "Run input payload." },
        { name: "started_at", type: "string(RFC3339)", required: true, description: "Start timestamp." },
      ],
      successCodes: [{ code: 201, meaning: "Run created" }],
      errorCodes: [
        { code: 400, meaning: "Invalid JSON body" },
        { code: 404, meaning: "Workflow not found" },
        { code: 500, meaning: "Run creation failure" },
      ],
      notes: [
        "Current implementation expects a JSON body. Send {} when you have no input.",
      ],
      requestExample: `{
  "input": {
    "order_id": "o-1001",
    "amount": 88.5
  }
}`,
      responseExample: `{
  "id": "run_001",
  "workflow_name": "order-pipeline",
  "status": "running",
  "trigger_type": "api",
  "started_at": "2026-02-08T14:10:00Z"
}`,
      orbitExample: "orbit workflows run order-pipeline --input '{\"order_id\":\"o-1001\"}'",
    },
  },
  {
    slug: "list-workflow-runs",
    spec: {
      id: "list-workflow-runs",
      title: "List Workflow Runs",
      method: "GET",
      path: "/workflows/{name}/runs",
      summary: "List runs for a workflow.",
      auth: "Same as create workflow.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Workflow name." }],
      responseFields: [
        { name: "[]id", type: "string", required: true, description: "Run ID." },
        { name: "[]status", type: "string", required: true, description: "Run status." },
        { name: "[]trigger_type", type: "string", required: true, description: "Trigger type." },
        { name: "[]started_at", type: "string(RFC3339)", required: true, description: "Start time." },
        { name: "[]finished_at", type: "string(RFC3339)", required: false, description: "Finish time." },
      ],
      successCodes: [{ code: 200, meaning: "OK" }],
      errorCodes: [{ code: 500, meaning: "Query failure" }],
      responseExample: `[
  {
    "id": "run_001",
    "status": "succeeded",
    "trigger_type": "api",
    "started_at": "2026-02-08T14:10:00Z",
    "finished_at": "2026-02-08T14:10:03Z"
  }
]`,
      orbitExample: "orbit workflows runs list order-pipeline",
    },
  },
  {
    slug: "get-workflow-run",
    spec: {
      id: "get-workflow-run",
      title: "Get Workflow Run",
      method: "GET",
      path: "/workflows/{name}/runs/{runID}",
      summary: "Read one run and (optionally) node-level execution details.",
      auth: "Same as create workflow.",
      pathFields: [
        { name: "name", type: "string", required: true, description: "Workflow name." },
        { name: "runID", type: "string", required: true, description: "Run ID." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Run ID." },
        { name: "workflow_name", type: "string", required: true, description: "Workflow name." },
        { name: "status", type: "string", required: true, description: "Run status." },
        { name: "output", type: "any JSON", required: false, description: "Run output when completed." },
        { name: "error_message", type: "string", required: false, description: "Failure reason." },
        { name: "nodes", type: "array", required: false, description: "Node execution states on detailed queries." },
      ],
      successCodes: [{ code: 200, meaning: "OK" }],
      errorCodes: [
        { code: 404, meaning: "Run not found (or workflow mismatch)" },
        { code: 500, meaning: "Query failure" },
      ],
      responseExample: `{
  "id": "run_001",
  "workflow_name": "order-pipeline",
  "status": "succeeded",
  "output": {
    "order_id": "o-1001",
    "status": "completed"
  },
  "started_at": "2026-02-08T14:10:00Z",
  "finished_at": "2026-02-08T14:10:03Z"
}`,
      orbitExample: "orbit workflows runs get order-pipeline run_001",
    },
  },
]

const eventEndpoints: ApiEndpointDoc[] = [
  {
    slug: "create-topic",
    spec: {
      id: "create-topic",
      title: "Create Topic",
      method: "POST",
      path: "/topics",
      summary: "Create an event topic for publish/subscribe delivery.",
      auth: "API key/JWT if enabled; tenant scope headers recommended.",
      requestFields: [
        { name: "name", type: "string", required: true, description: "Topic name." },
        { name: "description", type: "string", required: false, description: "Topic description." },
        { name: "retention_hours", type: "number", required: false, description: "Retention window; default 168." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Topic ID." },
        { name: "name", type: "string", required: true, description: "Topic name." },
        { name: "retention_hours", type: "number", required: true, description: "Retention hours." },
        { name: "created_at", type: "string(RFC3339)", required: true, description: "Creation timestamp." },
      ],
      successCodes: [{ code: 201, meaning: "Created" }],
      errorCodes: [
        { code: 400, meaning: "Validation failure" },
        { code: 409, meaning: "Topic already exists" },
      ],
      requestExample: `{
  "name": "orders",
  "description": "Order domain events",
  "retention_hours": 168
}`,
      responseExample: `{
  "id": "topic_001",
  "name": "orders",
  "retention_hours": 168,
  "created_at": "2026-02-08T15:00:00Z"
}`,
      orbitExample: "orbit topics create --name orders --description 'Order domain events'",
    },
  },
  {
    slug: "create-subscription",
    spec: {
      id: "create-subscription",
      title: "Create Subscription",
      method: "POST",
      path: "/topics/{name}/subscriptions",
      summary: "Create function or workflow subscription target with retry/flow controls.",
      auth: "Same as create topic.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Topic name." }],
      requestFields: [
        { name: "name", type: "string", required: true, description: "Subscription name." },
        { name: "consumer_group", type: "string", required: false, description: "Consumer group name." },
        { name: "type", type: "function|workflow", required: false, description: "Target type. Default function." },
        { name: "function_name", type: "string", required: false, description: "Required when type=function." },
        { name: "workflow_name", type: "string", required: false, description: "Required when type=workflow." },
        { name: "enabled", type: "boolean", required: false, description: "Enable immediately." },
        { name: "max_attempts", type: "number", required: false, description: "Retry attempts." },
        { name: "backoff_base_ms", type: "number", required: false, description: "Base retry interval." },
        { name: "backoff_max_ms", type: "number", required: false, description: "Max retry interval." },
        { name: "max_inflight", type: "number", required: false, description: "In-flight cap (0 unlimited)." },
        { name: "rate_limit_per_sec", type: "number", required: false, description: "Per-second cap (0 unlimited)." },
        { name: "webhook_url", type: "string(url)", required: false, description: "Optional workflow result sink." },
        { name: "webhook_method", type: "string", required: false, description: "Webhook method." },
        { name: "webhook_headers", type: "object<string,string>", required: false, description: "Webhook headers." },
        { name: "webhook_timeout_ms", type: "number", required: false, description: "Webhook timeout in ms." },
      ],
      responseFields: [
        { name: "id", type: "string", required: true, description: "Subscription ID." },
        { name: "type", type: "function|workflow", required: true, description: "Stored target type." },
        { name: "function_name", type: "string", required: false, description: "Function target name." },
        { name: "workflow_name", type: "string", required: false, description: "Workflow target name." },
        { name: "webhook_url", type: "string", required: false, description: "Configured webhook URL." },
        { name: "enabled", type: "boolean", required: true, description: "Enabled state." },
        { name: "lag", type: "number", required: true, description: "Delivery lag." },
        { name: "inflight", type: "number", required: true, description: "In-flight delivery count." },
        { name: "queued", type: "number", required: true, description: "Queued delivery count." },
        { name: "dlq", type: "number", required: true, description: "DLQ delivery count." },
      ],
      successCodes: [{ code: 201, meaning: "Created" }],
      errorCodes: [
        { code: 400, meaning: "Validation error" },
        { code: 404, meaning: "Topic/function/workflow not found" },
        { code: 409, meaning: "Subscription conflict" },
        { code: 500, meaning: "Workflow service unavailable" },
      ],
      notes: [
        "Workflow subscription without webhook_url keeps output internal (stored only).",
        "type=function routes to function delivery chain; type=workflow routes to workflow chain.",
      ],
      requestExample: `{
  "name": "sub-orders-workflow",
  "type": "workflow",
  "workflow_name": "order-pipeline",
  "webhook_url": "https://example.com/hook",
  "max_attempts": 3,
  "max_inflight": 0,
  "rate_limit_per_sec": 0
}`,
      responseExample: `{
  "id": "sub_001",
  "name": "sub-orders-workflow",
  "type": "workflow",
  "workflow_name": "order-pipeline",
  "webhook_url": "https://example.com/hook",
  "enabled": true,
  "lag": 0,
  "inflight": 0,
  "queued": 0,
  "dlq": 0
}`,
    },
  },
  {
    slug: "publish-event",
    spec: {
      id: "publish-event",
      title: "Publish Event",
      method: "POST",
      path: "/topics/{name}/publish",
      summary: "Publish one message and fan out to subscription deliveries.",
      auth: "Same as create topic.",
      pathFields: [{ name: "name", type: "string", required: true, description: "Topic name." }],
      requestFields: [
        { name: "payload", type: "any JSON", required: false, description: "Message payload. Defaults to {}." },
        { name: "headers", type: "object", required: false, description: "Message headers." },
        { name: "ordering_key", type: "string", required: false, description: "Ordering key." },
      ],
      responseFields: [
        { name: "message.id", type: "string", required: true, description: "Message ID." },
        { name: "message.sequence", type: "number", required: true, description: "Topic sequence." },
        { name: "message.published_at", type: "string(RFC3339)", required: true, description: "Publish time." },
        { name: "deliveries", type: "number", required: true, description: "Created delivery count." },
      ],
      successCodes: [{ code: 201, meaning: "Published" }],
      errorCodes: [
        { code: 400, meaning: "Invalid payload/ordering key" },
        { code: 404, meaning: "Topic not found" },
        { code: 429, meaning: "Tenant publish quota exceeded" },
        { code: 500, meaning: "Publish failure" },
      ],
      requestExample: `{
  "payload": { "order_id": "o-1001" },
  "headers": { "source": "checkout" },
  "ordering_key": "customer-42"
}`,
      responseExample: `{
  "message": {
    "id": "msg_001",
    "sequence": 101,
    "payload": { "order_id": "o-1001" },
    "published_at": "2026-02-08T15:10:00Z"
  },
  "deliveries": 2
}`,
      orbitExample: "orbit topics publish orders --payload '{\"order_id\":\"o-1001\"}'",
    },
  },
  {
    slug: "list-deliveries",
    spec: {
      id: "list-deliveries",
      title: "List Deliveries",
      method: "GET",
      path: "/subscriptions/{id}/deliveries",
      summary: "Inspect delivery lifecycle state for a subscription.",
      auth: "Same as create topic.",
      pathFields: [{ name: "id", type: "string", required: true, description: "Subscription ID." }],
      queryFields: [
        { name: "limit", type: "number", required: false, description: "Result cap." },
        { name: "status", type: "string(csv)", required: false, description: "queued,running,succeeded,dlq" },
      ],
      responseFields: [
        { name: "[]id", type: "string", required: true, description: "Delivery ID." },
        { name: "[]message_sequence", type: "number", required: true, description: "Topic message sequence." },
        { name: "[]status", type: "string", required: true, description: "Delivery status." },
        { name: "[]attempt", type: "number", required: true, description: "Attempt number." },
        { name: "[]last_error", type: "string", required: false, description: "Last error." },
        { name: "[]updated_at", type: "string(RFC3339)", required: true, description: "Last updated time." },
      ],
      successCodes: [{ code: 200, meaning: "OK" }],
      errorCodes: [
        { code: 400, meaning: "Invalid status filter" },
        { code: 500, meaning: "Query failure" },
      ],
      responseExample: `[
  {
    "id": "del_001",
    "message_sequence": 101,
    "status": "succeeded",
    "attempt": 1,
    "updated_at": "2026-02-08T15:11:00Z"
  }
]`,
    },
  },
  {
    slug: "replay-subscription",
    spec: {
      id: "replay-subscription",
      title: "Replay Subscription",
      method: "POST",
      path: "/subscriptions/{id}/replay",
      summary: "Requeue historical deliveries from sequence/time anchors.",
      auth: "Same as create topic.",
      pathFields: [{ name: "id", type: "string", required: true, description: "Subscription ID." }],
      requestFields: [
        { name: "from_sequence", type: "number", required: false, description: "Start sequence (default 1)." },
        { name: "from_time", type: "string(RFC3339)", required: false, description: "Resolve sequence from time." },
        { name: "limit", type: "number", required: false, description: "Max deliveries to queue." },
        { name: "reset_cursor", type: "boolean", required: false, description: "Reset cursor to replay anchor." },
      ],
      responseFields: [
        { name: "status", type: "string", required: true, description: "replayed" },
        { name: "subscriptionId", type: "string", required: true, description: "Subscription ID." },
        { name: "from_sequence", type: "number", required: true, description: "Effective replay start." },
        { name: "queued", type: "number", required: true, description: "Queued delivery count." },
      ],
      successCodes: [{ code: 200, meaning: "Replay accepted" }],
      errorCodes: [
        { code: 400, meaning: "Invalid replay payload" },
        { code: 404, meaning: "Subscription not found" },
      ],
      requestExample: `{
  "from_sequence": 100,
  "limit": 500,
  "reset_cursor": true
}`,
      responseExample: `{
  "status": "replayed",
  "subscriptionId": "sub_001",
  "from_sequence": 100,
  "queued": 500
}`,
    },
  },
]

const operationEndpoints: ApiEndpointDoc[] = [
  {
    slug: "health",
    spec: {
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
  },
  {
    slug: "health-probes",
    spec: {
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
  },
  {
    slug: "global-timeseries",
    spec: {
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
  },
  {
    slug: "function-logs",
    spec: {
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
  },
  {
    slug: "list-invocations",
    spec: {
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
  },
]

export const apiDomainOrder: ApiDomainKey[] = ["functions", "workflows", "events", "operations"]

export const apiDomainDocs: Record<ApiDomainKey, ApiDomainDoc> = {
  functions: {
    key: "functions",
    title: "Functions API",
    description: "Function lifecycle and invocation contracts.",
    coverage:
      "Core endpoints used for function management and execution. Version/code/layer endpoints are exposed as additional operational APIs.",
    endpoints: functionEndpoints,
  },
  workflows: {
    key: "workflows",
    title: "Workflows API",
    description: "Workflow definitions, version publishing, and run lifecycle endpoints.",
    coverage:
      "High-frequency workflow endpoints. Workflow admin operations (delete, get version, list versions) follow the same schema pattern.",
    endpoints: workflowEndpoints,
  },
  events: {
    key: "events",
    title: "Events API",
    description: "Topic, subscription, publish, replay, and delivery operations.",
    coverage:
      "Core event bus contracts for topics and deliveries. Outbox relay, seek cursor, and advanced retry operations are available as adjacent APIs.",
    endpoints: eventEndpoints,
  },
  operations: {
    key: "operations",
    title: "Operations API",
    description: "Operational observability and probe endpoints.",
    coverage:
      "Runtime health and observability endpoints used by dashboards and SRE workflows.",
    endpoints: operationEndpoints,
  },
}

export function apiEndpointHref(domain: ApiDomainKey, endpointSlug: string): string {
  return `/docs/api/${domain}/${endpointSlug}`
}

export function getApiDomainDoc(domain: string): ApiDomainDoc | null {
  if (domain in apiDomainDocs) {
    return apiDomainDocs[domain as ApiDomainKey]
  }
  return null
}

export function getApiEndpointDoc(domain: ApiDomainKey, endpointSlug: string): {
  domainDoc: ApiDomainDoc
  endpointDoc: ApiEndpointDoc
  index: number
} | null {
  const domainDoc = apiDomainDocs[domain]
  const index = domainDoc.endpoints.findIndex((endpoint) => endpoint.slug === endpointSlug)
  if (index < 0) {
    return null
  }
  return {
    domainDoc,
    endpointDoc: domainDoc.endpoints[index],
    index,
  }
}
