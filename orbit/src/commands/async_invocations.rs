use crate::client::NovaClient;
use crate::commands::functions::AsyncInvocationsSubCmd;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;

const ASYNC_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Function", "function_name"),
    Column::new("Status", "status"),
    Column::new("Attempts", "max_attempts"),
    Column::wide("Idempotency Key", "idempotency_key"),
    Column::new("Created", "created_at"),
    Column::wide("Updated", "updated_at"),
];

#[derive(Subcommand)]
pub enum GlobalAsyncCmd {
    /// List all async invocations
    List {
        #[arg(long)]
        limit: Option<u32>,
        #[arg(long)]
        status: Option<String>,
    },
    /// Get async invocation details
    Get { id: String },
    /// Retry a failed async invocation
    Retry { id: String },
}

pub async fn run_fn(
    cmd: AsyncInvocationsSubCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        AsyncInvocationsSubCmd::List {
            name,
            limit,
            status,
        } => {
            let mut path = format!("/functions/{name}/async-invocations");
            let mut params = vec![];
            if let Some(l) = limit {
                params.push(format!("limit={l}"));
            }
            if let Some(s) = status {
                params.push(format!("status={s}"));
            }
            if !params.is_empty() {
                path = format!("{}?{}", path, params.join("&"));
            }
            let result = client.get(&path).await?;
            output::render(&result, ASYNC_COLUMNS, output_format);
        }
    }
    Ok(())
}

pub async fn run_global(
    cmd: GlobalAsyncCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        GlobalAsyncCmd::List { limit, status } => {
            let mut path = "/async-invocations".to_string();
            let mut params = vec![];
            if let Some(l) = limit {
                params.push(format!("limit={l}"));
            }
            if let Some(s) = status {
                params.push(format!("status={s}"));
            }
            if !params.is_empty() {
                path = format!("{}?{}", path, params.join("&"));
            }
            let result = client.get(&path).await?;
            output::render(&result, ASYNC_COLUMNS, output_format);
        }
        GlobalAsyncCmd::Get { id } => {
            let result = client.get(&format!("/async-invocations/{id}")).await?;
            output::render_single(&result, ASYNC_COLUMNS, output_format);
        }
        GlobalAsyncCmd::Retry { id } => {
            let result = client
                .post(
                    &format!("/async-invocations/{id}/retry"),
                    &serde_json::json!({}),
                )
                .await?;
            output::render_single(&result, ASYNC_COLUMNS, output_format);
        }
    }
    Ok(())
}
