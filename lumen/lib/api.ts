// Nova API Client
// Connects to the Nova backend at /api (proxied via Next.js rewrites)

const API_BASE = "/api";

// Types matching backend domain models
export interface NovaFunction {
  id: string;
  name: string;
  runtime: string;
  handler: string;
  code_path: string;
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
  code_path?: string;
  code?: string;
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
  code_path?: string;
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
};

// Runtimes API
export const runtimesApi = {
  list: () => request<Runtime[]>("/runtimes"),

  create: (data: CreateRuntimeRequest) =>
    request<Runtime>("/runtimes", {
      method: "POST",
      body: JSON.stringify(data),
    }),

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
  timeseries: () => request<TimeSeriesPoint[]>("/metrics/timeseries"),
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

export { ApiError };
