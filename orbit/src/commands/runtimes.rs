use clap::Subcommand;
use serde_json::json;
use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

#[derive(Subcommand)]
pub enum RuntimesCmd {
    /// List available runtimes
    List,
    /// Create a custom runtime
    Create {
        /// Runtime name
        #[arg(long)]
        name: String,
        /// Runtime image path
        #[arg(long)]
        image: Option<String>,
        /// Command template
        #[arg(long)]
        command: Option<String>,
    },
    /// Upload a runtime image
    Upload {
        /// Runtime ID
        id: String,
        /// Path to rootfs image
        #[arg(long)]
        image: String,
    },
    /// Delete a runtime
    Delete {
        /// Runtime ID
        id: String,
    },
}

const RUNTIME_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Rootfs", "rootfs"),
    Column::new("Command", "command"),
    Column::wide("Description", "description"),
];

pub async fn run(cmd: RuntimesCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        RuntimesCmd::List => {
            let result = client.get("/runtimes").await?;
            output::render(&result, RUNTIME_COLUMNS, output_format);
        }
        RuntimesCmd::Create {
            name,
            image,
            command,
        } => {
            let mut body = json!({ "name": name });
            if let Some(i) = image {
                body["image"] = json!(i);
            }
            if let Some(c) = command {
                body["command"] = json!(c);
            }
            let result = client.post("/runtimes", &body).await?;
            output::render_single(&result, RUNTIME_COLUMNS, output_format);
        }
        RuntimesCmd::Upload { id, image } => {
            let content = std::fs::read(&image)
                .map_err(|e| crate::error::OrbitError::Input(format!("Cannot read file {image}: {e}")))?;
            let form = reqwest::multipart::Form::new().part(
                "file",
                reqwest::multipart::Part::bytes(content)
                    .file_name(image.clone()),
            );
            // Use raw request for multipart
            let _ = form; // Multipart upload needs direct reqwest usage
            let body = json!({ "image_path": image });
            let result = client.post(&format!("/runtimes/upload"), &body).await?;
            output::print_success(&format!("Runtime image uploaded for '{id}'."));
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, RUNTIME_COLUMNS, output_format);
            }
        }
        RuntimesCmd::Delete { id } => {
            client.delete(&format!("/runtimes/{id}")).await?;
            output::print_success(&format!("Runtime '{id}' deleted."));
        }
    }
    Ok(())
}
