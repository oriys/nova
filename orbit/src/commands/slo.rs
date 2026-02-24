use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum SloCmd {
    /// Get SLO policy for a function
    Get { name: String },
    /// Set SLO policy for a function
    Set {
        name: String,
        /// Target P99 latency in milliseconds
        #[arg(long)]
        target_p99_ms: Option<u64>,
        /// Target success rate (0.0 - 1.0)
        #[arg(long)]
        target_success_rate: Option<f64>,
        /// Evaluation window in seconds
        #[arg(long)]
        evaluation_window: Option<u64>,
    },
    /// Delete SLO policy for a function
    Delete { name: String },
}

const SLO_COLUMNS: &[Column] = &[
    Column::new("Function", "function_name"),
    Column::new("Target P99 (ms)", "target_p99_ms"),
    Column::new("Target Success Rate", "target_success_rate"),
    Column::new("Evaluation Window (s)", "evaluation_window"),
];

pub async fn run(cmd: SloCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        SloCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/slo")).await?;
            output::render_single(&result, SLO_COLUMNS, output_format);
        }
        SloCmd::Set {
            name,
            target_p99_ms,
            target_success_rate,
            evaluation_window,
        } => {
            let mut body = json!({});
            if let Some(v) = target_p99_ms {
                body["target_p99_ms"] = json!(v);
            }
            if let Some(v) = target_success_rate {
                body["target_success_rate"] = json!(v);
            }
            if let Some(v) = evaluation_window {
                body["evaluation_window"] = json!(v);
            }
            let result = client
                .put(&format!("/functions/{name}/slo"), &body)
                .await?;
            output::render_single(&result, SLO_COLUMNS, output_format);
        }
        SloCmd::Delete { name } => {
            client.delete(&format!("/functions/{name}/slo")).await?;
            output::print_success(&format!("SLO policy deleted for '{name}'."));
        }
    }
    Ok(())
}
