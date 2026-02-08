use indicatif::{ProgressBar, ProgressStyle};
use serde_json::{json, Value};
use std::time::Duration;
use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

const INVOKE_COLUMNS: &[Column] = &[
    Column::new("Request ID", "request_id"),
    Column::new("Duration (ms)", "duration_ms"),
    Column::new("Cold Start", "cold_start"),
    Column::new("Version", "version"),
    Column::new("Output", "output"),
    Column::new("Error", "error"),
];

const ASYNC_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Function", "function_name"),
    Column::new("Status", "status"),
    Column::new("Attempts", "max_attempts"),
    Column::new("Created", "created_at"),
];

pub async fn run_invoke(
    name: &str,
    payload: Option<String>,
    payload_file: Option<String>,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    let body: Value = match (payload, payload_file) {
        (Some(p), _) => serde_json::from_str(&p)
            .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON payload: {e}")))?,
        (_, Some(path)) => {
            let content = std::fs::read_to_string(&path)
                .map_err(|e| crate::error::OrbitError::Input(format!("Cannot read file {path}: {e}")))?;
            serde_json::from_str(&content)
                .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON in file: {e}")))?
        }
        _ => json!({}),
    };

    let spinner = ProgressBar::new_spinner();
    spinner.set_style(ProgressStyle::default_spinner().template("{spinner:.cyan} Invoking {msg}...").unwrap());
    spinner.set_message(name.to_string());
    spinner.enable_steady_tick(Duration::from_millis(80));

    let result = client.post(&format!("/functions/{name}/invoke"), &body).await?;
    spinner.finish_and_clear();

    output::render_single(&result, INVOKE_COLUMNS, output_format);
    Ok(())
}

pub async fn run_invoke_async(
    name: &str,
    payload: Option<String>,
    max_attempts: Option<i64>,
    idempotency_key: Option<String>,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    let mut body = json!({});
    if let Some(p) = payload {
        let parsed: Value = serde_json::from_str(&p)
            .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON payload: {e}")))?;
        body["payload"] = parsed;
    }
    if let Some(m) = max_attempts {
        body["max_attempts"] = json!(m);
    }
    if let Some(k) = idempotency_key {
        body["idempotency_key"] = json!(k);
    }

    let result = client.post(&format!("/functions/{name}/invoke-async"), &body).await?;
    output::render_single(&result, ASYNC_COLUMNS, output_format);
    Ok(())
}
