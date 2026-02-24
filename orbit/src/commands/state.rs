use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;

#[derive(Subcommand)]
pub enum StateCmd {
    /// Get function state
    Get { name: String },
    /// Put function state
    Put {
        name: String,
        /// State data as JSON string
        #[arg(long)]
        data: String,
    },
    /// Delete function state
    Delete { name: String },
}

const STATE_COLUMNS: &[Column] = &[
    Column::new("Function", "function_name"),
    Column::new("Size (bytes)", "size_bytes"),
    Column::new("Updated", "updated_at"),
];

pub async fn run(cmd: StateCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        StateCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/state")).await?;
            output::render_single(&result, STATE_COLUMNS, output_format);
        }
        StateCmd::Put { name, data } => {
            let parsed: serde_json::Value = serde_json::from_str(&data).map_err(|e| {
                crate::error::OrbitError::Input(format!("Invalid JSON data: {e}"))
            })?;
            let result = client
                .put(&format!("/functions/{name}/state"), &parsed)
                .await?;
            output::render_single(&result, STATE_COLUMNS, output_format);
        }
        StateCmd::Delete { name } => {
            client.delete(&format!("/functions/{name}/state")).await?;
            output::print_success(&format!("State deleted for '{name}'."));
        }
    }
    Ok(())
}
