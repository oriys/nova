use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum AiCmd {
    /// Generate a function from a prompt
    Generate {
        function_name: String,
        prompt: String,
    },
    /// Review a function
    Review { function_name: String },
    /// Rewrite a function with instructions
    Rewrite {
        function_name: String,
        instructions: String,
    },
    /// Generate docs for a function
    GenerateDocs { function_name: String },
    /// Generate docs for a workflow
    GenerateWorkflowDocs { workflow_name: String },
    /// Get AI service status
    Status,
    /// List available AI models
    Models,
}

const MODEL_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Provider", "provider"),
];

pub async fn run(cmd: AiCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        AiCmd::Generate {
            function_name,
            prompt,
        } => {
            let body = json!({ "function_name": function_name, "prompt": prompt });
            let result = client.post("/ai/generate", &body).await?;
            output::render(&result, &[], "json");
        }
        AiCmd::Review { function_name } => {
            let body = json!({ "function_name": function_name });
            let result = client.post("/ai/review", &body).await?;
            output::render(&result, &[], "json");
        }
        AiCmd::Rewrite {
            function_name,
            instructions,
        } => {
            let body = json!({ "function_name": function_name, "instructions": instructions });
            let result = client.post("/ai/rewrite", &body).await?;
            output::render(&result, &[], "json");
        }
        AiCmd::GenerateDocs { function_name } => {
            let body = json!({ "function_name": function_name });
            let result = client.post("/ai/generate-docs", &body).await?;
            output::render(&result, &[], "json");
        }
        AiCmd::GenerateWorkflowDocs { workflow_name } => {
            let body = json!({ "workflow_name": workflow_name });
            let result = client.post("/ai/generate-workflow-docs", &body).await?;
            output::render(&result, &[], "json");
        }
        AiCmd::Status => {
            let result = client.get("/ai/status").await?;
            output::render_single(
                &result,
                &[
                    Column::new("Status", "status"),
                    Column::new("Provider", "provider"),
                    Column::new("Model", "model"),
                ],
                output_format,
            );
        }
        AiCmd::Models => {
            let result = client.get("/ai/models").await?;
            output::render(&result, MODEL_COLUMNS, output_format);
        }
    }
    Ok(())
}
