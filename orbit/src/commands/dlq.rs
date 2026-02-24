use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum DlqCmd {
    /// List dead letter queue entries
    List,
    /// Retry all dead letter queue entries
    RetryAll,
}

const DLQ_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Function", "function_name"),
    Column::new("Status", "status"),
    Column::new("Created At", "created_at"),
];

pub async fn run(cmd: DlqCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        DlqCmd::List => {
            let result = client.get("/async-invocations/dlq").await?;
            output::render(&result, DLQ_COLUMNS, output_format);
        }
        DlqCmd::RetryAll => {
            let result = client
                .post("/async-invocations/dlq/retry-all", &json!({}))
                .await?;
            output::print_success("All DLQ entries queued for retry.");
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
    }
    Ok(())
}
