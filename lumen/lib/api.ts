// nova API client
// Connects to the nova backend at /api (proxied via Next.js rewrites)
import { getTenantScopeHeaders } from "@/lib/tenant-scope";
import { filterTenantsForSession } from "@/lib/auth";

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
  backend?: string;
  limits?: ResourceLimits;
  env_vars?: Record<string, string>;
  network_policy?: NetworkPolicy;
  rollout_policy?: RolloutPolicy;
  auto_scale_policy?: AutoScalePolicy;
  capacity_policy?: CapacityPolicy;
  slo_policy?: SLOPolicy;
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
  file_count?: number;
  entry_point?: string;
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

export interface SLOObjectives {
  success_rate_pct?: number;
  p95_duration_ms?: number;
  cold_start_rate_pct?: number;
}

export interface SLONotificationTarget {
  type: string;
  url: string;
  headers?: Record<string, string>;
}

export interface SLOPolicy {
  enabled: boolean;
  window_s?: number;
  min_samples?: number;
  objectives?: SLOObjectives;
  notifications?: SLONotificationTarget[];
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

export interface MenuPermission {
  tenant_id: string;
  menu_key: string;
  enabled: boolean;
  created_at: string;
}

export interface ButtonPermission {
  tenant_id: string;
  permission_key: string;
  enabled: boolean;
  created_at: string;
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

export interface InvocationListSummary {
  total_invocations: number;
  successes: number;
  failures: number;
  cold_starts: number;
  avg_duration_ms: number;
}

export interface InvokeResponse {
  request_id: string;
  output: unknown;
  error?: string;
  duration_ms: number;
  cold_start: boolean;
  version?: number;
}

export type AsyncInvocationStatus = "queued" | "running" | "succeeded" | "dlq" | "paused";

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

export interface AsyncInvocationSummary {
  total: number;
  queued: number;
  running: number;
  paused: number;
  succeeded: number;
  dlq: number;
  backlog: number;
  pending: number;
  consumed_last_1m: number;
  consumed_last_5m: number;
  consume_rate_per_sec_1m: number;
  consume_rate_per_sec_5m: number;
  consume_rate_per_minute_1m: number;
  consume_rate_per_minute_5m: number;
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

export interface FunctionSLOStatus {
  function_id: string;
  function_name: string;
  enabled: boolean;
  policy?: SLOPolicy;
  snapshot?: {
    window_seconds: number;
    total_invocations: number;
    successes: number;
    failures: number;
    cold_starts: number;
    success_rate_pct: number;
    cold_start_rate_pct: number;
    p95_duration_ms: number;
  };
  breaches: string[];
}

// Performance Recommendations types
export interface PerformanceRecommendation {
  category: string;
  priority: string;
  current_value: any;
  recommended_value: any;
  reasoning: string;
  expected_impact: string;
  metrics?: Record<string, string>;
}

export interface PerformanceRecommendationResponse {
  recommendations: PerformanceRecommendation[];
  confidence: number;
  estimated_savings?: string;
  analysis_summary: string;
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

export type NotificationStatus = "unread" | "read" | "all";

export interface NotificationEntry {
  id: string;
  tenant_id?: string;
  namespace?: string;
  type: string;
  severity: string;
  source?: string;
  function_id?: string;
  function_name?: string;
  title: string;
  message: string;
  data?: unknown;
  status: Exclude<NotificationStatus, "all">;
  created_at: string;
  read_at?: string;
}

export interface CreateFunctionRequest {
  name: string;
  runtime: string;
  handler?: string;
  code: string; // Source code (required)
  dependency_files?: Record<string, string>; // Optional: dependency files like go.mod, requirements.txt, Cargo.toml, package.json
  memory_mb?: number;
  timeout_s?: number;
  min_replicas?: number;
  max_replicas?: number;
  mode?: string;
  backend?: string;
  env_vars?: Record<string, string>;
  limits?: ResourceLimits;
  network_policy?: NetworkPolicy;
  rollout_policy?: RolloutPolicy;
  slo_policy?: SLOPolicy;
}

export interface UpdateFunctionRequest {
  handler?: string;
  code?: string; // Source code (optional for updates)
  memory_mb?: number;
  timeout_s?: number;
  min_replicas?: number;
  max_replicas?: number;
  mode?: string;
  backend?: string;
  env_vars?: Record<string, string>;
  limits?: ResourceLimits;
  network_policy?: NetworkPolicy;
  rollout_policy?: RolloutPolicy;
  slo_policy?: SLOPolicy;
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

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: string,
    public hint?: string,
    public details?: unknown
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export interface PaginatedResult<T> {
  items: T[];
  total: number;
}

interface ApiPaginationMetadata {
  limit?: number;
  offset?: number;
  returned?: number;
  total?: number;
  has_more?: boolean;
  next_offset?: number;
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function toNonNegativeInteger(value: unknown): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  const normalized = Math.floor(value);
  return normalized >= 0 ? normalized : undefined;
}

function buildNovaHeaders(options?: RequestInit): Headers {
  const headers = new Headers(options?.headers);
  if (!(options?.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const tenantHeaders = getTenantScopeHeaders();
  headers.set("X-Nova-Tenant", tenantHeaders["X-Nova-Tenant"]);
  headers.set("X-Nova-Namespace", tenantHeaders["X-Nova-Namespace"]);
  return headers;
}

async function requestRaw(
  path: string,
  options?: RequestInit
): Promise<Response> {
  const headers = buildNovaHeaders(options);

  return fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });
}

async function parseApiError(response: Response): Promise<never> {
  const contentType = response.headers.get("content-type") || "";
  const rawBody = await response.text();
  let message = response.statusText || "Request failed";
  let code: string | undefined;
  let hint: string | undefined;
  let details: unknown;

  if (contentType.includes("application/json") && rawBody.trim()) {
    try {
      const payload = JSON.parse(rawBody) as Record<string, unknown>;
      details = payload;
      if (typeof payload.error === "string" && payload.error.trim()) {
        message = payload.error.trim();
      } else if (typeof payload.message === "string" && payload.message.trim()) {
        message = payload.message.trim();
      }
      if (typeof payload.code === "string" && payload.code.trim()) {
        code = payload.code.trim();
      }
      if (typeof payload.hint === "string" && payload.hint.trim()) {
        hint = payload.hint.trim();
      }
    } catch {
      if (rawBody.trim()) {
        message = rawBody.trim();
      }
    }
  } else if (rawBody.trim()) {
    message = rawBody.trim();
  }

  throw new ApiError(response.status, message, code, hint, details);
}

async function parseResponseBody<T>(response: Response): Promise<T> {
  if (response.status === 204 || response.status === 205) {
    return undefined as T;
  }

  const rawBody = await response.text();
  if (!rawBody.trim()) {
    return undefined as T;
  }

  const contentType = response.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    return JSON.parse(rawBody) as T;
  }

  try {
    return JSON.parse(rawBody) as T;
  } catch {
    return rawBody as unknown as T;
  }
}

async function request<T>(
  path: string,
  options?: RequestInit
): Promise<T> {
  const response = await requestRaw(path, options);
  if (!response.ok) {
    return parseApiError(response);
  }
  return parseResponseBody<T>(response);
}

async function requestPaged<T>(
  path: string,
  options?: RequestInit
): Promise<PaginatedResult<T>> {
  const response = await requestRaw(path, options);
  if (!response.ok) {
    return parseApiError(response);
  }

  const payload = await parseResponseBody<unknown>(response);
  if (isObjectRecord(payload) && Array.isArray(payload.items)) {
    const envelope = payload as { items: unknown; pagination?: unknown };
    const items = envelope.items as T[];
    const pagination = isObjectRecord(envelope.pagination)
      ? (envelope.pagination as ApiPaginationMetadata)
      : undefined;
    const total = toNonNegativeInteger(pagination?.total);
    if (total === undefined) {
      throw new ApiError(response.status, "Invalid paginated response: missing pagination.total");
    }
    return { items, total };
  }

  throw new ApiError(response.status, "Invalid paginated response");
}

// Cost Intelligence types
export interface FunctionCostSummary {
  function_id: string;
  function_name: string;
  total_cost: number;
  invocations_cost: number;
  compute_cost: number;
  cold_start_cost: number;
  invocations: number;
  total_duration_ms: number;
  cold_starts: number;
  avg_cost: number;
}

export interface TenantCostSummary {
  tenant_id: string;
  total_cost: number;
  functions: FunctionCostSummary[];
  period_from: string;
  period_to: string;
}

// Functions API
export const functionsApi = {
  list: async (search?: string, limit?: number, offset?: number, runtime?: string) => {
    const params = new URLSearchParams();
    if (search) params.set("search", search);
    if (runtime) params.set("runtime", runtime);
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<NovaFunction>(`/functions?${params.toString()}`);
    return result.items;
  },

  listPage: (search?: string, limit: number = 20, offset: number = 0, runtime?: string) => {
    const params = new URLSearchParams();
    if (search) params.set("search", search);
    if (runtime) params.set("runtime", runtime);
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<NovaFunction>(`/functions?${params.toString()}`);
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

  listAsyncInvocationsPage: (
    name: string,
    limit: number = 50,
    status?: AsyncInvocationStatus | AsyncInvocationStatus[],
    offset?: number
  ) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (status) {
      params.set("status", Array.isArray(status) ? status.join(",") : status);
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<AsyncInvocationJob>(
      `/functions/${encodeURIComponent(name)}/async-invocations?${params.toString()}`
    );
  },

  listAsyncInvocations: async (
    name: string,
    limit: number = 50,
    status?: AsyncInvocationStatus | AsyncInvocationStatus[],
    offset?: number
  ) => {
    const result = await functionsApi.listAsyncInvocationsPage(name, limit, status, offset);
    return result.items;
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
    functionsApi.logsPage(name, tail, 0).then((result) => result.items),

  logsPage: (name: string, limit: number = 20, offset: number = 0) => {
    const params = new URLSearchParams();
    params.set("tail", String(Math.max(1, Math.floor(limit))));
    if (offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<LogEntry>(`/functions/${encodeURIComponent(name)}/logs?${params.toString()}`);
  },

  logsByRequest: (name: string, requestID: string) =>
    request<LogEntry>(`/functions/${encodeURIComponent(name)}/logs?request_id=${encodeURIComponent(requestID)}`),

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

  updateCodeWithFiles: (name: string, code: string, dependencyFiles: Record<string, string>, entryPoint?: string) =>
    request<UpdateCodeResponse>(`/functions/${encodeURIComponent(name)}/code`, {
      method: "PUT",
      body: JSON.stringify({
        code,
        dependency_files: dependencyFiles,
        ...(entryPoint ? { entry_point: entryPoint } : {}),
      }),
    }),

  listVersions: async (name: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<FunctionVersionEntry>(
      `/functions/${encodeURIComponent(name)}/versions?${params.toString()}`
    );
    return result.items;
  },

  getVersion: (name: string, version: number) =>
    request<FunctionVersionEntry>(`/functions/${encodeURIComponent(name)}/versions/${version}`),

  heatmap: (name: string, weeks: number = 52) =>
    request<HeatmapPoint[]>(`/functions/${encodeURIComponent(name)}/heatmap?weeks=${weeks}`),

  recommendations: (name: string, days: number = 7) =>
    request<PerformanceRecommendationResponse>(
      `/functions/${encodeURIComponent(name)}/recommendations?days=${Math.max(1, Math.floor(days))}`
    ),

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

  getSLOPolicy: (name: string) =>
    request<SLOPolicy>(`/functions/${encodeURIComponent(name)}/slo`),

  setSLOPolicy: (name: string, data: SLOPolicy) =>
    request<SLOPolicy>(`/functions/${encodeURIComponent(name)}/slo`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteSLOPolicy: (name: string) =>
    request<{ status: string; function: string }>(`/functions/${encodeURIComponent(name)}/slo`, {
      method: "DELETE",
    }),

  sloStatus: (name: string) =>
    request<FunctionSLOStatus>(`/functions/${encodeURIComponent(name)}/slo/status`),

  getTestSuite: (name: string) =>
    request<TestSuiteRecord>(`/functions/${encodeURIComponent(name)}/test-suite`),

  saveTestSuite: (name: string, testCases: TestSuiteCase[]) =>
    request<TestSuiteRecord>(`/functions/${encodeURIComponent(name)}/test-suite`, {
      method: "PUT",
      body: JSON.stringify({ test_cases: testCases }),
    }),

  deleteTestSuite: (name: string) =>
    request<{ status: string; function_name: string }>(`/functions/${encodeURIComponent(name)}/test-suite`, {
      method: "DELETE",
    }),
};

// Tenant and namespace management API
export const tenantsApi = {
  list: async (limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<TenantEntry>(`/tenants?${params.toString()}`);
    const tenantList = result.items;
    return filterTenantsForSession(tenantList);
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

  listNamespaces: async (tenantID: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<NamespaceEntry>(
      `/tenants/${encodeURIComponent(tenantID)}/namespaces?${params.toString()}`
    );
    return result.items;
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

  listQuotas: async (tenantID: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<TenantQuotaEntry>(
      `/tenants/${encodeURIComponent(tenantID)}/quotas?${params.toString()}`
    );
    return result.items;
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

  usage: async (tenantID: string, refresh: boolean = true, limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("refresh", refresh ? "true" : "false");
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<TenantUsageEntry>(
      `/tenants/${encodeURIComponent(tenantID)}/usage?${params.toString()}`
    );
    return result.items;
  },

  listMenuPermissions: async (tenantID: string, limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<MenuPermission>(
      `/tenants/${encodeURIComponent(tenantID)}/menu-permissions?${params.toString()}`
    );
    return result.items;
  },

  upsertMenuPermission: (
    tenantID: string,
    menuKey: string,
    enabled: boolean
  ) =>
    request<MenuPermission>(
      `/tenants/${encodeURIComponent(tenantID)}/menu-permissions/${encodeURIComponent(menuKey)}`,
      { method: "PUT", body: JSON.stringify({ enabled }) }
    ),

  deleteMenuPermission: (tenantID: string, menuKey: string) =>
    request<{ status: string }>(
      `/tenants/${encodeURIComponent(tenantID)}/menu-permissions/${encodeURIComponent(menuKey)}`,
      { method: "DELETE" }
    ),

  listButtonPermissions: async (tenantID: string, limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<ButtonPermission>(
      `/tenants/${encodeURIComponent(tenantID)}/button-permissions?${params.toString()}`
    );
    return result.items;
  },

  upsertButtonPermission: (
    tenantID: string,
    permissionKey: string,
    enabled: boolean
  ) =>
    request<ButtonPermission>(
      `/tenants/${encodeURIComponent(tenantID)}/button-permissions/${encodeURIComponent(permissionKey)}`,
      { method: "PUT", body: JSON.stringify({ enabled }) }
    ),

  deleteButtonPermission: (tenantID: string, permissionKey: string) =>
    request<{ status: string }>(
      `/tenants/${encodeURIComponent(tenantID)}/button-permissions/${encodeURIComponent(permissionKey)}`,
      { method: "DELETE" }
    ),
};

// Event bus API
export const eventsApi = {
  listTopics: async (limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<EventTopic>(`/topics?${params.toString()}`);
    return result.items;
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
    return requestPaged<EventOutboxJob>(
      `/topics/${encodeURIComponent(topicName)}/outbox?${params.toString()}`
    ).then((result) => result.items);
  },

  listMessages: async (topicName: string, limit: number = 50, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<EventMessage>(
      `/topics/${encodeURIComponent(topicName)}/messages?${params.toString()}`
    );
    return result.items;
  },

  listSubscriptions: async (topicName: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<EventSubscription>(
      `/topics/${encodeURIComponent(topicName)}/subscriptions?${params.toString()}`
    );
    return result.items;
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
    return requestPaged<EventDelivery>(
      `/subscriptions/${encodeURIComponent(subscriptionID)}/deliveries?${params.toString()}`
    ).then((result) => result.items);
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
  listRoutes: async (domain?: string, limit?: number, offset?: number) => {
    const result = await gatewayApi.listRoutesPage(domain, limit, offset);
    return result.items;
  },

  listRoutesPage: (domain?: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (domain?.trim()) {
      params.set("domain", domain.trim());
    }
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<GatewayRoute>(`/gateway/routes?${params.toString()}`);
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
  list: async (limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<LayerEntry>(`/layers?${params.toString()}`);
    return result.items;
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

  getFunctionLayers: async (functionName: string, limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<LayerEntry>(
      `/functions/${encodeURIComponent(functionName)}/layers?${params.toString()}`
    );
    return result.items;
  },
};

// Runtimes API
export const runtimesApi = {
  list: async (limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<Runtime>(`/runtimes?${params.toString()}`);
    return result.items;
  },

  listPage: (limit: number = 20, offset: number = 0) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<Runtime>(`/runtimes?${params.toString()}`);
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

// Backend detection types
export interface BackendInfo {
  name: string;
  available: boolean;
  reason?: string;
}

export interface BackendsResponse {
  backends: BackendInfo[];
  default_backend: string;
}

export const backendsApi = {
  list: () => request<BackendsResponse>("/backends"),
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
  list: async (limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<LogEntry>(`/invocations?${params.toString()}`);
    return result.items;
  },

  listPage: (
    limit: number = 20,
    offset: number = 0,
    options?: {
      search?: string;
      functionName?: string;
      status?: "all" | "success" | "failed";
    }
  ) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    if (options?.search) {
      params.set("search", options.search);
    }
    if (options?.functionName && options.functionName !== "all") {
      params.set("function", options.functionName);
    }
    if (options?.status && options.status !== "all") {
      params.set("status", options.status);
    }
    return request<{
      items?: LogEntry[];
      pagination?: ApiPaginationMetadata;
      summary?: Partial<InvocationListSummary>;
    }>(`/invocations?${params.toString()}`).then((payload) => {
      const items = Array.isArray(payload?.items) ? payload.items : [];
      const total = toNonNegativeInteger(payload?.pagination?.total) ?? items.length;
      const fallback = summarizeInvocationEntries(items);
      const summary = payload?.summary ?? {};
      return {
        items,
        total,
        summary: {
          total_invocations: toNonNegativeInteger(summary.total_invocations) ?? total,
          successes: toNonNegativeInteger(summary.successes) ?? fallback.successes,
          failures: toNonNegativeInteger(summary.failures) ?? fallback.failures,
          cold_starts: toNonNegativeInteger(summary.cold_starts) ?? fallback.cold_starts,
          avg_duration_ms: toNonNegativeInteger(summary.avg_duration_ms) ?? fallback.avg_duration_ms,
        } as InvocationListSummary,
      };
    });
  },
};

function summarizeInvocationEntries(entries: LogEntry[]): InvocationListSummary {
  if (entries.length === 0) {
    return {
      total_invocations: 0,
      successes: 0,
      failures: 0,
      cold_starts: 0,
      avg_duration_ms: 0,
    };
  }

  let successes = 0;
  let failures = 0;
  let coldStarts = 0;
  let totalDuration = 0;
  for (const entry of entries) {
    if (entry.success) {
      successes += 1;
    } else {
      failures += 1;
    }
    if (entry.cold_start) {
      coldStarts += 1;
    }
    totalDuration += Number.isFinite(entry.duration_ms) ? entry.duration_ms : 0;
  }

  return {
    total_invocations: entries.length,
    successes,
    failures,
    cold_starts: coldStarts,
    avg_duration_ms: Math.round(totalDuration / entries.length),
  };
}

// Async invocations API (global scope)
export const asyncInvocationsApi = {
  listPage: (
    limit: number = 100,
    status?: AsyncInvocationStatus | AsyncInvocationStatus[],
    offset?: number
  ) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (status) {
      params.set("status", Array.isArray(status) ? status.join(",") : status);
    }
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<AsyncInvocationJob>(`/async-invocations?${params.toString()}`);
  },

  list: async (limit: number = 100, status?: AsyncInvocationStatus | AsyncInvocationStatus[], offset?: number) => {
    const result = await asyncInvocationsApi.listPage(limit, status, offset);
    return result.items;
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

  pause: (id: string) =>
    request<AsyncInvocationJob>(`/async-invocations/${encodeURIComponent(id)}/pause`, {
      method: "POST",
    }),

  resume: (id: string) =>
    request<AsyncInvocationJob>(`/async-invocations/${encodeURIComponent(id)}/resume`, {
      method: "POST",
    }),

  delete: (id: string) =>
    request<void>(`/async-invocations/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  summary: () => request<AsyncInvocationSummary>("/async-invocations/summary"),
};

// Health API
export const healthApi = {
  check: () => request<HealthStatus>("/health"),
  ready: () => request<{ status: string }>("/health/ready"),
  live: () => request<{ status: string }>("/health/live"),
};

// Notifications API (for header bell menu)
export const notificationsApi = {
  list: async (status: NotificationStatus = "all", limit: number = 20, offset?: number) => {
    const params = new URLSearchParams();
    params.set("status", status);
    params.set("limit", String(limit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<NotificationEntry>(`/notifications?${params.toString()}`);
    return result.items;
  },

  unreadCount: () => request<{ unread: number }>("/notifications/unread-count"),

  markRead: (id: string) =>
    request<NotificationEntry>(`/notifications/${encodeURIComponent(id)}/read`, {
      method: "POST",
      body: JSON.stringify({}),
    }),

  markAllRead: () =>
    request<{ updated: number }>("/notifications/read-all", {
      method: "POST",
      body: JSON.stringify({}),
    }),
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
    snapshotsApi.listPage(100, 0).then((result) => result.items),

  listPage: (limit: number = 20, offset: number = 0) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<{
      function_id: string;
      function_name: string;
      snap_size: number;
      mem_size: number;
      total_size: number;
      created_at: string;
    }>(`/snapshots?${params.toString()}`);
  },

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

export type NodeType = "function" | "sub_workflow";

export interface WorkflowNode {
  id: string;
  version_id: string;
  node_key: string;
  node_type: NodeType;
  function_name: string;
  workflow_name?: string;
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
  node_type: NodeType;
  function_name: string;
  workflow_name?: string;
  child_run_id?: string;
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
  node_type?: NodeType;
  function_name?: string;
  workflow_name?: string;
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
  list: async (limit: number = 100, offset?: number) => {
    const result = await workflowsApi.listPage(limit, offset);
    return result.items;
  },

  listPage: (limit: number = 20, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<Workflow>(`/workflows?${params.toString()}`);
  },

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

  listVersions: async (name: string, limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<WorkflowVersion>(
      `/workflows/${encodeURIComponent(name)}/versions?${params.toString()}`
    );
    return result.items;
  },

  getVersion: (name: string, version: number) =>
    request<WorkflowVersion>(`/workflows/${encodeURIComponent(name)}/versions/${version}`),

  publishVersion: (name: string, def: PublishVersionRequest) =>
    request<WorkflowVersion>(`/workflows/${encodeURIComponent(name)}/versions`, {
      method: "POST",
      body: JSON.stringify(def),
    }),

  listRuns: async (name: string, limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<WorkflowRun>(
      `/workflows/${encodeURIComponent(name)}/runs?${params.toString()}`
    );
    return result.items;
  },

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
  list: async (limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<APIKeyEntry>(`/apikeys?${params.toString()}`);
    return result.items;
  },

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
  list: async (limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<SecretEntry>(`/secrets?${params.toString()}`);
    return result.items;
  },

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
  list: async (functionName: string, limit: number = 100, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<ScheduleEntry>(
      `/functions/${encodeURIComponent(functionName)}/schedules?${params.toString()}`
    );
    return result.items;
  },

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

// AI types
export interface AIGenerateRequest {
  description: string;
  runtime: string;
}

export interface AIGenerateResponse {
  code: string;
  explanation?: string;
  function_name?: string;
}

export interface AIReviewRequest {
  code: string;
  runtime: string;
  include_security?: boolean;
  include_compliance?: boolean;
}

export interface SecurityIssue {
  severity: string;
  type: string;
  description: string;
  line_number?: number;
  remediation: string;
}

export interface ComplianceIssue {
  standard: string;
  violation: string;
  description: string;
  severity: string;
}

export interface AIReviewResponse {
  feedback: string;
  suggestions?: string[];
  score?: number;
  security_issues?: SecurityIssue[];
  compliance_issues?: ComplianceIssue[];
}

export interface AIRewriteRequest {
  code: string;
  runtime: string;
  instructions?: string;
}

export interface AIRewriteResponse {
  code: string;
  explanation?: string;
}

export interface AIStatusResponse {
  enabled: boolean;
}

export interface AIConfigResponse {
  enabled: boolean;
  api_key: string;
  model: string;
  base_url: string;
  prompt_dir?: string;
}

export interface AIConfigUpdateRequest {
  enabled?: boolean;
  api_key?: string;
  model?: string;
  base_url?: string;
  prompt_dir?: string;
}

export interface AIModelEntry {
  id: string;
  object: string;
  created: number;
  owned_by: string;
}

export interface AIPromptTemplateMeta {
  name: string;
  label: string;
  file: string;
  description: string;
  customized: boolean;
}

export interface AIPromptTemplate extends AIPromptTemplateMeta {
  content: string;
}

export interface AIPromptTemplateUpdateRequest {
  content: string;
}

// Diagnostics Analysis types
export interface DiagnosticsErrorSample {
  timestamp: string;
  error_message: string;
  duration_ms: number;
  cold_start: boolean;
}

export interface DiagnosticsSlowSample {
  timestamp: string;
  duration_ms: number;
  cold_start: boolean;
}

export interface DiagnosticsRecommendation {
  category: string;
  priority: string;
  action: string;
  expected_impact: string;
}

export interface DiagnosticsAnomaly {
  type: string;
  severity: string;
  description: string;
}

export interface DiagnosticsAnalysisResponse {
  summary: string;
  root_causes: string[];
  recommendations: DiagnosticsRecommendation[];
  anomalies: DiagnosticsAnomaly[];
  performance_score: number;
}

// Test Suite types
export interface TestSuiteRecord {
  function_name: string;
  test_cases: TestSuiteCase[];
  updated_at: string;
  created_at: string;
}

export interface TestSuiteCase {
  id: string;
  name: string;
  input: string;
  expectedOutput: string;
}

export interface GenerateTestsRequest {
  function_name: string;
  runtime: string;
  code: string;
  handler?: string;
}

export interface GeneratedTestCase {
  name: string;
  input: string;
  expected_output: string;
}

export interface GenerateTestsResponse {
  test_cases: GeneratedTestCase[];
}

// AI API
export const aiApi = {
  status: () => request<AIStatusResponse>("/ai/status"),

  getConfig: () => request<AIConfigResponse>("/ai/config"),

  updateConfig: (data: AIConfigUpdateRequest) =>
    request<AIConfigResponse>("/ai/config", {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  listModelsPage: (limit: number = 100, offset: number = 0) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<AIModelEntry>(`/ai/models?${params.toString()}`);
  },

  listModels: async (limit: number = 100, offset: number = 0) => {
    const result = await aiApi.listModelsPage(limit, offset);
    return result.items;
  },

  listPromptTemplatesPage: (limit: number = 100, offset: number = 0) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    return requestPaged<AIPromptTemplateMeta>(`/ai/prompts?${params.toString()}`);
  },

  listPromptTemplates: async (limit: number = 100, offset: number = 0) => {
    const result = await aiApi.listPromptTemplatesPage(limit, offset);
    return result.items;
  },

  getPromptTemplate: (name: string) =>
    request<AIPromptTemplate>(`/ai/prompts/${encodeURIComponent(name)}`),

  updatePromptTemplate: (name: string, data: AIPromptTemplateUpdateRequest) =>
    request<AIPromptTemplate>(`/ai/prompts/${encodeURIComponent(name)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  generate: (data: AIGenerateRequest) =>
    request<AIGenerateResponse>("/ai/generate", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  review: (data: AIReviewRequest) =>
    request<AIReviewResponse>("/ai/review", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  rewrite: (data: AIRewriteRequest) =>
    request<AIRewriteResponse>("/ai/rewrite", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  analyzeDiagnostics: (functionName: string, params?: { window?: string }) =>
    request<DiagnosticsAnalysisResponse>(
      `/functions/${functionName}/diagnostics/analyze${params?.window ? `?window=${params.window}` : ""}`,
      {
        method: "POST",
      }
    ),

  generateTests: (data: GenerateTestsRequest) =>
    request<GenerateTestsResponse>("/ai/generate-tests", {
      method: "POST",
      body: JSON.stringify(data),
    }),
};

// ─── RBAC Types ─────────────────────────────────────────────────────────────

export interface RBACRole {
  id: string;
  tenant_id: string;
  name: string;
  is_system: boolean;
  created_at: string;
  updated_at: string;
}

export interface RBACPermission {
  id: string;
  code: string;
  resource_type: string;
  action: string;
  description: string;
  created_at: string;
}

export interface RBACRoleAssignment {
  id: string;
  tenant_id: string;
  principal_type: string;
  principal_id: string;
  role_id: string;
  scope_type: string;
  scope_id: string;
  created_by: string;
  created_at: string;
}

// ─── RBAC API ───────────────────────────────────────────────────────────────

export const rbacApi = {
  // Roles
  listRoles: async (params?: { tenant_id?: string; limit?: number; offset?: number }) => {
    const query = new URLSearchParams(
      Object.entries(params ?? {})
        .filter(([, v]) => v !== undefined)
        .map(([k, v]) => [k, String(v)])
    );
    if (!query.has("limit")) {
      query.set("limit", "100");
    }
    const result = await requestPaged<RBACRole>(`/rbac/roles?${query.toString()}`);
    return result.items;
  },

  getRole: (id: string) => request<RBACRole>(`/rbac/roles/${id}`),

  createRole: (data: { id: string; tenant_id?: string; name: string; is_system?: boolean }) =>
    request<RBACRole>("/rbac/roles", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteRole: (id: string) =>
    request<{ status: string; id: string }>(`/rbac/roles/${id}`, {
      method: "DELETE",
    }),

  // Permissions
  listPermissions: async (params?: { limit?: number; offset?: number }) => {
    const query = new URLSearchParams(
      Object.entries(params ?? {})
        .filter(([, v]) => v !== undefined)
        .map(([k, v]) => [k, String(v)])
    );
    if (!query.has("limit")) {
      query.set("limit", "100");
    }
    const result = await requestPaged<RBACPermission>(`/rbac/permissions?${query.toString()}`);
    return result.items;
  },

  getPermission: (id: string) => request<RBACPermission>(`/rbac/permissions/${id}`),

  createPermission: (data: {
    id: string;
    code: string;
    resource_type?: string;
    action?: string;
    description?: string;
  }) =>
    request<RBACPermission>("/rbac/permissions", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deletePermission: (id: string) =>
    request<{ status: string; id: string }>(`/rbac/permissions/${id}`, {
      method: "DELETE",
    }),

  // Role ↔ Permission mapping
  listRolePermissions: async (roleId: string, limit: number = 100, offset?: number) => {
    const query = new URLSearchParams();
    query.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      query.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<RBACPermission>(`/rbac/roles/${roleId}/permissions?${query.toString()}`);
    return result.items;
  },

  assignPermissionToRole: (roleId: string, permissionId: string) =>
    request<{ status: string; role_id: string; permission_id: string }>(
      `/rbac/roles/${roleId}/permissions`,
      {
        method: "POST",
        body: JSON.stringify({ permission_id: permissionId }),
      }
    ),

  revokePermissionFromRole: (roleId: string, permissionId: string) =>
    request<{ status: string; role_id: string; permission_id: string }>(
      `/rbac/roles/${roleId}/permissions/${permissionId}`,
      {
        method: "DELETE",
      }
    ),

  // Role Assignments
  listRoleAssignments: async (params?: {
    tenant_id?: string;
    principal_type?: string;
    principal_id?: string;
    limit?: number;
    offset?: number;
  }) => {
    const query = new URLSearchParams(
      Object.entries(params ?? {})
        .filter(([, v]) => v !== undefined)
        .map(([k, v]) => [k, String(v)])
    );
    if (!query.has("limit")) {
      query.set("limit", "100");
    }
    const result = await requestPaged<RBACRoleAssignment>(`/rbac/assignments?${query.toString()}`);
    return result.items;
  },

  getRoleAssignment: (id: string) =>
    request<RBACRoleAssignment>(`/rbac/assignments/${id}`),

  createRoleAssignment: (data: {
    id: string;
    tenant_id?: string;
    principal_type: string;
    principal_id: string;
    role_id: string;
    scope_type: string;
    scope_id?: string;
    created_by?: string;
  }) =>
    request<RBACRoleAssignment>("/rbac/assignments", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteRoleAssignment: (id: string) =>
    request<{ status: string; id: string }>(`/rbac/assignments/${id}`, {
      method: "DELETE",
    }),
};

// ─── API Docs Types ─────────────────────────────────────────────────────────

export interface DocField {
  name: string;
  type: string;
  required: boolean;
  description: string;
  default?: string;
  example?: string;
  validation?: string;
  enum_values?: string;
}

export interface DocStatusCode {
  code: number;
  meaning: string;
}

export interface DocErrorModel {
  format: string;
  retryable: string;
  description: string;
}

export interface GenerateDocsRequest {
  function_name: string;
  runtime: string;
  code: string;
  handler: string;
  method?: string;
  path?: string;
}

export interface GenerateWorkflowDocsRequest {
  workflow_name: string;
  description?: string;
  nodes: string;
  edges: string;
}

export interface GenerateDocsResponse {
  name: string;
  operation_id: string;
  service: string;
  version: string;
  protocol: string;
  stability: string;
  summary: string;
  method: string;
  path: string;
  content_type: string;
  auth: string;
  request_fields: DocField[];
  response_fields: DocField[];
  success_codes: DocStatusCode[];
  error_codes: DocStatusCode[];
  error_model: DocErrorModel;
  curl_example: string;
  request_example: string;
  response_example: string;
  error_example: string;
  auth_method: string;
  roles_required: string[];
  idempotent: boolean;
  idempotent_key: string;
  rate_limit: string;
  timeout: string;
  pagination: string;
  supports_tracing: boolean;
  changelog: string[];
  notes: string[];
}

export interface APIDocShare {
  id: string;
  tenant_id: string;
  namespace: string;
  function_name: string;
  title: string;
  token: string;
  doc_content: GenerateDocsResponse;
  created_by: string;
  expires_at?: string;
  access_count: number;
  last_access_at?: string;
  created_at: string;
}

export interface CreateShareRequest {
  function_name: string;
  title: string;
  doc_content: GenerateDocsResponse;
  expires_in?: string;
}

export interface CreateShareResponse {
  id: string;
  token: string;
  share_url: string;
  expires_at?: string;
  created_at: string;
}

export const apiDocsApi = {
  generateDocs: (data: GenerateDocsRequest) =>
    request<GenerateDocsResponse>("/ai/generate-docs", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  generateWorkflowDocs: (data: GenerateWorkflowDocsRequest) =>
    request<GenerateDocsResponse>("/ai/generate-workflow-docs", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  createShare: (data: CreateShareRequest) =>
    request<CreateShareResponse>("/api-docs/shares", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  listShares: async (limit: number = 50, offset?: number) => {
    const params = new URLSearchParams();
    params.set("limit", String(Math.max(1, Math.floor(limit))));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<APIDocShare>(`/api-docs/shares?${params.toString()}`);
    return result.items;
  },

  deleteShare: (id: string) =>
    request<{ status: string; id: string }>(`/api-docs/shares/${id}`, {
      method: "DELETE",
    }),

  getSharedDoc: (token: string) =>
    request<APIDocShare>(`/api-docs/shared/${token}`),
};

// Per-function persisted documentation
export interface FunctionDocRecord {
  function_name: string;
  doc_content: GenerateDocsResponse;
  updated_at: string;
  created_at: string;
}

export const functionDocsApi = {
  get: (functionName: string) =>
    request<FunctionDocRecord>(`/functions/${functionName}/docs`),

  save: (functionName: string, docContent: GenerateDocsResponse) =>
    request<FunctionDocRecord>(`/functions/${functionName}/docs`, {
      method: "PUT",
      body: JSON.stringify({ doc_content: docContent }),
    }),

  delete: (functionName: string) =>
    request<{ status: string; function_name: string }>(`/functions/${functionName}/docs`, {
      method: "DELETE",
    }),
};

// Per-workflow persisted documentation
export interface WorkflowDocRecord {
  workflow_name: string;
  doc_content: GenerateDocsResponse;
  updated_at: string;
  created_at: string;
}

export const workflowDocsApi = {
  get: (workflowName: string) =>
    request<WorkflowDocRecord>(`/workflows/${encodeURIComponent(workflowName)}/docs`),

  save: (workflowName: string, docContent: GenerateDocsResponse) =>
    request<WorkflowDocRecord>(`/workflows/${encodeURIComponent(workflowName)}/docs`, {
      method: "PUT",
      body: JSON.stringify({ doc_content: docContent }),
    }),

  delete: (workflowName: string) =>
    request<{ status: string; workflow_name: string }>(`/workflows/${encodeURIComponent(workflowName)}/docs`, {
      method: "DELETE",
    }),
};

// Volumes API
export interface VolumeEntry {
  id: string;
  tenant_id?: string;
  namespace?: string;
  name: string;
  size_mb: number;
  image_path: string;
  shared: boolean;
  description?: string;
  created_at: string;
  updated_at: string;
}

export const volumesApi = {
  list: async (limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    const resolvedLimit =
      typeof limit === "number" && Number.isFinite(limit) && limit > 0
        ? Math.floor(limit)
        : 100;
    params.set("limit", String(resolvedLimit));
    if (typeof offset === "number" && Number.isFinite(offset) && offset > 0) {
      params.set("offset", String(Math.floor(offset)));
    }
    const result = await requestPaged<VolumeEntry>(`/volumes?${params.toString()}`);
    return result.items;
  },

  get: (name: string) =>
    request<VolumeEntry>(`/volumes/${encodeURIComponent(name)}`),

  create: (data: { name: string; size_mb: number; shared?: boolean; description?: string }) =>
    request<VolumeEntry>("/volumes", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  delete: (name: string) =>
    request<void>(`/volumes/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
};

// Cost Intelligence API
export const costApi = {
  functionCost: (name: string, windowSeconds?: number) => {
    const params = new URLSearchParams();
    if (typeof windowSeconds === "number" && Number.isFinite(windowSeconds) && windowSeconds > 0) {
      params.set("window", String(windowSeconds));
    }
    const qs = params.toString();
    return request<FunctionCostSummary>(`/functions/${encodeURIComponent(name)}/cost${qs ? `?${qs}` : ""}`);
  },

  summary: (windowSeconds?: number) => {
    const params = new URLSearchParams();
    if (typeof windowSeconds === "number" && Number.isFinite(windowSeconds) && windowSeconds > 0) {
      params.set("window", String(windowSeconds));
    }
    const qs = params.toString();
    return request<TenantCostSummary>(`/cost/summary${qs ? `?${qs}` : ""}`);
  },
};
