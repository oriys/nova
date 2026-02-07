// nova API client
// Connects to the nova backend at /api (proxied via Next.js rewrites)

const API_BASE = "/api";

// Types matching backend domain models
export interface NovaFunction {
  id: string;
  name: string;
  runtime: string;
  handler: string;
  code_hash?: string;
  memory_mb: number;
  timeout_s: number;
  min_replicas: number;
  max_replicas?: number;
  mode?: string;
  limits?: ResourceLimits;
  env_vars?: Record<string, string>;
  created_at: string;
  updated_at: string;
  version?: number;
  // Code-related fields from create response
  source_code?: string;
  compile_status?: CompileStatus;
  compile_error?: string;
}

export type CompileStatus = 'pending' | 'compiling' | 'success' | 'failed' | 'not_required';

export interface FunctionCodeResponse {
  function_id: string;
  source_code?: string;
  source_hash?: string;
  compile_status?: CompileStatus;
  compile_error?: string;
  binary_hash?: string;
}

export interface UpdateCodeResponse {
  compile_status: CompileStatus;
  source_hash: string;
}

export interface ResourceLimits {
  vcpus?: number;
  disk_iops?: number;
  disk_bandwidth?: number;
  net_rx_bandwidth?: number;
  net_tx_bandwidth?: number;
}

export interface Runtime {
  id: string;
  name: string;
  version: string;
  status: "available" | "deprecated" | "maintenance";
  image_name?: string;
  entrypoint?: string[];
  file_extension?: string;
  env_vars?: Record<string, string>;
  functions_count: number;
}

export interface LogEntry {
  id: string;
  request_id?: string;
  function_id: string;
  function_name: string;
  runtime: string;
  timestamp?: string;
  created_at: string;
  stdout?: string;
  stderr?: string;
  input?: unknown;
  output?: unknown;
  duration_ms: number;
  cold_start: boolean;
  success: boolean;
  error_message?: string;
  input_size?: number;
  output_size?: number;
}

export interface InvokeResponse {
  request_id: string;
  output: unknown;
  error?: string;
  duration_ms: number;
  cold_start: boolean;
  version?: number;
}

export interface FunctionMetrics {
  function_id: string;
  function_name: string;
  invocations: {
    invocations: number;
    successes: number;
    failures: number;
    cold_starts: number;
    warm_starts: number;
    avg_ms: number;
    min_ms: number;
    max_ms: number;
  };
  pool: {
    active_vms: number;
    busy_vms: number;
    idle_vms: number;
  };
  timeseries?: TimeSeriesPoint[];
}

export interface GlobalMetrics {
  uptime_seconds: number;
  invocations: {
    total: number;
    success: number;
    failed: number;
    cold: number;
    warm: number;
    cold_pct: number;
  };
  latency_ms: {
    avg: number;
    min: number;
    max: number;
  };
  vms: {
    created: number;
    stopped: number;
    crashed: number;
    snapshots_hit: number;
  };
  functions: Record<string, {
    invocations: number;
    successes: number;
    failures: number;
    cold_starts: number;
    warm_starts: number;
    avg_ms: number;
    min_ms: number;
    max_ms: number;
  }>;
}

export interface HealthStatus {
  status: string;
  components?: {
    postgres: boolean;
    pool: {
      active_vms: number;
      total_pools: number;
    };
  };
  uptime_seconds?: number;
}

export interface CreateFunctionRequest {
  name: string;
  runtime: string;
  handler?: string;
  code: string; // Source code (required)
  memory_mb?: number;
  timeout_s?: number;
  min_replicas?: number;
  max_replicas?: number;
  mode?: string;
  env_vars?: Record<string, string>;
  limits?: ResourceLimits;
}

export interface UpdateFunctionRequest {
  handler?: string;
  code?: string; // Source code (optional for updates)
  memory_mb?: number;
  timeout_s?: number;
  min_replicas?: number;
  max_replicas?: number;
  mode?: string;
  env_vars?: Record<string, string>;
  limits?: ResourceLimits;
}

export interface CreateRuntimeRequest {
  id: string;
  name: string;
  version: string;
  status?: string;
  image_name: string;
  entrypoint: string[];
  file_extension: string;
  env_vars?: Record<string, string>;
}

export interface UploadRuntimeRequest {
  id: string;
  name: string;
  version?: string;
  entrypoint: string[];
  file_extension: string;
  env_vars?: Record<string, string>;
}

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(
  path: string,
  options?: RequestInit
): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });

  if (!response.ok) {
    const text = await response.text();
    throw new ApiError(response.status, text || response.statusText);
  }

  return response.json();
}

// Functions API
export const functionsApi = {
  list: () => request<NovaFunction[]>("/functions"),

  get: (name: string) => request<NovaFunction>(`/functions/${encodeURIComponent(name)}`),

  create: (data: CreateFunctionRequest) =>
    request<NovaFunction>("/functions", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  update: (name: string, data: UpdateFunctionRequest) =>
    request<NovaFunction>(`/functions/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),

  delete: (name: string) =>
    request<{ status: string; name: string }>(`/functions/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  invoke: (name: string, payload: unknown = {}) =>
    request<InvokeResponse>(`/functions/${encodeURIComponent(name)}/invoke`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  logs: (name: string, tail: number = 10) =>
    request<LogEntry[]>(`/functions/${encodeURIComponent(name)}/logs?tail=${tail}`),

  metrics: (name: string) =>
    request<FunctionMetrics>(`/functions/${encodeURIComponent(name)}/metrics`),

  getCode: (name: string) =>
    request<FunctionCodeResponse>(`/functions/${encodeURIComponent(name)}/code`),

  updateCode: (name: string, code: string) =>
    request<UpdateCodeResponse>(`/functions/${encodeURIComponent(name)}/code`, {
      method: "PUT",
      body: JSON.stringify({ code }),
    }),

  listVersions: (name: string) =>
    request<FunctionVersionEntry[]>(`/functions/${encodeURIComponent(name)}/versions`),

  getVersion: (name: string, version: number) =>
    request<FunctionVersionEntry>(`/functions/${encodeURIComponent(name)}/versions/${version}`),
};

// Runtimes API
export const runtimesApi = {
  list: () => request<Runtime[]>("/runtimes"),

  create: (data: CreateRuntimeRequest) =>
    request<Runtime>("/runtimes", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  upload: async (file: File, metadata: UploadRuntimeRequest): Promise<Runtime> => {
    const formData = new FormData();
    formData.append("file", file);
    formData.append("metadata", JSON.stringify(metadata));

    const response = await fetch(`${API_BASE}/runtimes/upload`, {
      method: "POST",
      body: formData,
    });

    if (!response.ok) {
      const text = await response.text();
      throw new ApiError(response.status, text || response.statusText);
    }

    return response.json();
  },

  delete: (id: string) =>
    request<{ status: string; id: string }>(`/runtimes/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
};

export interface TimeSeriesPoint {
  timestamp: string;
  invocations: number;
  errors: number;
  avg_duration: number;
}

// Metrics API
export const metricsApi = {
  global: () => request<GlobalMetrics>("/metrics"),
  timeseries: (range?: string) =>
    request<TimeSeriesPoint[]>(`/metrics/timeseries${range ? `?range=${range}` : ""}`),
  stats: () => request<Record<string, unknown>>("/stats"),
};

// Invocations API (global history)
export const invocationsApi = {
  list: (limit: number = 100) =>
    request<LogEntry[]>(`/invocations?limit=${limit}`),
};

// Health API
export const healthApi = {
  check: () => request<HealthStatus>("/health"),
  ready: () => request<{ status: string }>("/health/ready"),
  live: () => request<{ status: string }>("/health/live"),
};

// Config API
export const configApi = {
  get: () => request<Record<string, string>>("/config"),
  update: (data: Record<string, string>) =>
    request<Record<string, string>>("/config", {
      method: "PUT",
      body: JSON.stringify(data),
    }),
};

// Snapshots API
export const snapshotsApi = {
  list: () =>
    request<
      Array<{
        function_id: string;
        function_name: string;
        snap_size: number;
        mem_size: number;
        total_size: number;
        created_at: string;
      }>
    >("/snapshots"),

  create: (name: string) =>
    request<{ status: string; message: string }>(`/functions/${encodeURIComponent(name)}/snapshot`, {
      method: "POST",
    }),

  delete: (name: string) =>
    request<{ status: string; message: string }>(`/functions/${encodeURIComponent(name)}/snapshot`, {
      method: "DELETE",
    }),
};

// --- Workflow Types ---

export type WorkflowStatus = "active" | "inactive" | "deleted";
export type RunStatus = "pending" | "running" | "succeeded" | "failed" | "cancelled";
export type NodeStatus = "pending" | "ready" | "running" | "succeeded" | "failed" | "skipped";

export interface Workflow {
  id: string;
  name: string;
  description: string;
  status: WorkflowStatus;
  current_version: number;
  created_at: string;
  updated_at: string;
}

export interface WorkflowVersion {
  id: string;
  workflow_id: string;
  version: number;
  definition: unknown;
  nodes?: WorkflowNode[];
  edges?: WorkflowEdge[];
  created_at: string;
}

export interface WorkflowNode {
  id: string;
  version_id: string;
  node_key: string;
  function_name: string;
  input_mapping?: unknown;
  retry_policy?: RetryPolicy;
  timeout_s: number;
  position: number;
}

export interface WorkflowEdge {
  id: string;
  version_id: string;
  from_node_id: string;
  to_node_id: string;
}

export interface RetryPolicy {
  max_attempts: number;
  base_ms: number;
  max_backoff_ms: number;
}

export interface WorkflowRun {
  id: string;
  workflow_id: string;
  workflow_name?: string;
  version_id: string;
  version?: number;
  status: RunStatus;
  trigger_type: string;
  input?: unknown;
  output?: unknown;
  error_message?: string;
  started_at: string;
  finished_at?: string;
  created_at: string;
  nodes?: RunNode[];
}

export interface RunNode {
  id: string;
  run_id: string;
  node_id: string;
  node_key: string;
  function_name: string;
  status: NodeStatus;
  unresolved_deps: number;
  attempt: number;
  input?: unknown;
  output?: unknown;
  error_message?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
}

export interface NodeDefinition {
  node_key: string;
  function_name: string;
  input_mapping?: unknown;
  retry_policy?: RetryPolicy;
  timeout_s?: number;
}

export interface EdgeDefinition {
  from: string;
  to: string;
}

export interface PublishVersionRequest {
  nodes: NodeDefinition[];
  edges: EdgeDefinition[];
}

// Workflows API
export const workflowsApi = {
  list: () => request<Workflow[]>("/workflows"),

  get: (name: string) => request<Workflow>(`/workflows/${encodeURIComponent(name)}`),

  create: (data: { name: string; description?: string }) =>
    request<Workflow>("/workflows", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  delete: (name: string) =>
    request<{ status: string; name: string }>(`/workflows/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  listVersions: (name: string) =>
    request<WorkflowVersion[]>(`/workflows/${encodeURIComponent(name)}/versions`),

  getVersion: (name: string, version: number) =>
    request<WorkflowVersion>(`/workflows/${encodeURIComponent(name)}/versions/${version}`),

  publishVersion: (name: string, def: PublishVersionRequest) =>
    request<WorkflowVersion>(`/workflows/${encodeURIComponent(name)}/versions`, {
      method: "POST",
      body: JSON.stringify(def),
    }),

  listRuns: (name: string) =>
    request<WorkflowRun[]>(`/workflows/${encodeURIComponent(name)}/runs`),

  getRun: (name: string, runID: string) =>
    request<WorkflowRun>(`/workflows/${encodeURIComponent(name)}/runs/${encodeURIComponent(runID)}`),

  triggerRun: (name: string, input: unknown = {}) =>
    request<WorkflowRun>(`/workflows/${encodeURIComponent(name)}/runs`, {
      method: "POST",
      body: JSON.stringify({ input }),
    }),
};

// --- API Keys ---

export interface APIKeyEntry {
  name: string;
  tier: string;
  enabled: boolean;
  created_at: string;
}

export interface APIKeyCreateResponse {
  name: string;
  key: string;
  tier: string;
}

export const apiKeysApi = {
  list: () => request<APIKeyEntry[]>("/apikeys"),

  create: (name: string, tier: string = "default") =>
    request<APIKeyCreateResponse>("/apikeys", {
      method: "POST",
      body: JSON.stringify({ name, tier }),
    }),

  delete: (name: string) =>
    request<{ status: string; name: string }>(`/apikeys/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  toggle: (name: string, enabled: boolean) =>
    request<{ name: string; enabled: boolean }>(`/apikeys/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
};

// --- Secrets ---

export interface SecretEntry {
  name: string;
  created_at: string;
}

export const secretsApi = {
  list: () => request<SecretEntry[]>("/secrets"),

  create: (name: string, value: string) =>
    request<{ name: string; status: string }>("/secrets", {
      method: "POST",
      body: JSON.stringify({ name, value }),
    }),

  delete: (name: string) =>
    request<{ status: string; name: string }>(`/secrets/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
};

// --- Function Versions ---

export interface FunctionVersionEntry {
  function_id: string;
  version: number;
  code_hash: string;
  handler: string;
  memory_mb: number;
  timeout_s: number;
  mode?: string;
  limits?: ResourceLimits;
  env_vars?: Record<string, string>;
  description?: string;
  created_at: string;
}

// --- Schedules ---

export interface ScheduleEntry {
  id: string;
  function_name: string;
  cron_expression: string;
  input?: unknown;
  enabled: boolean;
  last_run_at?: string;
  created_at: string;
  updated_at: string;
}

export const schedulesApi = {
  list: (functionName: string) =>
    request<ScheduleEntry[]>(`/functions/${encodeURIComponent(functionName)}/schedules`),

  create: (functionName: string, cronExpression: string, input?: unknown) =>
    request<ScheduleEntry>(`/functions/${encodeURIComponent(functionName)}/schedules`, {
      method: "POST",
      body: JSON.stringify({ cron_expression: cronExpression, input }),
    }),

  delete: (functionName: string, id: string) =>
    request<{ status: string; id: string }>(`/functions/${encodeURIComponent(functionName)}/schedules/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  toggle: (functionName: string, id: string, enabled: boolean) =>
    request<{ id: string; enabled: boolean }>(`/functions/${encodeURIComponent(functionName)}/schedules/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
};

export { ApiError };
