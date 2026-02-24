use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum VolumesCmd {
    /// Create a volume
    Create {
        /// Volume name
        name: String,
        /// Size in MB
        #[arg(long)]
        size_mb: Option<u64>,
        /// Description
        #[arg(long)]
        description: Option<String>,
    },
    /// List all volumes
    List,
    /// Get volume details
    Get { name: String },
    /// Delete a volume
    Delete { name: String },
}

#[derive(Subcommand)]
pub enum MountsCmd {
    /// Get mounts for a function
    Get { name: String },
    /// Set mount for a function
    Set {
        name: String,
        /// Volume name
        #[arg(long)]
        volume: String,
        /// Mount path inside the function
        #[arg(long)]
        mount_path: String,
    },
}

const VOLUME_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Size MB", "size_mb"),
    Column::new("Description", "description"),
];

const MOUNT_COLUMNS: &[Column] = &[
    Column::new("Volume", "volume"),
    Column::new("Mount Path", "mount_path"),
];

pub async fn run(cmd: VolumesCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        VolumesCmd::Create {
            name,
            size_mb,
            description,
        } => {
            let mut body = json!({ "name": name });
            if let Some(v) = size_mb {
                body["size_mb"] = json!(v);
            }
            if let Some(v) = description {
                body["description"] = json!(v);
            }
            let result = client.post("/volumes", &body).await?;
            output::render_single(&result, VOLUME_COLUMNS, output_format);
        }
        VolumesCmd::List => {
            let result = client.get("/volumes").await?;
            output::render(&result, VOLUME_COLUMNS, output_format);
        }
        VolumesCmd::Get { name } => {
            let result = client.get(&format!("/volumes/{name}")).await?;
            output::render_single(&result, VOLUME_COLUMNS, output_format);
        }
        VolumesCmd::Delete { name } => {
            client.delete(&format!("/volumes/{name}")).await?;
            output::print_success(&format!("Volume '{name}' deleted."));
        }
    }
    Ok(())
}

pub async fn run_mounts(cmd: MountsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        MountsCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/mounts")).await?;
            output::render(&result, MOUNT_COLUMNS, output_format);
        }
        MountsCmd::Set {
            name,
            volume,
            mount_path,
        } => {
            let body = json!({ "volume": volume, "mount_path": mount_path });
            let result = client
                .put(&format!("/functions/{name}/mounts"), &body)
                .await?;
            output::render_single(&result, MOUNT_COLUMNS, output_format);
        }
    }
    Ok(())
}
