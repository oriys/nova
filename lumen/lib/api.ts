// nova API client
// Connects to the nova backend at /api (proxied via Next.js rewrites)
import { getTenantScopeHeaders } from "@/lib/tenant-scope";

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
  network_policy?: NetworkPolicy;
  rollout_policy?: RolloutPolicy;
  auto_scale_policy?: AutoScalePolicy;
  capacity_policy?: CapacityPolicy;
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

export interface EgressRule {
  host: string;
  port?: number;
  protocol?: string;
}

export interface IngressRule {
  source: string;
  port?: number;
  protocol?: string;
}

export interface NetworkPolicy {
  isolation_mode?: string;
  ingress_rules?: IngressRule[];
  egress_rules?: EgressRule[];
  deny_external_access?: boolean;
}

export interface RolloutPolicy {
  enabled?: boolean;
  canary_function?: string;
  canary_percent?: number;
}

export interface ScaleThresholds {
  queue_depth?: number;
  queue_wait_ms?: number;
  avg_latency_ms?: number;
  cold_start_pct?: number;
  idle_pct?: number;
  target_concurrency?: number;
}

export interface AutoScalePolicy {
  enabled: boolean;
  min_replicas?: number;
  max_replicas?: number;
  target_utilization?: number;
  scale_up_thresholds?: ScaleThresholds;
  scale_down_thresholds?: ScaleThresholds;
  cooldown_scale_up_s?: number;
  cooldown_scale_down_s?: number;
  scale_down_step?: number;
  scale_up_step_max?: number;
  scale_down_stabilization_s?: number;
  min_sample_count?: number;
}

export interface CapacityPolicy {
  enabled: boolean;
  max_inflight?: number;
  max_queue_depth?: number;
  max_queue_wait_ms?: number;
  shed_status_code?: number;
  retry_after_s?: number;
  breaker_error_pct?: number;
  breaker_window_s?: number;
  breaker_open_s?: number;
  half_open_probes?: number;
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

export interface RouteRateLimit {
  requests_per_second: number;
  burst_size: number;
}

export interface GatewayRateLimitTemplate {
  enabled: boolean;
  requests_per_second: number;
  burst_size: number;
}

export interface GatewayRoute {
  id: string;
  domain: string;
  path: string;
  methods?: string[];
  function_name: string;
  auth_strategy: string;
  auth_config?: Record<string, string>;
  request_schema?: unknown;
  rate_limit?: RouteRateLimit;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateGatewayRouteRequest {
  domain?: string;
  path: string;
  methods?: string[];
  function_name: string;
  auth_strategy?: string;
  auth_config?: Record<string, string>;
  request_schema?: unknown;
  rate_limit?: RouteRateLimit;
  enabled?: boolean;
}

export interface UpdateGatewayRouteRequest {
  domain?: string;
  path?: string;
  methods?: string[];
  function_name?: string;
  auth_strategy?: string;
  auth_config?: Record<string, string>;
  request_schema?: unknown;
  rate_limit?: RouteRateLimit;
  enabled?: boolean;
}

export interface LayerEntry {
  id: string;
  name: string;
  runtime: string;
  version: string;
  content_hash?: string;
  size_mb: number;
  files: string[];
  image_path: string;
  created_at: string;
  updated_at: string;
}

export interface CreateLayerRequest {
  name: string;
  runtime: string;
  files: Record<string, string>; // path -> base64-encoded content
}

export interface FunctionLayerBinding {
  position: number;
  id: string;
  name: string;
  size_mb: number;
}

export interface SetFunctionLayersResponse {
  status: string;
  function: string;
  layers: FunctionLayerBinding[];
  note?: string;
}

export interface TenantEntry {
  id: string;
  name: string;
  status: string;
  tier: string;
  created_at: string;
  updated_at: string;
}

export interface NamespaceEntry {
  id: string;
  tenant_id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface TenantQuotaEntry {
  tenant_id: string;
  dimension: string;
  hard_limit: number;
  soft_limit: number;
  burst: number;
  window_s: number;
  updated_at: string;
}

export interface TenantUsageEntry {
  tenant_id: string;
  dimension: string;
  used: number;
  updated_at: string;
}

export interface TenantQuotaDecision {
  tenant_id: string;
  dimension: string;
  allowed: boolean;
  used: number;
  limit: number;
  window_s?: number;
  retry_after_s?: number;
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

export type AsyncInvocationStatus = "queued" | "running" | "succeeded" | "dlq";

export interface AsyncInvocationJob {
  id: string;
  function_id: string;
  function_name: string;
  payload: unknown;
  status: AsyncInvocationStatus;
  attempt: number;
  max_attempts: number;
  backoff_base_ms: number;
  backoff_max_ms: number;
  next_run_at: string;
  locked_by?: string;
  locked_until?: string;
  request_id?: string;
  output?: unknown;
  duration_ms?: number;
  cold_start?: boolean;
  last_error?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface EventTopic {
  id: string;
  name: string;
  description?: string;
  retention_hours: number;
  created_at: string;
  updated_at: string;
}

export type EventSubscriptionType = "function" | "workflow";

export interface EventSubscription {
  id: string;
  topic_id: string;
  topic_name?: string;
  name: string;
  consumer_group: string;
  function_id: string;
  function_name: string;
  workflow_id?: string;
  workflow_name?: string;
  enabled: boolean;
  max_attempts: number;
  backoff_base_ms: number;
  backoff_max_ms: number;
  max_inflight: number;
  rate_limit_per_sec: number;
  last_dispatch_at?: string;
  last_acked_sequence: number;
  last_acked_at?: string;
  lag: number;
  inflight: number;
  queued: number;
  dlq: number;
  oldest_unacked_age_s?: number;
  created_at: string;
  updated_at: string;
  // Webhook fields
  type: EventSubscriptionType;
  webhook_url?: string;
  webhook_method?: string;
  webhook_headers?: Record<string, string>;
  webhook_signing_secret?: string;
  webhook_timeout_ms?: number;
}

export interface EventMessage {
  id: string;
  topic_id: string;
  topic_name?: string;
  sequence: number;
  ordering_key?: string;
  payload: unknown;
  headers?: unknown;
  published_at: string;
  created_at: string;
}

export type EventDeliveryStatus = "queued" | "running" | "succeeded" | "dlq";

export type EventOutboxStatus = "pending" | "publishing" | "published" | "failed";

export interface EventDelivery {
  id: string;
  topic_id: string;
  topic_name?: string;
  subscription_id: string;
  subscription_name?: string;
  consumer_group?: string;
  message_id: string;
  message_sequence: number;
  ordering_key?: string;
  payload: unknown;
  headers?: unknown;
  status: EventDeliveryStatus;
  attempt: number;
  max_attempts: number;
  backoff_base_ms: number;
  backoff_max_ms: number;
  next_run_at: string;
  locked_by?: string;
  locked_until?: string;
  function_id: string;
  function_name: string;
  workflow_id?: string;
  workflow_name?: string;
  request_id?: string;
  output?: unknown;
  duration_ms?: number;
  cold_start?: boolean;
  last_error?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
  // Webhook fields
  subscription_type: EventSubscriptionType;
  webhook_url?: string;
  webhook_method?: string;
  webhook_timeout_ms?: number;
}

export interface EventOutboxJob {
  id: string;
  topic_id: string;
  topic_name: string;
  ordering_key?: string;
  payload: unknown;
  headers?: unknown;
  status: EventOutboxStatus;
  attempt: number;
  max_attempts: number;
  backoff_base_ms: number;
  backoff_max_ms: number;
  next_attempt_at: string;
  locked_by?: string;
  locked_until?: string;
  message_id?: string;
  last_error?: string;
  published_at?: string;
  created_at: string;
  updated_at: string;
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

export interface FunctionDiagnosticsSlowInvocation {
  id: string;
  created_at: string;
  duration_ms: number;
  cold_start: boolean;
  success: boolean;
  error_message?: string;
}

export interface FunctionDiagnostics {
  function_id: string;
  function_name: string;
  window_seconds: number;
  sample_size: number;
  total_invocations: number;
  avg_duration_ms: number;
  p50_duration_ms: number;
  p95_duration_ms: number;
  p99_duration_ms: number;
  max_duration_ms: number;
  error_rate_pct: number;
  cold_start_rate_pct: number;
  slow_threshold_ms: number;
  slow_count: number;
  slow_invocations: FunctionDiagnosticsSlowInvocation[];
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
    postgres?: boolean | string;
    pool?: {
      active_vms?: number;
      total_pools?: number | null;
    };
    zenith?: string;
    nova?: string;
    comet?: string;
    [key: string]: unknown;
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
  network_policy?: NetworkPolicy;
  rollout_policy?: RolloutPolicy;
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
  network_policy?: NetworkPolicy;
  rollout_policy?: RolloutPolicy;
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
  const headers = new Headers(options?.headers);
  if (!(options?.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const tenantHeaders = getTenantScopeHeaders();
  headers.set("X-Nova-Tenant", tenantHeaders["X-Nova-Tenant"]);
  headers.set("X-Nova-Namespace", tenantHeaders["X-Nova-Namespace"]);

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const text = await response.text();
    throw new ApiError(response.status, text || response.statusText);
  }

  return response.json();
}

// Functions API
export const functionsApi = {
  list: (search?: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (search) params.set("search", search);
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<NovaFunction[]>(`/functions${qs ? `?${qs}` : ""}`);
  },

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

  invokeAsync: (
    name: string,
    payload: unknown = {},
    options?: {
      max_attempts?: number;
      backoff_base_ms?: number;
      backoff_max_ms?: number;
      idempotency_key?: string;
      idempotency_ttl_s?: number;
    }
  ) =>
    request<AsyncInvocationJob>(`/functions/${encodeURIComponent(name)}/invoke-async`, {
      method: "POST",
      body: JSON.stringify({
        payload,
        ...(options || {}),
      }),
    }),

  listAsyncInvocations: (name: string, limit: number = 50, status?: AsyncInvocationStatus | AsyncInvocationStatus[], offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (status) {
      params.set("status", Array.isArray(status) ? status.join(",") : status);
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return request<AsyncInvocationJob[]>(
      `/functions/${encodeURIComponent(name)}/async-invocations?${params.toString()}`
    );
  },

  getAsyncInvocation: (id: string) =>
    request<AsyncInvocationJob>(`/async-invocations/${encodeURIComponent(id)}`),

  retryAsyncInvocation: (id: string, maxAttempts?: number) =>
    request<AsyncInvocationJob>(`/async-invocations/${encodeURIComponent(id)}/retry`, {
      method: "POST",
      body: JSON.stringify(
        maxAttempts && maxAttempts > 0 ? { max_attempts: maxAttempts } : {}
      ),
    }),

  logs: (name: string, tail: number = 10) =>
    request<LogEntry[]>(`/functions/${encodeURIComponent(name)}/logs?tail=${tail}`),

  metrics: (name: string) =>
    request<FunctionMetrics>(`/functions/${encodeURIComponent(name)}/metrics`),

  diagnostics: (name: string, window: string = "24h", sample: number = 1000) =>
    request<FunctionDiagnostics>(
      `/functions/${encodeURIComponent(name)}/diagnostics?window=${encodeURIComponent(window)}&sample=${Math.max(1, Math.floor(sample))}`
    ),

  getCode: (name: string) =>
    request<FunctionCodeResponse>(`/functions/${encodeURIComponent(name)}/code`),

  updateCode: (name: string, code: string) =>
    request<UpdateCodeResponse>(`/functions/${encodeURIComponent(name)}/code`, {
      method: "PUT",
      body: JSON.stringify({ code }),
    }),

  listVersions: (name: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<FunctionVersionEntry[]>(`/functions/${encodeURIComponent(name)}/versions${qs ? `?${qs}` : ""}`);
  },

  getVersion: (name: string, version: number) =>
    request<FunctionVersionEntry>(`/functions/${encodeURIComponent(name)}/versions/${version}`),

  heatmap: (name: string, weeks: number = 52) =>
    request<HeatmapPoint[]>(`/functions/${encodeURIComponent(name)}/heatmap?weeks=${weeks}`),

  getScalingPolicy: (name: string) =>
    request<AutoScalePolicy>(`/functions/${encodeURIComponent(name)}/scaling`),

  setScalingPolicy: (name: string, data: AutoScalePolicy) =>
    request<AutoScalePolicy>(`/functions/${encodeURIComponent(name)}/scaling`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteScalingPolicy: (name: string) =>
    request<{ status: string; function: string }>(`/functions/${encodeURIComponent(name)}/scaling`, {
      method: "DELETE",
    }),

  getCapacityPolicy: (name: string) =>
    request<CapacityPolicy>(`/functions/${encodeURIComponent(name)}/capacity`),

  setCapacityPolicy: (name: string, data: CapacityPolicy) =>
    request<CapacityPolicy>(`/functions/${encodeURIComponent(name)}/capacity`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteCapacityPolicy: (name: string) =>
    request<{ status: string; function: string }>(`/functions/${encodeURIComponent(name)}/capacity`, {
      method: "DELETE",
    }),
};

// Tenant and namespace management API
export const tenantsApi = {
  list: (limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<TenantEntry[]>(`/tenants${qs ? `?${qs}` : ""}`);
  },

  create: (data: { id: string; name?: string; status?: string; tier?: string }) =>
    request<TenantEntry>("/tenants", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  update: (tenantID: string, data: { name?: string; status?: string; tier?: string }) =>
    request<TenantEntry>(`/tenants/${encodeURIComponent(tenantID)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),

  delete: (tenantID: string) =>
    request<{ status: string; id: string }>(`/tenants/${encodeURIComponent(tenantID)}`, {
      method: "DELETE",
    }),

  listNamespaces: (tenantID: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<NamespaceEntry[]>(`/tenants/${encodeURIComponent(tenantID)}/namespaces${qs ? `?${qs}` : ""}`);
  },

  createNamespace: (tenantID: string, data: { name: string }) =>
    request<NamespaceEntry>(`/tenants/${encodeURIComponent(tenantID)}/namespaces`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateNamespace: (tenantID: string, namespaceName: string, data: { name: string }) =>
    request<NamespaceEntry>(
      `/tenants/${encodeURIComponent(tenantID)}/namespaces/${encodeURIComponent(namespaceName)}`,
      {
        method: "PATCH",
        body: JSON.stringify(data),
      }
    ),

  deleteNamespace: (tenantID: string, namespaceName: string) =>
    request<{ status: string; tenant_id: string; name: string }>(
      `/tenants/${encodeURIComponent(tenantID)}/namespaces/${encodeURIComponent(namespaceName)}`,
      {
        method: "DELETE",
      }
    ),

  listQuotas: (tenantID: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<TenantQuotaEntry[]>(`/tenants/${encodeURIComponent(tenantID)}/quotas${qs ? `?${qs}` : ""}`);
  },

  upsertQuota: (
    tenantID: string,
    dimension: string,
    data: {
      hard_limit: number;
      soft_limit?: number;
      burst?: number;
      window_s?: number;
    }
  ) =>
    request<TenantQuotaEntry>(
      `/tenants/${encodeURIComponent(tenantID)}/quotas/${encodeURIComponent(dimension)}`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      }
    ),

  deleteQuota: (tenantID: string, dimension: string) =>
    request<{ status: string; tenant_id: string; dimension: string }>(
      `/tenants/${encodeURIComponent(tenantID)}/quotas/${encodeURIComponent(dimension)}`,
      {
        method: "DELETE",
      }
    ),

  usage: (tenantID: string, refresh: boolean = true) =>
    request<TenantUsageEntry[]>(
      `/tenants/${encodeURIComponent(tenantID)}/usage?refresh=${refresh ? "true" : "false"}`
    ),
};

// Event bus API
export const eventsApi = {
  listTopics: (limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return request<EventTopic[]>(`/topics?${params.toString()}`);
  },

  getTopic: (name: string) =>
    request<EventTopic>(`/topics/${encodeURIComponent(name)}`),

  createTopic: (data: { name: string; description?: string; retention_hours?: number }) =>
    request<EventTopic>("/topics", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteTopic: (name: string) =>
    request<{ status: string; name: string }>(`/topics/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  publish: (
    topicName: string,
    data: { payload?: unknown; headers?: unknown; ordering_key?: string }
  ) =>
    request<{ message: EventMessage; deliveries: number }>(`/topics/${encodeURIComponent(topicName)}/publish`, {
      method: "POST",
      body: JSON.stringify({
        payload: data.payload ?? {},
        ...(data.headers !== undefined ? { headers: data.headers } : {}),
        ...(data.ordering_key ? { ordering_key: data.ordering_key } : {}),
      }),
    }),

  enqueueOutbox: (
    topicName: string,
    data: {
      payload?: unknown;
      headers?: unknown;
      ordering_key?: string;
      max_attempts?: number;
      backoff_base_ms?: number;
      backoff_max_ms?: number;
    }
  ) =>
    request<EventOutboxJob>(`/topics/${encodeURIComponent(topicName)}/outbox`, {
      method: "POST",
      body: JSON.stringify({
        payload: data.payload ?? {},
        ...(data.headers !== undefined ? { headers: data.headers } : {}),
        ...(data.ordering_key ? { ordering_key: data.ordering_key } : {}),
        ...(typeof data.max_attempts === "number" ? { max_attempts: data.max_attempts } : {}),
        ...(typeof data.backoff_base_ms === "number" ? { backoff_base_ms: data.backoff_base_ms } : {}),
        ...(typeof data.backoff_max_ms === "number" ? { backoff_max_ms: data.backoff_max_ms } : {}),
      }),
    }),

  listOutbox: (
    topicName: string,
    limit: number = 100,
    status?: EventOutboxStatus | EventOutboxStatus[],
    offset?: number
  ) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (status) {
      params.set("status", Array.isArray(status) ? status.join(",") : status);
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return request<EventOutboxJob[]>(
      `/topics/${encodeURIComponent(topicName)}/outbox?${params.toString()}`
    );
  },

  listMessages: (topicName: string, limit: number = 50, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return request<EventMessage[]>(`/topics/${encodeURIComponent(topicName)}/messages?${params.toString()}`);
  },

  listSubscriptions: (topicName: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<EventSubscription[]>(`/topics/${encodeURIComponent(topicName)}/subscriptions${qs ? `?${qs}` : ""}`);
  },

  createSubscription: (
    topicName: string,
    data: {
      name: string;
      consumer_group?: string;
      type?: EventSubscriptionType;
      // Function fields
      function_name?: string;
      workflow_name?: string;
      // Webhook fields
      webhook_url?: string;
      webhook_method?: string;
      webhook_headers?: Record<string, string>;
      webhook_signing_secret?: string;
      webhook_timeout_ms?: number;
      // Common fields
      enabled?: boolean;
      max_attempts?: number;
      backoff_base_ms?: number;
      backoff_max_ms?: number;
      max_inflight?: number;
      rate_limit_per_sec?: number;
    }
  ) =>
    request<EventSubscription>(`/topics/${encodeURIComponent(topicName)}/subscriptions`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getSubscription: (id: string) =>
    request<EventSubscription>(`/subscriptions/${encodeURIComponent(id)}`),

  updateSubscription: (
    id: string,
    data: {
      name?: string;
      consumer_group?: string;
      function_name?: string;
      workflow_name?: string;
      enabled?: boolean;
      max_attempts?: number;
      backoff_base_ms?: number;
      backoff_max_ms?: number;
      max_inflight?: number;
      rate_limit_per_sec?: number;
    }
  ) =>
    request<EventSubscription>(`/subscriptions/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),

  deleteSubscription: (id: string) =>
    request<{ status: string; id: string }>(`/subscriptions/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  listDeliveries: (
    subscriptionID: string,
    limit: number = 100,
    status?: EventDeliveryStatus | EventDeliveryStatus[],
    offset?: number
  ) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (status) {
      params.set("status", Array.isArray(status) ? status.join(",") : status);
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return request<EventDelivery[]>(
      `/subscriptions/${encodeURIComponent(subscriptionID)}/deliveries?${params.toString()}`
    );
  },

  getDelivery: (id: string) =>
    request<EventDelivery>(`/deliveries/${encodeURIComponent(id)}`),

  replaySubscription: (
    subscriptionID: string,
    fromSequence?: number,
    limit?: number,
    options?: { from_time?: string; reset_cursor?: boolean }
  ) =>
    request<{ status: string; subscriptionId: string; from_sequence: number; queued: number }>(
      `/subscriptions/${encodeURIComponent(subscriptionID)}/replay`,
      {
        method: "POST",
        body: JSON.stringify({
          ...(typeof fromSequence === "number" ? { from_sequence: fromSequence } : {}),
          ...(typeof limit === "number" ? { limit } : {}),
          ...(options?.from_time ? { from_time: options.from_time } : {}),
          ...(options?.reset_cursor ? { reset_cursor: true } : {}),
        }),
      }
    ),

  seekSubscription: (subscriptionID: string, fromSequence?: number, fromTime?: string) =>
    request<{ status: string; subscriptionId: string; from_sequence: number; subscription: EventSubscription }>(
      `/subscriptions/${encodeURIComponent(subscriptionID)}/seek`,
      {
        method: "POST",
        body: JSON.stringify({
          ...(typeof fromSequence === "number" ? { from_sequence: fromSequence } : {}),
          ...(fromTime ? { from_time: fromTime } : {}),
        }),
      }
    ),

  retryDelivery: (id: string, maxAttempts?: number) =>
    request<EventDelivery>(`/deliveries/${encodeURIComponent(id)}/retry`, {
      method: "POST",
      body: JSON.stringify(
        maxAttempts && maxAttempts > 0 ? { max_attempts: maxAttempts } : {}
      ),
    }),

  retryOutbox: (id: string, maxAttempts?: number) =>
    request<EventOutboxJob>(`/outbox/${encodeURIComponent(id)}/retry`, {
      method: "POST",
      body: JSON.stringify(
        maxAttempts && maxAttempts > 0 ? { max_attempts: maxAttempts } : {}
      ),
    }),
};

// Gateway API
export const gatewayApi = {
  listRoutes: (domain?: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (domain?.trim()) {
      params.set("domain", domain.trim());
    }
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<GatewayRoute[]>(`/gateway/routes${qs ? `?${qs}` : ""}`);
  },

  getRoute: (id: string) =>
    request<GatewayRoute>(`/gateway/routes/${encodeURIComponent(id)}`),

  createRoute: (data: CreateGatewayRouteRequest) =>
    request<GatewayRoute>("/gateway/routes", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateRoute: (id: string, data: UpdateGatewayRouteRequest) =>
    request<GatewayRoute>(`/gateway/routes/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),

  deleteRoute: (id: string) =>
    request<{ status: string; id: string }>(`/gateway/routes/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  getRateLimitTemplate: () =>
    request<GatewayRateLimitTemplate>("/gateway/rate-limit-template"),

  updateRateLimitTemplate: (data: Partial<GatewayRateLimitTemplate>) =>
    request<GatewayRateLimitTemplate>("/gateway/rate-limit-template", {
      method: "PUT",
      body: JSON.stringify(data),
    }),
};

// Shared Layers API
export const layersApi = {
  list: (limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<LayerEntry[]>(`/layers${qs ? `?${qs}` : ""}`);
  },

  get: (name: string) =>
    request<LayerEntry>(`/layers/${encodeURIComponent(name)}`),

  create: (data: CreateLayerRequest) =>
    request<LayerEntry>("/layers", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  delete: (name: string) =>
    request<{ status: string; name: string }>(`/layers/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  setFunctionLayers: (functionName: string, layerIDs: string[]) =>
    request<SetFunctionLayersResponse>(`/functions/${encodeURIComponent(functionName)}/layers`, {
      method: "PUT",
      body: JSON.stringify({ layer_ids: layerIDs }),
    }),

  getFunctionLayers: (functionName: string) =>
    request<LayerEntry[]>(`/functions/${encodeURIComponent(functionName)}/layers`),
};

// Runtimes API
export const runtimesApi = {
  list: (limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (typeof limit === "number" && Number.isFinite(limit) && limit > 0) {
      params.set("limit", String(Math.floor(limit)));
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const qs = params.toString();
    return request<Runtime[]>(`/runtimes${qs ? `?${qs}` : ""}`);
  },

  create: (data: CreateRuntimeRequest) =>
    request<Runtime>("/runtimes", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  upload: async (file: File, metadata: UploadRuntimeRequest): Promise<Runtime> => {
    const formData = new FormData();
    formData.append("file", file);
    formData.append("metadata", JSON.stringify(metadata));
    const headers = new Headers();
    const tenantHeaders = getTenantScopeHeaders();
    headers.set("X-Nova-Tenant", tenantHeaders["X-Nova-Tenant"]);
    headers.set("X-Nova-Namespace", tenantHeaders["X-Nova-Namespace"]);

    const response = await fetch(`${API_BASE}/runtimes/upload`, {
      method: "POST",
      headers,
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

export interface HeatmapPoint {
  date: string;
  invocations: number;
}

// Metrics API
export const metricsApi = {
  global: () => request<GlobalMetrics>("/metrics"),
  timeseries: (range?: string) =>
    request<TimeSeriesPoint[]>(`/metrics/timeseries${range ? `?range=${range}` : ""}`),
  heatmap: (weeks: number = 52) =>
    request<HeatmapPoint[]>(`/metrics/heatmap?weeks=${weeks}`),
  stats: () => request<Record<string, unknown>>("/stats"),
};

// Invocations API (global history)
export const invocationsApi = {
  list: (limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return request<LogEntry[]>(`/invocations?${params.toString()}`);
  },
};

// Async invocations API (global scope)
export const asyncInvocationsApi = {
  list: (limit: number = 100, status?: AsyncInvocationStatus | AsyncInvocationStatus[], offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (status) {
      params.set("status", Array.isArray(status) ? status.join(",") : status);
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return request<AsyncInvocationJob[]>(`/async-invocations?${params.toString()}`);
  },

  get: (id: string) =>
    request<AsyncInvocationJob>(`/async-invocations/${encodeURIComponent(id)}`),

  retry: (id: string, maxAttempts?: number) =>
    request<AsyncInvocationJob>(`/async-invocations/${encodeURIComponent(id)}/retry`, {
      method: "POST",
      body: JSON.stringify(
        maxAttempts && maxAttempts > 0 ? { max_attempts: maxAttempts } : {}
      ),
    }),
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
    request<ScheduleEntry>(`/functions/${encodeURIComponent(functionName)}/schedules/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),

  updateCron: (functionName: string, id: string, cronExpression: string) =>
    request<ScheduleEntry>(`/functions/${encodeURIComponent(functionName)}/schedules/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ cron_expression: cronExpression }),
    }),
};

export { ApiError };
