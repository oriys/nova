use clap::Subcommand;
use serde_json::{json, Value};
use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

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

fn parse_env_vars(env_vars: &[String]) -> Value {
    let mut map = serde_json::Map::new();
    for item in env_vars {
        if let Some((k, v)) = item.split_once('=') {
            map.insert(k.to_string(), Value::String(v.to_string()));
        }
    }
    Value::Object(map)
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
            mode,
            env_vars,
        } => {
            let code_value = match (&code, &code_path) {
                (Some(c), _) => Some(Value::String(c.clone())),
                (_, Some(path)) => {
                    let content = std::fs::read_to_string(path)
                        .map_err(|e| crate::error::OrbitError::Input(format!("Cannot read file {path}: {e}")))?;
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
            mode,
            env_vars,
        } => {
            let mut body = json!({});
            if let Some(h) = handler {
                body["handler"] = json!(h);
            }
            if let Some(m) = memory {
                body["memory_mb"] = json!(m);
            }
            if let Some(t) = timeout {
                body["timeout_s"] = json!(t);
            }
            if let Some(c) = code {
                body["code"] = json!(c);
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
        FunctionsCmd::Invoke { name, payload, payload_file } => {
            crate::commands::invoke::run_invoke(&name, payload, payload_file, client, output_format).await?;
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
