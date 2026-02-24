use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum DiagnosticsCmd {
    /// Get diagnostics for a function
    Get { name: String },
    /// Run diagnostic analysis for a function
    Analyze { name: String },
    /// Get recommendations for a function
    Recommendations { name: String },
    /// Get SLO status for a function
    SloStatus { name: String },
}

const DIAGNOSTICS_COLUMNS: &[Column] = &[
    Column::new("Function", "function_name"),
    Column::new("Status", "status"),
    Column::new("Cold Starts", "cold_starts"),
    Column::new("Error Rate", "error_rate"),
    Column::new("Avg Duration (ms)", "avg_duration_ms"),
];

const RECOMMENDATION_COLUMNS: &[Column] = &[
    Column::new("Category", "category"),
    Column::new("Severity", "severity"),
    Column::new("Message", "message"),
];

const SLO_STATUS_COLUMNS: &[Column] = &[
    Column::new("Function", "function_name"),
    Column::new("P99 (ms)", "current_p99_ms"),
    Column::new("Success Rate", "current_success_rate"),
    Column::new("SLO Met", "slo_met"),
];

pub async fn run(cmd: DiagnosticsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        DiagnosticsCmd::Get { name } => {
            let result = client
                .get(&format!("/functions/{name}/diagnostics"))
                .await?;
            output::render_single(&result, DIAGNOSTICS_COLUMNS, output_format);
        }
        DiagnosticsCmd::Analyze { name } => {
            let result = client
                .post(
                    &format!("/functions/{name}/diagnostics/analyze"),
                    &json!({}),
                )
                .await?;
            output::render_single(&result, DIAGNOSTICS_COLUMNS, output_format);
        }
        DiagnosticsCmd::Recommendations { name } => {
            let result = client
                .get(&format!("/functions/{name}/recommendations"))
                .await?;
            output::render(&result, RECOMMENDATION_COLUMNS, output_format);
        }
        DiagnosticsCmd::SloStatus { name } => {
            let result = client
                .get(&format!("/functions/{name}/slo/status"))
                .await?;
            output::render_single(&result, SLO_STATUS_COLUMNS, output_format);
        }
    }
    Ok(())
}
