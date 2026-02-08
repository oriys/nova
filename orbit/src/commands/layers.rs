use crate::client::NovaClient;
use crate::commands::functions::FnLayersSubCmd;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum LayersCmd {
    /// Create a layer
    Create {
        #[arg(long)]
        name: String,
        #[arg(long)]
        runtime: String,
        #[arg(long)]
        version: Option<String>,
    },
    /// List layers
    List,
    /// Get layer details
    Get { name: String },
    /// Delete a layer
    Delete { name: String },
}

const LAYER_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Runtime", "runtime"),
    Column::new("Version", "version"),
    Column::new("Size (MB)", "size_mb"),
    Column::wide("Files", "files"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: LayersCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        LayersCmd::Create {
            name,
            runtime,
            version,
        } => {
            let mut body = json!({ "name": name, "runtime": runtime });
            if let Some(v) = version {
                body["version"] = json!(v);
            }
            let result = client.post("/layers", &body).await?;
            output::render_single(&result, LAYER_COLUMNS, output_format);
        }
        LayersCmd::List => {
            let result = client.get("/layers").await?;
            output::render(&result, LAYER_COLUMNS, output_format);
        }
        LayersCmd::Get { name } => {
            let result = client.get(&format!("/layers/{name}")).await?;
            output::render_single(&result, LAYER_COLUMNS, output_format);
        }
        LayersCmd::Delete { name } => {
            client.delete(&format!("/layers/{name}")).await?;
            output::print_success(&format!("Layer '{name}' deleted."));
        }
    }
    Ok(())
}

pub async fn run_fn(cmd: FnLayersSubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        FnLayersSubCmd::Set { name, layers } => {
            let body = json!({ "layers": layers });
            let result = client
                .put(&format!("/functions/{name}/layers"), &body)
                .await?;
            output::print_success(&format!("Layers set for '{name}'."));
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
        FnLayersSubCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/layers")).await?;
            output::render(&result, LAYER_COLUMNS, output_format);
        }
    }
    Ok(())
}
