use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum TriggersCmd {
    /// Create a trigger
    Create {
        /// Trigger name
        name: String,
        /// Function to trigger
        #[arg(long)]
        function: String,
        /// Trigger type (e.g. http, cron, event)
        #[arg(long, name = "type")]
        trigger_type: String,
        /// Configuration as JSON string
        #[arg(long)]
        config: Option<String>,
    },
    /// List triggers
    List {
        /// Filter by function name
        #[arg(long)]
        function: Option<String>,
    },
    /// Get trigger details
    Get { id: String },
    /// Update a trigger
    Update {
        id: String,
        /// Enable or disable the trigger
        #[arg(long)]
        enabled: Option<bool>,
    },
    /// Delete a trigger
    Delete { id: String },
}

const TRIGGER_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Function", "function"),
    Column::new("Type", "type"),
    Column::new("Enabled", "enabled"),
];

pub async fn run(cmd: TriggersCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        TriggersCmd::Create {
            name,
            function,
            trigger_type,
            config,
        } => {
            let mut body = json!({
                "name": name,
                "function": function,
                "type": trigger_type,
            });
            if let Some(c) = config {
                let parsed: serde_json::Value = serde_json::from_str(&c).map_err(|e| {
                    crate::error::OrbitError::Input(format!("Invalid JSON config: {e}"))
                })?;
                body["config"] = parsed;
            }
            let result = client.post("/triggers", &body).await?;
            output::render_single(&result, TRIGGER_COLUMNS, output_format);
        }
        TriggersCmd::List { function } => {
            let mut path = "/triggers".to_string();
            if let Some(f) = function {
                path = format!("{path}?function={f}");
            }
            let result = client.get(&path).await?;
            output::render(&result, TRIGGER_COLUMNS, output_format);
        }
        TriggersCmd::Get { id } => {
            let result = client.get(&format!("/triggers/{id}")).await?;
            output::render_single(&result, TRIGGER_COLUMNS, output_format);
        }
        TriggersCmd::Update { id, enabled } => {
            let mut body = json!({});
            if let Some(v) = enabled {
                body["enabled"] = json!(v);
            }
            let result = client
                .put(&format!("/triggers/{id}"), &body)
                .await?;
            output::render_single(&result, TRIGGER_COLUMNS, output_format);
        }
        TriggersCmd::Delete { id } => {
            client.delete(&format!("/triggers/{id}")).await?;
            output::print_success(&format!("Trigger '{id}' deleted."));
        }
    }
    Ok(())
}
