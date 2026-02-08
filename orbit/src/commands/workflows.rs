use clap::Subcommand;
use serde_json::json;
use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

#[derive(Subcommand)]
pub enum WorkflowsCmd {
    /// Create a workflow
    Create {
        #[arg(long)]
        name: String,
        #[arg(long)]
        description: Option<String>,
        /// Workflow definition (JSON)
        #[arg(long)]
        definition: Option<String>,
        /// Path to definition file
        #[arg(long)]
        definition_file: Option<String>,
    },
    /// List workflows
    List,
    /// Get workflow details
    Get { name: String },
    /// Update a workflow
    Update {
        name: String,
        #[arg(long)]
        description: Option<String>,
        #[arg(long)]
        definition: Option<String>,
        #[arg(long)]
        definition_file: Option<String>,
    },
    /// Delete a workflow
    Delete { name: String },
    /// Manage versions
    Versions {
        #[command(subcommand)]
        cmd: WfVersionsCmd,
    },
    /// Run a workflow
    Run {
        name: String,
        /// Input JSON
        #[arg(long)]
        input: Option<String>,
    },
    /// Manage workflow runs
    Runs {
        #[command(subcommand)]
        cmd: WfRunsCmd,
    },
}

#[derive(Subcommand)]
pub enum WfVersionsCmd {
    /// Publish a new version
    Publish {
        name: String,
        #[arg(long)]
        definition: Option<String>,
        #[arg(long)]
        definition_file: Option<String>,
    },
    /// List versions
    List { name: String },
    /// Get specific version
    Get { name: String, version: u32 },
}

#[derive(Subcommand)]
pub enum WfRunsCmd {
    /// List workflow runs
    List { name: String },
    /// Get run details
    Get { name: String, id: String },
    /// Cancel a run
    Cancel { name: String, id: String },
}

const WF_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Status", "status"),
    Column::new("Version", "current_version"),
    Column::wide("Description", "description"),
    Column::new("Created", "created_at"),
];

const RUN_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Status", "status"),
    Column::new("Trigger", "trigger_type"),
    Column::wide("Error", "error_message"),
    Column::new("Started", "started_at"),
    Column::wide("Finished", "finished_at"),
];

const WF_VERSION_COLUMNS: &[Column] = &[
    Column::new("Version", "version"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: WorkflowsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        WorkflowsCmd::Create {
            name,
            description,
            definition,
            definition_file,
        } => {
            let mut body = json!({ "name": name });
            if let Some(d) = description {
                body["description"] = json!(d);
            }
            if let Some(def) = definition {
                let parsed: serde_json::Value = serde_json::from_str(&def)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON: {e}")))?;
                body["definition"] = parsed;
            } else if let Some(path) = definition_file {
                let content = std::fs::read_to_string(&path)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Cannot read file: {e}")))?;
                let parsed: serde_json::Value = serde_json::from_str(&content)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON in file: {e}")))?;
                body["definition"] = parsed;
            }
            let result = client.post("/workflows", &body).await?;
            output::render_single(&result, WF_COLUMNS, output_format);
        }
        WorkflowsCmd::List => {
            let result = client.get("/workflows").await?;
            output::render(&result, WF_COLUMNS, output_format);
        }
        WorkflowsCmd::Get { name } => {
            let result = client.get(&format!("/workflows/{name}")).await?;
            output::render_single(&result, WF_COLUMNS, output_format);
        }
        WorkflowsCmd::Update {
            name,
            description,
            definition,
            definition_file,
        } => {
            let mut body = json!({});
            if let Some(d) = description {
                body["description"] = json!(d);
            }
            if let Some(def) = definition {
                let parsed: serde_json::Value = serde_json::from_str(&def)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON: {e}")))?;
                body["definition"] = parsed;
            } else if let Some(path) = definition_file {
                let content = std::fs::read_to_string(&path)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Cannot read file: {e}")))?;
                let parsed: serde_json::Value = serde_json::from_str(&content)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON in file: {e}")))?;
                body["definition"] = parsed;
            }
            let result = client.put(&format!("/workflows/{name}"), &body).await?;
            output::render_single(&result, WF_COLUMNS, output_format);
        }
        WorkflowsCmd::Delete { name } => {
            client.delete(&format!("/workflows/{name}")).await?;
            output::print_success(&format!("Workflow '{name}' deleted."));
        }
        WorkflowsCmd::Versions { cmd } => match cmd {
            WfVersionsCmd::Publish {
                name,
                definition,
                definition_file,
            } => {
                let mut body = json!({});
                if let Some(def) = definition {
                    let parsed: serde_json::Value = serde_json::from_str(&def)
                        .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON: {e}")))?;
                    body["definition"] = parsed;
                } else if let Some(path) = definition_file {
                    let content = std::fs::read_to_string(&path)
                        .map_err(|e| crate::error::OrbitError::Input(format!("Cannot read: {e}")))?;
                    let parsed: serde_json::Value = serde_json::from_str(&content)
                        .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON: {e}")))?;
                    body["definition"] = parsed;
                }
                let result = client
                    .post(&format!("/workflows/{name}/versions"), &body)
                    .await?;
                output::render_single(&result, WF_VERSION_COLUMNS, output_format);
            }
            WfVersionsCmd::List { name } => {
                let result = client.get(&format!("/workflows/{name}/versions")).await?;
                output::render(&result, WF_VERSION_COLUMNS, output_format);
            }
            WfVersionsCmd::Get { name, version } => {
                let result = client
                    .get(&format!("/workflows/{name}/versions/{version}"))
                    .await?;
                output::render_single(&result, WF_VERSION_COLUMNS, output_format);
            }
        },
        WorkflowsCmd::Run { name, input } => {
            let mut body = json!({});
            if let Some(inp) = input {
                let parsed: serde_json::Value = serde_json::from_str(&inp)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON: {e}")))?;
                body["input"] = parsed;
            }
            let result = client
                .post(&format!("/workflows/{name}/run"), &body)
                .await?;
            output::render_single(&result, RUN_COLUMNS, output_format);
        }
        WorkflowsCmd::Runs { cmd } => match cmd {
            WfRunsCmd::List { name } => {
                let result = client.get(&format!("/workflows/{name}/runs")).await?;
                output::render(&result, RUN_COLUMNS, output_format);
            }
            WfRunsCmd::Get { name, id } => {
                let result = client
                    .get(&format!("/workflows/{name}/runs/{id}"))
                    .await?;
                output::render_single(&result, RUN_COLUMNS, output_format);
            }
            WfRunsCmd::Cancel { name, id } => {
                let result = client
                    .post(
                        &format!("/workflows/{name}/runs/{id}/cancel"),
                        &json!({}),
                    )
                    .await?;
                output::print_success(&format!("Run '{id}' cancelled."));
                if output_format == "json" || output_format == "yaml" {
                    output::render_single(&result, RUN_COLUMNS, output_format);
                }
            }
        },
    }
    Ok(())
}
