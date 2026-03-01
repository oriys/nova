// Frontend types and transformers
// Converts between nova backend types and lumen display types

import type { NovaFunction, LogEntry as ApiLogEntry, FunctionMetrics, Runtime, CompileStatus, ResourceLimits, RolloutPolicy, AsyncDestinations } from "./api";

// Frontend display types
export interface FunctionData {
  id: string;
  name: string;
  runtime: string;
  runtimeId: string; // Base runtime ID (e.g., "python", "go")
  status: "active" | "inactive" | "error";
  memory: number;
  timeout: number;
  invocations: number;
  errors: number;
  avgDuration: number;
  lastModified: string;
  region: string;
  handler: string;
  description?: string;
  code?: string;
  codeHash?: string;
  minReplicas?: number;
  maxReplicas?: number;
  mode?: string;
  backend?: string;
  version?: number;
  envVars?: Record<string, string>;
  tags?: Record<string, string>;
  logRetentionDays?: number;
  compileStatus?: CompileStatus;
  compileError?: string;
  limits?: ResourceLimits;
  networkPolicy?: NetworkPolicy;
  rolloutPolicy?: RolloutPolicy;
  asyncDestinations?: AsyncDestinations;
}

export interface NetworkPolicy {
  isolation_mode?: string;
  ingress_rules?: IngressRule[];
  egress_rules?: EgressRule[];
  deny_external_access?: boolean;
}

export interface IngressRule {
  source: string;
  port?: number;
  protocol?: string;
}

export interface EgressRule {
  host: string;
  port?: number;
  protocol?: string;
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
  param_mapping?: ParamMapping[];
  rate_limit?: RouteRateLimit;
  cors?: CORSConfig;
  timeout_ms?: number;
  retry_policy?: RouteRetryPolicy;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export type ParamSource = "query" | "path" | "body" | "header";
export type ParamTransform = "" | "camel_case" | "snake_case" | "upper_case" | "lower_case" | "upper_first" | "kebab_case";
export type ParamType = "" | "integer" | "float" | "boolean" | "json";

export interface ParamMapping {
  source: ParamSource;
  name: string;
  target?: string;
  transform?: ParamTransform;
  type?: ParamType;
  default?: unknown;
  required?: boolean;
}

export interface CORSConfig {
  allow_origins?: string[];
  allow_methods?: string[];
  allow_headers?: string[];
  expose_headers?: string[];
  allow_credentials?: boolean;
  max_age?: number;
}

export interface RouteRateLimit {
  requests_per_second: number;
  burst_size: number;
}

export interface RouteRetryPolicy {
  max_attempts: number;
  backoff_ms?: number;
}

export interface LogEntry {
  id: string;
  functionId: string;
  functionName: string;
  timestamp: string;
  level: "info" | "warn" | "error" | "debug";
  message: string;
  requestId: string;
  duration?: number;
}

export interface RuntimeInfo {
  id: string;
  name: string;
  version: string;
  status: "available" | "deprecated" | "maintenance";
  functionsCount: number;
  icon: string;
  imageName?: string;
  entrypoint?: string[];
  fileExtension?: string;
  envVars?: Record<string, string>;
}

// Runtime display names
const RUNTIME_DISPLAY_NAMES: Record<string, string> = {
  python: "Python 3.12.12",
  node: "Node.js 24.13.0",
  go: "Go 1.25.6",
  rust: "Rust 1.93.0",
  java: "Java 21.0.10",
  ruby: "Ruby 3.4.8",
  php: "PHP 8.4.17",
  deno: "Deno 2.6.7",
  bun: "Bun 1.3.8",
  graalvm: "GraalVM 21.0.2",
};

// Runtime icons for display
const RUNTIME_ICONS: Record<string, string> = {
  python: "python",
  node: "nodejs",
  go: "go",
  rust: "rust",
  java: "java",
  ruby: "ruby",
  php: "php",
  deno: "deno",
  bun: "bun",
  graalvm: "java",
};

// Transform backend function to frontend display format
export function transformFunction(
  fn: NovaFunction,
  metrics?: FunctionMetrics
): FunctionData {
  const invocations = metrics?.invocations?.invocations ?? 0;
  const errors = metrics?.invocations?.failures ?? 0;
  const avgDuration = metrics?.invocations?.avg_ms ?? 0;

  // Determine status based on metrics and pool state
  let status: "active" | "inactive" | "error" = "inactive";
  if (metrics?.pool?.active_vms && metrics.pool.active_vms > 0) {
    status = "active";
  } else if (invocations > 0) {
    status = errors / invocations > 0.1 ? "error" : "active";
  }

  return {
    id: fn.id,
    name: fn.name,
    runtime: RUNTIME_DISPLAY_NAMES[fn.runtime] || fn.runtime,
    runtimeId: fn.runtime,
    status,
    memory: fn.memory_mb,
    timeout: fn.timeout_s,
    invocations,
    errors,
    avgDuration: Math.round(avgDuration),
    lastModified: fn.updated_at,
    region: "local", // nova runs locally
    handler: fn.handler,
    codeHash: fn.code_hash,
    minReplicas: fn.min_replicas,
    maxReplicas: fn.max_replicas,
    mode: fn.mode,
    backend: fn.backend,
    version: fn.version,
    envVars: fn.env_vars,
    tags: fn.tags,
    logRetentionDays: fn.log_retention_days,
    compileStatus: fn.compile_status,
    compileError: fn.compile_error,
    limits: fn.limits,
    networkPolicy: fn.network_policy,
    rolloutPolicy: fn.rollout_policy,
    asyncDestinations: fn.async_destinations,
  };
}

// Transform backend log entry to frontend display format
export function transformLog(log: ApiLogEntry): LogEntry {
  // Determine log level based on success and content
  let level: "info" | "warn" | "error" | "debug" = "info";
  if (!log.success || log.stderr) {
    level = "error";
  } else if (log.stdout?.toLowerCase().includes("warn")) {
    level = "warn";
  } else if (log.stdout?.toLowerCase().includes("debug")) {
    level = "debug";
  }

  // Combine stdout and stderr for message
  const message = log.stderr || log.stdout || "Function executed";

  return {
    id: log.id,
    functionId: log.function_id,
    functionName: log.function_name,
    timestamp: log.created_at,
    level,
    message,
    requestId: log.id,
    duration: log.duration_ms,
  };
}

// Transform backend runtime to frontend display format
export function transformRuntime(runtime: Runtime): RuntimeInfo {
  return {
    id: runtime.id,
    name: runtime.name,
    version: runtime.version,
    status: runtime.status,
    functionsCount: runtime.functions_count,
    icon: RUNTIME_ICONS[runtime.id] || runtime.id,
    imageName: runtime.image_name,
    entrypoint: runtime.entrypoint,
    fileExtension: runtime.file_extension,
    envVars: runtime.env_vars,
  };
}

// Convert frontend runtime display name to backend runtime ID
export function runtimeDisplayToId(display: string): string {
  for (const [id, name] of Object.entries(RUNTIME_DISPLAY_NAMES)) {
    if (name === display || display.toLowerCase().includes(id)) {
      return id;
    }
  }
  return display.toLowerCase().split(" ")[0];
}

// Get all available runtime IDs
export function getAvailableRuntimes(): string[] {
  return Object.keys(RUNTIME_DISPLAY_NAMES);
}

// Get display name for a runtime ID
export function getRuntimeDisplayName(id: string): string {
  return RUNTIME_DISPLAY_NAMES[id] || id;
}

// Replay / Time-travel types

export interface Recording {
  id: string;
  function_id: string;
  invocation_id: string;
  runtime: string;
  arch: string;
  created_at: string;
  events_count: number;
}

export interface ReplayResult {
  replay_id: string;
  status: 'success' | 'diverged' | 'failed';
  divergences: Divergence[];
  duration_ms: number;
  events_replayed: number;
}

export interface Divergence {
  event_seq: number;
  type: string;
  expected: string;
  actual: string;
  message: string;
}

export interface TimeTravelState {
  step: number;
  line: number;
  file: string;
  variables: Record<string, string>;
  call_stack: StackFrame[];
  output: string;
  completed: boolean;
}

export interface StackFrame {
  function: string;
  file: string;
  line: number;
}

// Sandbox display types

import type { Sandbox } from "./api";

export interface SandboxData {
  id: string;
  template: string;
  status: "creating" | "running" | "paused" | "stopped" | "error";
  memoryMB: number;
  vcpus: number;
  timeoutS: number;
  onIdleS: number;
  networkPolicy: string;
  envVars?: Record<string, string>;
  createdAt: string;
  lastActiveAt: string;
  expiresAt: string;
  error?: string;
}

export function transformSandbox(sb: Sandbox): SandboxData {
  return {
    id: sb.id,
    template: sb.template,
    status: sb.status,
    memoryMB: sb.memory_mb,
    vcpus: sb.vcpus,
    timeoutS: sb.timeout_s,
    onIdleS: sb.on_idle_s,
    networkPolicy: sb.network_policy,
    envVars: sb.env_vars,
    createdAt: sb.created_at,
    lastActiveAt: sb.last_active_at,
    expiresAt: sb.expires_at,
    error: sb.error,
  };
}
