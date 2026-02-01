// Frontend types and transformers
// Converts between Nova backend types and frontend display types

import type { NovaFunction, LogEntry as ApiLogEntry, FunctionMetrics, Runtime } from "./api";

// Frontend display types
export interface FunctionData {
  id: string;
  name: string;
  runtime: string;
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
  codePath?: string;
  minReplicas?: number;
  maxReplicas?: number;
  mode?: string;
  envVars?: Record<string, string>;
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
}

// Runtime display names
const RUNTIME_DISPLAY_NAMES: Record<string, string> = {
  python: "Python 3.11",
  go: "Go 1.21",
  rust: "Rust 1.75",
  wasm: "WebAssembly",
  node: "Node.js 20.x",
  ruby: "Ruby 3.3",
  java: "Java 21",
  deno: "Deno 1.40",
  bun: "Bun 1.0",
};

// Runtime icons for display
const RUNTIME_ICONS: Record<string, string> = {
  python: "python",
  go: "go",
  rust: "rust",
  wasm: "wasm",
  node: "nodejs",
  ruby: "ruby",
  java: "java",
  deno: "deno",
  bun: "bun",
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
    status,
    memory: fn.memory_mb,
    timeout: fn.timeout_s,
    invocations,
    errors,
    avgDuration: Math.round(avgDuration),
    lastModified: fn.updated_at,
    region: "local", // Nova runs locally
    handler: fn.handler,
    codePath: fn.code_path,
    minReplicas: fn.min_replicas,
    maxReplicas: fn.max_replicas,
    mode: fn.mode,
    envVars: fn.env_vars,
  };
}

// Transform backend log entry to frontend display format
export function transformLog(log: ApiLogEntry): LogEntry {
  // Determine log level based on success and content
  let level: "info" | "warn" | "error" | "debug" = "info";
  if (!log.success || log.stderr) {
    level = "error";
  } else if (log.stdout.toLowerCase().includes("warn")) {
    level = "warn";
  } else if (log.stdout.toLowerCase().includes("debug")) {
    level = "debug";
  }

  // Combine stdout and stderr for message
  const message = log.stderr || log.stdout || "Function executed";

  return {
    id: log.request_id,
    functionId: log.function_id,
    functionName: log.function_name,
    timestamp: log.created_at,
    level,
    message,
    requestId: log.request_id,
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
