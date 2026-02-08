use crate::client::NovaClient;
use crate::commands::functions::SnapshotSubCmd;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use indicatif::{ProgressBar, ProgressStyle};
use std::time::Duration;

const SNAPSHOT_COLUMNS: &[Column] = &[
    Column::new("Function", "function_name"),
    Column::new("State", "state_path"),
    Column::new("Memory", "memory_path"),
    Column::wide("Code Drive", "code_drive"),
    Column::new("Created", "created_at"),
];

#[derive(Subcommand)]
pub enum SnapshotsCmd {
    /// List all snapshots
    List,
}

pub async fn run_list(client: &NovaClient, output_format: &str) -> Result<()> {
    let result = client.get("/snapshots").await?;
    output::render(&result, SNAPSHOT_COLUMNS, output_format);
    Ok(())
}

pub async fn run_fn(cmd: SnapshotSubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        SnapshotSubCmd::Create { name } => {
            let spinner = ProgressBar::new_spinner();
            spinner.set_style(
                ProgressStyle::default_spinner()
                    .template("{spinner:.cyan} Creating snapshot for {msg}...")
                    .unwrap(),
            );
            spinner.set_message(name.clone());
            spinner.enable_steady_tick(Duration::from_millis(80));

            let result = client
                .post(
                    &format!("/functions/{name}/snapshot"),
                    &serde_json::json!({}),
                )
                .await?;
            spinner.finish_and_clear();
            output::print_success(&format!("Snapshot created for '{name}'."));
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, SNAPSHOT_COLUMNS, output_format);
            }
        }
        SnapshotSubCmd::Delete { name } => {
            client
                .delete(&format!("/functions/{name}/snapshot"))
                .await?;
            output::print_success(&format!("Snapshot deleted for '{name}'."));
        }
    }
    Ok(())
}
