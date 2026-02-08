use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum SecretsCmd {
    /// Create a secret
    Create {
        #[arg(long)]
        name: String,
        #[arg(long)]
        value: String,
    },
    /// List secrets
    List,
    /// Delete a secret
    Delete { name: String },
}

const SECRET_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: SecretsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        SecretsCmd::Create { name, value } => {
            let body = json!({ "name": name, "value": value });
            let result = client.post("/secrets", &body).await?;
            output::render_single(&result, SECRET_COLUMNS, output_format);
        }
        SecretsCmd::List => {
            let result = client.get("/secrets").await?;
            output::render(&result, SECRET_COLUMNS, output_format);
        }
        SecretsCmd::Delete { name } => {
            client.delete(&format!("/secrets/{name}")).await?;
            output::print_success(&format!("Secret '{name}' deleted."));
        }
    }
    Ok(())
}
