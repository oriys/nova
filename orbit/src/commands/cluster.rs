use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;

#[derive(Subcommand)]
pub enum ClusterCmd {
    /// List cluster nodes
    List,
    /// List healthy cluster nodes
    Healthy,
    /// Get a cluster node
    Get { id: String },
    /// Delete a cluster node
    Delete { id: String },
}

const NODE_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Address", "address"),
    Column::new("Status", "status"),
    Column::new("Last Heartbeat", "last_heartbeat"),
];

pub async fn run(cmd: ClusterCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        ClusterCmd::List => {
            let result = client.get("/cluster/nodes").await?;
            output::render(&result, NODE_COLUMNS, output_format);
        }
        ClusterCmd::Healthy => {
            let result = client.get("/cluster/nodes/healthy").await?;
            output::render(&result, NODE_COLUMNS, output_format);
        }
        ClusterCmd::Get { id } => {
            let result = client.get(&format!("/cluster/nodes/{id}")).await?;
            output::render_single(&result, NODE_COLUMNS, output_format);
        }
        ClusterCmd::Delete { id } => {
            client.delete(&format!("/cluster/nodes/{id}")).await?;
            output::print_success(&format!("Node '{id}' deleted."));
        }
    }
    Ok(())
}
