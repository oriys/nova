use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::{Value, json};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};

#[derive(Subcommand)]
pub enum FunctionsCmd {
    /// Create a new function
    Create {
        /// Function name
        #[arg(long)]
        name: String,
        /// Runtime (python, go, rust, node, etc.)
        #[arg(long)]
        runtime: String,
        /// Source code (inline string)
        #[arg(long)]
        code: Option<String>,
        /// Path to code file
        #[arg(long)]
        code_path: Option<String>,
        /// Handler entry point
        #[arg(long)]
        handler: Option<String>,
        /// Memory in MB
        #[arg(long)]
        memory: Option<i64>,
        /// Timeout in seconds
        #[arg(long)]
        timeout: Option<i64>,
        /// Minimum replicas
        #[arg(long)]
        min_replicas: Option<i64>,
        /// Maximum replicas (0 = unlimited)
        #[arg(long)]
        max_replicas: Option<i64>,
        /// Maximum in-flight requests per instance
        #[arg(long)]
        instance_concurrency: Option<i64>,
        /// CPU limit (vCPU)
        #[arg(long)]
        vcpus: Option<i64>,
        /// Disk IOPS limit (0 = unlimited)
        #[arg(long)]
        disk_iops: Option<i64>,
        /// Disk bandwidth limit in bytes/s (0 = unlimited)
        #[arg(long)]
        disk_bandwidth: Option<i64>,
        /// Network RX bandwidth limit in bytes/s (0 = unlimited)
        #[arg(long)]
        net_rx_bandwidth: Option<i64>,
        /// Network TX bandwidth limit in bytes/s (0 = unlimited)
        #[arg(long)]
        net_tx_bandwidth: Option<i64>,
        /// Execution mode (process or persistent)
        #[arg(long)]
        mode: Option<String>,
        /// Environment variables (KEY=VAL)
        #[arg(long = "env", value_name = "KEY=VAL")]
        env_vars: Vec<String>,
    },
    /// List all functions
    List {
        /// Search filter
        #[arg(long)]
        search: Option<String>,
        /// Limit results
        #[arg(long)]
        limit: Option<u32>,
    },
    /// Get function details
    Get {
        /// Function name
        name: String,
    },
    /// Update a function
    Update {
        /// Function name
        name: String,
        /// Handler entry point
        #[arg(long)]
        handler: Option<String>,
        /// Memory in MB
        #[arg(long)]
        memory: Option<i64>,
        /// Timeout in seconds
        #[arg(long)]
        timeout: Option<i64>,
        /// Source code (inline string)
        #[arg(long)]
        code: Option<String>,
        /// Path to code file
        #[arg(long)]
        code_path: Option<String>,
        /// Minimum replicas
        #[arg(long)]
        min_replicas: Option<i64>,
        /// Maximum replicas (0 = unlimited)
        #[arg(long)]
        max_replicas: Option<i64>,
        /// Maximum in-flight requests per instance
        #[arg(long)]
        instance_concurrency: Option<i64>,
        /// CPU limit (vCPU)
        #[arg(long)]
        vcpus: Option<i64>,
        /// Disk IOPS limit (0 = unlimited)
        #[arg(long)]
        disk_iops: Option<i64>,
        /// Disk bandwidth limit in bytes/s (0 = unlimited)
        #[arg(long)]
        disk_bandwidth: Option<i64>,
        /// Network RX bandwidth limit in bytes/s (0 = unlimited)
        #[arg(long)]
        net_rx_bandwidth: Option<i64>,
        /// Network TX bandwidth limit in bytes/s (0 = unlimited)
        #[arg(long)]
        net_tx_bandwidth: Option<i64>,
        /// Execution mode
        #[arg(long)]
        mode: Option<String>,
        /// Environment variables (KEY=VAL)
        #[arg(long = "env", value_name = "KEY=VAL")]
        env_vars: Vec<String>,
    },
    /// Delete a function
    Delete {
        /// Function name
        name: String,
    },
    /// Manage function code
    Code {
        #[command(subcommand)]
        cmd: CodeSubCmd,
    },
    /// Pull remote function source to local directory
    Pull {
        /// Function name
        name: String,
        /// Local output directory root
        #[arg(long, default_value = ".orbit/functions")]
        output_dir: String,
        /// Overwrite existing local directory
        #[arg(long)]
        force: bool,
        /// Run local test immediately after pull
        #[arg(long)]
        test: bool,
        /// JSON payload for local test (defaults to {})
        #[arg(long)]
        payload: Option<String>,
        /// Path to JSON payload file for local test
        #[arg(long)]
        payload_file: Option<String>,
    },
    /// List function files
    Files {
        /// Function name
        name: String,
    },
    /// Manage function versions
    Versions {
        #[command(subcommand)]
        cmd: VersionsSubCmd,
    },
    /// Invoke a function
    Invoke {
        /// Function name
        name: String,
        /// JSON payload
        #[arg(long)]
        payload: Option<String>,
        /// Path to payload file
        #[arg(long)]
        payload_file: Option<String>,
    },
    /// Invoke a function asynchronously
    InvokeAsync {
        /// Function name
        name: String,
        /// JSON payload
        #[arg(long)]
        payload: Option<String>,
        /// Max retry attempts
        #[arg(long)]
        max_attempts: Option<i64>,
        /// Idempotency key
        #[arg(long)]
        idempotency_key: Option<String>,
    },
    /// Manage async invocations
    AsyncInvocations {
        #[command(subcommand)]
        cmd: AsyncInvocationsSubCmd,
    },
    /// Get function logs
    Logs {
        /// Function name
        name: String,
        /// Last N logs
        #[arg(long)]
        tail: Option<u32>,
        /// Filter by request ID
        #[arg(long)]
        request_id: Option<String>,
    },
    /// Get function metrics
    Metrics {
        /// Function name
        name: String,
        /// Time range (e.g. 1h, 5m, 1d)
        #[arg(long)]
        range: Option<String>,
    },
    /// Get function invocation heatmap
    Heatmap {
        /// Function name
        name: String,
        /// Number of weeks
        #[arg(long, default_value = "52")]
        weeks: u32,
    },
    /// Manage auto-scaling policy
    Scaling {
        #[command(subcommand)]
        cmd: ScalingSubCmd,
    },
    /// Manage capacity policy
    Capacity {
        #[command(subcommand)]
        cmd: CapacitySubCmd,
    },
    /// Manage schedules
    Schedules {
        #[command(subcommand)]
        cmd: SchedulesSubCmd,
    },
    /// Manage snapshots
    Snapshot {
        #[command(subcommand)]
        cmd: SnapshotSubCmd,
    },
    /// Manage function layers
    Layers {
        #[command(subcommand)]
        cmd: FnLayersSubCmd,
    },
}

#[derive(Subcommand)]
pub enum CodeSubCmd {
    /// Get function source code
    Get { name: String },
    /// Update function code
    Update {
        name: String,
        /// Inline code string
        #[arg(long)]
        code: Option<String>,
        /// Path to code file
        #[arg(long)]
        file: Option<String>,
    },
}

#[derive(Subcommand)]
pub enum VersionsSubCmd {
    /// List function versions
    List { name: String },
    /// Get specific version
    Get { name: String, version: u32 },
}

#[derive(Subcommand)]
pub enum AsyncInvocationsSubCmd {
    /// List async invocations for a function
    List {
        name: String,
        #[arg(long)]
        limit: Option<u32>,
        #[arg(long)]
        status: Option<String>,
    },
}

#[derive(Subcommand)]
pub enum ScalingSubCmd {
    /// Get scaling policy
    Get { name: String },
    /// Set scaling policy
    Set {
        name: String,
        #[arg(long)]
        min_replicas: Option<i64>,
        #[arg(long)]
        max_replicas: Option<i64>,
        #[arg(long)]
        target_utilization: Option<f64>,
        #[arg(long)]
        cooldown_up: Option<i64>,
        #[arg(long)]
        cooldown_down: Option<i64>,
    },
    /// Delete scaling policy
    Delete { name: String },
}

#[derive(Subcommand)]
pub enum CapacitySubCmd {
    /// Get capacity policy
    Get { name: String },
    /// Set capacity policy
    Set {
        name: String,
        #[arg(long)]
        max_inflight: Option<i64>,
        #[arg(long)]
        max_queue_depth: Option<i64>,
        #[arg(long)]
        max_queue_wait_ms: Option<i64>,
        #[arg(long)]
        shed_status_code: Option<i64>,
    },
    /// Delete capacity policy
    Delete { name: String },
}

#[derive(Subcommand)]
pub enum SchedulesSubCmd {
    /// Create a schedule
    Create {
        name: String,
        /// Cron expression
        #[arg(long)]
        cron: String,
        /// Input JSON
        #[arg(long)]
        input: Option<String>,
    },
    /// List schedules
    List { name: String },
    /// Delete a schedule
    Delete {
        name: String,
        /// Schedule ID
        id: String,
    },
    /// Update (toggle) a schedule
    Update {
        name: String,
        /// Schedule ID
        id: String,
        /// Enable or disable
        #[arg(long)]
        enabled: Option<bool>,
    },
}

#[derive(Subcommand)]
pub enum SnapshotSubCmd {
    /// Create a snapshot
    Create { name: String },
    /// Delete a snapshot
    Delete { name: String },
}

#[derive(Subcommand)]
pub enum FnLayersSubCmd {
    /// Set layers for a function
    Set {
        name: String,
        /// Layer names
        #[arg(long = "layer")]
        layers: Vec<String>,
    },
    /// Get layers for a function
    Get { name: String },
}

const FN_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Runtime", "runtime"),
    Column::new("Memory", "memory_mb"),
    Column::new("Timeout", "timeout_s"),
    Column::new("Mode", "mode"),
    Column::wide("Handler", "handler"),
    Column::wide("Version", "version"),
    Column::wide("Created", "created_at"),
];

const FN_DETAIL_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Runtime", "runtime"),
    Column::new("Handler", "handler"),
    Column::new("Memory (MB)", "memory_mb"),
    Column::new("Timeout (s)", "timeout_s"),
    Column::new("Mode", "mode"),
    Column::new("Version", "version"),
    Column::new("Code Hash", "code_hash"),
    Column::new("Min Replicas", "min_replicas"),
    Column::new("Max Replicas", "max_replicas"),
    Column::new("Created", "created_at"),
    Column::new("Updated", "updated_at"),
];

const FN_PULL_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Runtime", "runtime"),
    Column::new("Handler", "handler"),
    Column::new("Directory", "directory"),
    Column::wide("Source File", "source_file"),
    Column::wide("Payload File", "payload_file"),
    Column::new("Local Test", "local_test"),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum RuntimeFamily {
    Python,
    Node,
    Go,
    Rust,
    Java,
    Unknown,
}

enum LocalTestOutcome {
    Executed { command: String, output: String },
    Skipped { reason: String },
}

fn parse_env_vars(env_vars: &[String]) -> Value {
    let mut map = serde_json::Map::new();
    for item in env_vars {
        if let Some((k, v)) = item.split_once('=') {
            map.insert(k.to_string(), Value::String(v.to_string()));
        }
    }
    Value::Object(map)
}

fn parse_json_payload(payload: Option<String>, payload_file: Option<String>) -> Result<Value> {
    match (payload, payload_file) {
        (Some(p), _) => serde_json::from_str(&p)
            .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON payload: {e}"))),
        (_, Some(path)) => {
            let content = std::fs::read_to_string(&path).map_err(|e| {
                crate::error::OrbitError::Input(format!("Cannot read file {path}: {e}"))
            })?;
            serde_json::from_str(&content)
                .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON in file: {e}")))
        }
        _ => Ok(json!({})),
    }
}

fn detect_runtime_family(runtime: &str) -> RuntimeFamily {
    let rt = runtime.to_lowercase();
    if rt.contains("python") {
        RuntimeFamily::Python
    } else if rt.contains("node") || rt.contains("javascript") || rt.contains("typescript") {
        RuntimeFamily::Node
    } else if rt.contains("golang") || rt == "go" || rt.starts_with("go:") {
        RuntimeFamily::Go
    } else if rt.contains("rust") {
        RuntimeFamily::Rust
    } else if rt.contains("java") {
        RuntimeFamily::Java
    } else {
        RuntimeFamily::Unknown
    }
}

fn source_file_rel_path(runtime: &str, handler: &str) -> String {
    let family = detect_runtime_family(runtime);
    let module = handler
        .rsplit_once('.')
        .map(|(left, _)| left)
        .filter(|left| !left.is_empty());

    match family {
        RuntimeFamily::Python => {
            if let Some(m) = module {
                format!("{}.py", m.replace('.', "/"))
            } else {
                "main.py".to_string()
            }
        }
        RuntimeFamily::Node => {
            if let Some(m) = module {
                format!("{}.js", m.replace('.', "/"))
            } else {
                "index.js".to_string()
            }
        }
        RuntimeFamily::Go => "main.go".to_string(),
        RuntimeFamily::Rust => "src/main.rs".to_string(),
        RuntimeFamily::Java => "Main.java".to_string(),
        RuntimeFamily::Unknown => "function.txt".to_string(),
    }
}

fn find_available_binary(candidates: &[&str]) -> Option<String> {
    for candidate in candidates {
        let status = Command::new(candidate)
            .arg("--version")
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status();

        if status.map(|s| s.success()).unwrap_or(false) {
            return Some((*candidate).to_string());
        }
    }
    None
}

fn ensure_toolchain(runtime: &str, family: RuntimeFamily) -> Result<String> {
    let missing_error = |label: &str, hint: &str| {
        crate::error::OrbitError::Input(format!(
            "Missing runtime toolchain for '{runtime}' ({label}). Install first.\n{hint}"
        ))
    };

    match family {
        RuntimeFamily::Python => find_available_binary(&["python3", "python"]).ok_or_else(|| {
            missing_error(
                "python",
                "Install Python 3.10+.\n  macOS: brew install python\n  Ubuntu/Debian: sudo apt-get install -y python3",
            )
        }),
        RuntimeFamily::Node => find_available_binary(&["node"]).ok_or_else(|| {
            missing_error(
                "node",
                "Install Node.js 18+.\n  macOS: brew install node\n  Ubuntu/Debian: sudo apt-get install -y nodejs npm",
            )
        }),
        RuntimeFamily::Go => find_available_binary(&["go"]).ok_or_else(|| {
            missing_error(
                "go",
                "Install Go 1.22+.\n  macOS: brew install go\n  Ubuntu/Debian: sudo apt-get install -y golang-go",
            )
        }),
        RuntimeFamily::Rust => find_available_binary(&["cargo", "rustc"]).ok_or_else(|| {
            missing_error(
                "rust",
                "Install Rust toolchain.\n  macOS/Linux: curl https://sh.rustup.rs -sSf | sh",
            )
        }),
        RuntimeFamily::Java => find_available_binary(&["java"]).ok_or_else(|| {
            missing_error(
                "java",
                "Install JDK 17+.\n  macOS: brew install openjdk@17\n  Ubuntu/Debian: sudo apt-get install -y openjdk-17-jdk",
            )
        }),
        RuntimeFamily::Unknown => Err(crate::error::OrbitError::Input(format!(
            "Runtime '{runtime}' is not recognized. Pull succeeded, but local test needs a known runtime toolchain."
        ))),
    }
}

fn run_python_local_test(
    python_cmd: &str,
    source_path: &Path,
    handler: &str,
    payload_path: &Path,
) -> Result<String> {
    let handler_name = handler.rsplit('.').next().unwrap_or(handler);
    let script = r#"
import importlib.util, json, pathlib, sys

source_path = pathlib.Path(sys.argv[1]).resolve()
handler_name = sys.argv[2]
payload_path = pathlib.Path(sys.argv[3]).resolve()

spec = importlib.util.spec_from_file_location("orbit_local_fn", source_path)
if spec is None or spec.loader is None:
    raise RuntimeError(f"failed to load module from {source_path}")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

fn = getattr(module, handler_name, None)
if fn is None:
    raise RuntimeError(f"handler '{handler_name}' not found in {source_path}")

with payload_path.open("r", encoding="utf-8") as fp:
    payload = json.load(fp)

try:
    result = fn(payload, {})
except TypeError:
    result = fn(payload)

print(json.dumps(result, ensure_ascii=False, default=str))
"#;

    let output = Command::new(python_cmd)
        .arg("-c")
        .arg(script)
        .arg(source_path.to_string_lossy().to_string())
        .arg(handler_name)
        .arg(payload_path.to_string_lossy().to_string())
        .output()?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        return Err(crate::error::OrbitError::Input(format!(
            "Local python test failed: {}",
            if stderr.is_empty() {
                "unknown error".to_string()
            } else {
                stderr
            }
        )));
    }

    Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
}

fn run_node_local_test(
    node_cmd: &str,
    source_path: &Path,
    handler: &str,
    payload_path: &Path,
) -> Result<String> {
    let script = r#"
const fs = require("fs");
const path = require("path");

(async () => {
  const sourcePath = process.env.ORBIT_SOURCE_PATH;
  const payloadPath = process.env.ORBIT_PAYLOAD_PATH;
  const handlerRaw = process.env.ORBIT_HANDLER || "handler";
  const payload = JSON.parse(fs.readFileSync(payloadPath, "utf8"));

  const parts = handlerRaw.split(".");
  const exportName = parts.length > 1 ? parts[parts.length - 1] : handlerRaw;
  let modulePath = sourcePath;
  if (parts.length > 1) {
    const moduleName = parts.slice(0, -1).join(".");
    modulePath = path.resolve(path.dirname(sourcePath), moduleName.replace(/\./g, "/") + ".js");
  }

  const mod = require(modulePath);
  const fn = mod[exportName] || mod.default || mod.handler;

  if (typeof fn !== "function") {
    throw new Error(`handler '${exportName}' not found in ${modulePath}`);
  }

  const result = await fn(payload, {});
  process.stdout.write(JSON.stringify(result, null, 2));
})().catch((err) => {
  const msg = err && err.stack ? err.stack : String(err);
  process.stderr.write(msg);
  process.exit(1);
});
"#;

    let output = Command::new(node_cmd)
        .arg("-e")
        .arg(script)
        .env(
            "ORBIT_SOURCE_PATH",
            source_path.to_string_lossy().to_string(),
        )
        .env(
            "ORBIT_PAYLOAD_PATH",
            payload_path.to_string_lossy().to_string(),
        )
        .env("ORBIT_HANDLER", handler)
        .output()?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        return Err(crate::error::OrbitError::Input(format!(
            "Local node test failed: {}",
            if stderr.is_empty() {
                "unknown error".to_string()
            } else {
                stderr
            }
        )));
    }

    Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
}

fn run_local_test(
    runtime: &str,
    handler: &str,
    source_path: &Path,
    payload_path: &Path,
) -> Result<LocalTestOutcome> {
    let family = detect_runtime_family(runtime);
    let tool = ensure_toolchain(runtime, family)?;

    match family {
        RuntimeFamily::Python => {
            let output = run_python_local_test(&tool, source_path, handler, payload_path)?;
            Ok(LocalTestOutcome::Executed {
                command: format!("{tool} <inline-runner>"),
                output,
            })
        }
        RuntimeFamily::Node => {
            let output = run_node_local_test(&tool, source_path, handler, payload_path)?;
            Ok(LocalTestOutcome::Executed {
                command: format!("{tool} -e <inline-runner>"),
                output,
            })
        }
        RuntimeFamily::Go => Ok(LocalTestOutcome::Skipped {
            reason: format!(
                "Toolchain '{tool}' is installed. Auto local runner is currently available for python/node runtimes only. Run go tests manually in {}.",
                source_path.parent().unwrap_or(Path::new(".")).display()
            ),
        }),
        RuntimeFamily::Rust => Ok(LocalTestOutcome::Skipped {
            reason: format!(
                "Toolchain '{tool}' is installed. Auto local runner is currently available for python/node runtimes only. Run cargo commands manually in {}.",
                source_path.parent().unwrap_or(Path::new(".")).display()
            ),
        }),
        RuntimeFamily::Java => Ok(LocalTestOutcome::Skipped {
            reason: format!(
                "Toolchain '{tool}' is installed. Auto local runner is currently available for python/node runtimes only. Compile/run manually from {}.",
                source_path.parent().unwrap_or(Path::new(".")).display()
            ),
        }),
        RuntimeFamily::Unknown => Ok(LocalTestOutcome::Skipped {
            reason: format!(
                "Runtime '{runtime}' is not recognized for auto local test. Source has been pulled for manual testing."
            ),
        }),
    }
}

async fn run_pull(
    name: String,
    output_dir: String,
    force: bool,
    test: bool,
    payload: Option<String>,
    payload_file: Option<String>,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    let fn_info = client.get(&format!("/functions/{name}")).await?;
    let code_info = client.get(&format!("/functions/{name}/code")).await?;

    let runtime = fn_info
        .get("runtime")
        .and_then(|v| v.as_str())
        .unwrap_or("unknown")
        .to_string();
    let handler = fn_info
        .get("handler")
        .and_then(|v| v.as_str())
        .unwrap_or("handler")
        .to_string();

    let source_code = code_info
        .get("source_code")
        .or_else(|| code_info.get("code"))
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();

    if source_code.trim().is_empty() {
        return Err(crate::error::OrbitError::Input(format!(
            "Function '{name}' does not have source code in control plane."
        )));
    }

    let base_dir = PathBuf::from(output_dir);
    let fn_dir = base_dir.join(&name);
    if fn_dir.exists() && !force {
        return Err(crate::error::OrbitError::Input(format!(
            "Directory '{}' already exists. Use --force to overwrite.",
            fn_dir.display()
        )));
    }
    std::fs::create_dir_all(&fn_dir)?;

    let source_rel_path = source_file_rel_path(&runtime, &handler);
    let source_path = fn_dir.join(&source_rel_path);
    if let Some(parent) = source_path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    std::fs::write(&source_path, source_code)?;

    let payload_value = parse_json_payload(payload, payload_file)?;
    let payload_path = fn_dir.join("payload.json");
    std::fs::write(&payload_path, serde_json::to_string_pretty(&payload_value)?)?;

    let metadata = json!({
        "name": name,
        "runtime": runtime,
        "handler": handler,
        "source_file": source_rel_path,
        "payload_file": "payload.json",
        "function": fn_info,
    });
    let metadata_path = fn_dir.join("function.meta.json");
    std::fs::write(&metadata_path, serde_json::to_string_pretty(&metadata)?)?;

    let mut local_test_status = "not-run".to_string();
    if test {
        match run_local_test(
            metadata["runtime"].as_str().unwrap_or("unknown"),
            metadata["handler"].as_str().unwrap_or("handler"),
            &source_path,
            &payload_path,
        )? {
            LocalTestOutcome::Executed { command, output } => {
                local_test_status = "executed".to_string();
                println!("Local test command: {command}");
                if output.is_empty() {
                    println!("Local test output: <empty>");
                } else {
                    println!("Local test output:\n{output}");
                }
            }
            LocalTestOutcome::Skipped { reason } => {
                local_test_status = "skipped".to_string();
                println!("{reason}");
            }
        }
    }

    let summary = json!({
        "name": metadata["name"],
        "runtime": metadata["runtime"],
        "handler": metadata["handler"],
        "directory": fn_dir.to_string_lossy().to_string(),
        "source_file": source_path.to_string_lossy().to_string(),
        "payload_file": payload_path.to_string_lossy().to_string(),
        "metadata_file": metadata_path.to_string_lossy().to_string(),
        "local_test": local_test_status,
    });
    output::render_single(&summary, FN_PULL_COLUMNS, output_format);

    Ok(())
}

pub async fn run(cmd: FunctionsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        FunctionsCmd::Create {
            name,
            runtime,
            code,
            code_path,
            handler,
            memory,
            timeout,
            min_replicas,
            max_replicas,
            instance_concurrency,
            vcpus,
            disk_iops,
            disk_bandwidth,
            net_rx_bandwidth,
            net_tx_bandwidth,
            mode,
            env_vars,
        } => {
            let code_value = match (&code, &code_path) {
                (Some(c), _) => Some(Value::String(c.clone())),
                (_, Some(path)) => {
                    let content = std::fs::read_to_string(path).map_err(|e| {
                        crate::error::OrbitError::Input(format!("Cannot read file {path}: {e}"))
                    })?;
                    Some(Value::String(content))
                }
                _ => None,
            };

            let mut body = json!({
                "name": name,
                "runtime": runtime,
            });
            if let Some(c) = code_value {
                body["code"] = c;
            }
            if let Some(cp) = &code_path {
                body["code_path"] = json!(cp);
            }
            if let Some(h) = handler {
                body["handler"] = json!(h);
            }
            if let Some(m) = memory {
                body["memory_mb"] = json!(m);
            }
            if let Some(t) = timeout {
                body["timeout_s"] = json!(t);
            }
            if let Some(v) = min_replicas {
                body["min_replicas"] = json!(v);
            }
            if let Some(v) = max_replicas {
                body["max_replicas"] = json!(v);
            }
            if let Some(v) = instance_concurrency {
                body["instance_concurrency"] = json!(v);
            }
            if vcpus.is_some()
                || disk_iops.is_some()
                || disk_bandwidth.is_some()
                || net_rx_bandwidth.is_some()
                || net_tx_bandwidth.is_some()
            {
                let mut limits = json!({});
                if let Some(v) = vcpus {
                    limits["vcpus"] = json!(v);
                }
                if let Some(v) = disk_iops {
                    limits["disk_iops"] = json!(v);
                }
                if let Some(v) = disk_bandwidth {
                    limits["disk_bandwidth"] = json!(v);
                }
                if let Some(v) = net_rx_bandwidth {
                    limits["net_rx_bandwidth"] = json!(v);
                }
                if let Some(v) = net_tx_bandwidth {
                    limits["net_tx_bandwidth"] = json!(v);
                }
                body["limits"] = limits;
            }
            if let Some(m) = mode {
                body["mode"] = json!(m);
            }
            if !env_vars.is_empty() {
                body["env_vars"] = parse_env_vars(&env_vars);
            }
            let result = client.post("/functions", &body).await?;
            output::render_single(&result, FN_DETAIL_COLUMNS, output_format);
        }
        FunctionsCmd::List { search, limit } => {
            let mut path = "/functions".to_string();
            let mut params = vec![];
            if let Some(s) = search {
                params.push(format!("search={s}"));
            }
            if let Some(l) = limit {
                params.push(format!("limit={l}"));
            }
            if !params.is_empty() {
                path = format!("{}?{}", path, params.join("&"));
            }
            let result = client.get(&path).await?;
            output::render(&result, FN_COLUMNS, output_format);
        }
        FunctionsCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}")).await?;
            output::render_single(&result, FN_DETAIL_COLUMNS, output_format);
        }
        FunctionsCmd::Update {
            name,
            handler,
            memory,
            timeout,
            code,
            code_path,
            min_replicas,
            max_replicas,
            instance_concurrency,
            vcpus,
            disk_iops,
            disk_bandwidth,
            net_rx_bandwidth,
            net_tx_bandwidth,
            mode,
            env_vars,
        } => {
            let mut body = json!({});
            let code_value = match (&code, &code_path) {
                (Some(c), _) => Some(Value::String(c.clone())),
                (_, Some(path)) => {
                    let content = std::fs::read_to_string(path).map_err(|e| {
                        crate::error::OrbitError::Input(format!("Cannot read file {path}: {e}"))
                    })?;
                    Some(Value::String(content))
                }
                _ => None,
            };
            if let Some(h) = handler {
                body["handler"] = json!(h);
            }
            if let Some(m) = memory {
                body["memory_mb"] = json!(m);
            }
            if let Some(t) = timeout {
                body["timeout_s"] = json!(t);
            }
            if let Some(c) = code_value {
                body["code"] = json!(c);
            }
            if let Some(v) = min_replicas {
                body["min_replicas"] = json!(v);
            }
            if let Some(v) = max_replicas {
                body["max_replicas"] = json!(v);
            }
            if let Some(v) = instance_concurrency {
                body["instance_concurrency"] = json!(v);
            }
            if vcpus.is_some()
                || disk_iops.is_some()
                || disk_bandwidth.is_some()
                || net_rx_bandwidth.is_some()
                || net_tx_bandwidth.is_some()
            {
                let mut limits = json!({});
                if let Some(v) = vcpus {
                    limits["vcpus"] = json!(v);
                }
                if let Some(v) = disk_iops {
                    limits["disk_iops"] = json!(v);
                }
                if let Some(v) = disk_bandwidth {
                    limits["disk_bandwidth"] = json!(v);
                }
                if let Some(v) = net_rx_bandwidth {
                    limits["net_rx_bandwidth"] = json!(v);
                }
                if let Some(v) = net_tx_bandwidth {
                    limits["net_tx_bandwidth"] = json!(v);
                }
                body["limits"] = limits;
            }
            if let Some(m) = mode {
                body["mode"] = json!(m);
            }
            if !env_vars.is_empty() {
                body["env_vars"] = parse_env_vars(&env_vars);
            }
            let result = client.patch(&format!("/functions/{name}"), &body).await?;
            output::render_single(&result, FN_DETAIL_COLUMNS, output_format);
        }
        FunctionsCmd::Delete { name } => {
            client.delete(&format!("/functions/{name}")).await?;
            output::print_success(&format!("Function '{name}' deleted."));
        }
        FunctionsCmd::Code { cmd } => {
            crate::commands::code::run(cmd, client, output_format).await?;
        }
        FunctionsCmd::Pull {
            name,
            output_dir,
            force,
            test,
            payload,
            payload_file,
        } => {
            run_pull(
                name,
                output_dir,
                force,
                test,
                payload,
                payload_file,
                client,
                output_format,
            )
            .await?;
        }
        FunctionsCmd::Files { name } => {
            let result = client.get(&format!("/functions/{name}/files")).await?;
            output::render(
                &result,
                &[Column::new("File", "name"), Column::new("Size", "size")],
                output_format,
            );
        }
        FunctionsCmd::Versions { cmd } => {
            crate::commands::versions::run(cmd, client, output_format).await?;
        }
        FunctionsCmd::Invoke {
            name,
            payload,
            payload_file,
        } => {
            crate::commands::invoke::run_invoke(
                &name,
                payload,
                payload_file,
                client,
                output_format,
            )
            .await?;
        }
        FunctionsCmd::InvokeAsync {
            name,
            payload,
            max_attempts,
            idempotency_key,
        } => {
            crate::commands::invoke::run_invoke_async(
                &name,
                payload,
                max_attempts,
                idempotency_key,
                client,
                output_format,
            )
            .await?;
        }
        FunctionsCmd::AsyncInvocations { cmd } => {
            crate::commands::async_invocations::run_fn(cmd, client, output_format).await?;
        }
        FunctionsCmd::Logs {
            name,
            tail,
            request_id,
        } => {
            crate::commands::logs::run(&name, tail, request_id, client, output_format).await?;
        }
        FunctionsCmd::Metrics { name, range } => {
            crate::commands::metrics::run_fn_metrics(&name, range, client, output_format).await?;
        }
        FunctionsCmd::Heatmap { name, weeks } => {
            crate::commands::metrics::run_fn_heatmap(&name, weeks, client, output_format).await?;
        }
        FunctionsCmd::Scaling { cmd } => {
            crate::commands::scaling::run(cmd, client, output_format).await?;
        }
        FunctionsCmd::Capacity { cmd } => {
            crate::commands::capacity::run(cmd, client, output_format).await?;
        }
        FunctionsCmd::Schedules { cmd } => {
            crate::commands::schedules::run(cmd, client, output_format).await?;
        }
        FunctionsCmd::Snapshot { cmd } => {
            crate::commands::snapshots::run_fn(cmd, client, output_format).await?;
        }
        FunctionsCmd::Layers { cmd } => {
            crate::commands::layers::run_fn(cmd, client, output_format).await?;
        }
    }
    Ok(())
}
