use clap::Subcommand;
use serde_json::json;
use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

#[derive(Subcommand)]
pub enum ApiKeysCmd {
    /// Create an API key
    Create {
        #[arg(long)]
        name: String,
        #[arg(long)]
        scopes: Vec<String>,
    },
    /// List API keys
    List,
    /// Delete an API key
    Delete { id: String },
    /// Update an API key
    Update {
        id: String,
        #[arg(long)]
        name: Option<String>,
        #[arg(long)]
        scopes: Vec<String>,
    },
}

const APIKEY_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Key", "key"),
    Column::new("Scopes", "scopes"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: ApiKeysCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        ApiKeysCmd::Create { name, scopes } => {
            let mut body = json!({ "name": name });
            if !scopes.is_empty() {
                body["scopes"] = json!(scopes);
            }
            let result = client.post("/api-keys", &body).await?;
            output::render_single(&result, APIKEY_COLUMNS, output_format);
        }
        ApiKeysCmd::List => {
            let result = client.get("/api-keys").await?;
            output::render(&result, APIKEY_COLUMNS, output_format);
        }
        ApiKeysCmd::Delete { id } => {
            client.delete(&format!("/api-keys/{id}")).await?;
            output::print_success(&format!("API key '{id}' revoked."));
        }
        ApiKeysCmd::Update { id, name, scopes } => {
            let mut body = json!({});
            if let Some(n) = name {
                body["name"] = json!(n);
            }
            if !scopes.is_empty() {
                body["scopes"] = json!(scopes);
            }
            let result = client.patch(&format!("/api-keys/{id}"), &body).await?;
            output::render_single(&result, APIKEY_COLUMNS, output_format);
        }
    }
    Ok(())
}
