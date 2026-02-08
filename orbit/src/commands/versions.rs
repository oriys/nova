use crate::client::NovaClient;
use crate::commands::functions::VersionsSubCmd;
use crate::error::Result;
use crate::output::{self, Column};

const VERSION_COLUMNS: &[Column] = &[
    Column::new("Version", "version"),
    Column::new("Code Hash", "code_hash"),
    Column::new("Handler", "handler"),
    Column::new("Memory", "memory_mb"),
    Column::new("Timeout", "timeout_s"),
    Column::new("Mode", "mode"),
    Column::wide("Description", "description"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: VersionsSubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        VersionsSubCmd::List { name } => {
            let result = client.get(&format!("/functions/{name}/versions")).await?;
            output::render(&result, VERSION_COLUMNS, output_format);
        }
        VersionsSubCmd::Get { name, version } => {
            let result = client
                .get(&format!("/functions/{name}/versions/{version}"))
                .await?;
            output::render_single(&result, VERSION_COLUMNS, output_format);
        }
    }
    Ok(())
}
