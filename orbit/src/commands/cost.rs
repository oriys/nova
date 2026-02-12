use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;

#[derive(Subcommand)]
pub enum CostCmd {
    /// Get cost summary across all functions
    Summary {
        /// Time window in seconds (default: 86400 = 24h)
        #[arg(long, default_value = "86400")]
        window: u64,
    },
    /// Get cost breakdown for a specific function
    Function {
        /// Function name
        name: String,
        /// Time window in seconds (default: 86400 = 24h)
        #[arg(long, default_value = "86400")]
        window: u64,
    },
}

const COST_SUMMARY_COLUMNS: &[Column] = &[
    Column::new("Function", "function_name"),
    Column::new("Invocations", "invocations"),
    Column::new("Cold Starts", "cold_starts"),
    Column::new("Compute Cost", "compute_cost"),
    Column::new("Total Cost", "total_cost"),
    Column::new("Avg Cost", "avg_cost"),
];

const FUNCTION_COST_COLUMNS: &[Column] = &[
    Column::new("Function", "function_name"),
    Column::new("Invocations", "invocations"),
    Column::new("Duration (ms)", "total_duration_ms"),
    Column::new("Cold Starts", "cold_starts"),
    Column::new("Invocation Cost", "invocations_cost"),
    Column::new("Compute Cost", "compute_cost"),
    Column::new("Cold Start Cost", "cold_start_cost"),
    Column::new("Total Cost", "total_cost"),
    Column::new("Avg Cost", "avg_cost"),
];

pub async fn run(cmd: CostCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        CostCmd::Summary { window } => {
            let result = client
                .get(&format!("/cost/summary?window={window}"))
                .await?;
            if let Some(functions) = result.get("functions") {
                output::render(functions, COST_SUMMARY_COLUMNS, output_format);
            }
            if let Some(total) = result.get("total_cost") {
                println!("\nTotal Cost: {total}");
            }
        }
        CostCmd::Function { name, window } => {
            let result = client
                .get(&format!("/functions/{name}/cost?window={window}"))
                .await?;
            output::render_single(&result, FUNCTION_COST_COLUMNS, output_format);
        }
    }
    Ok(())
}
