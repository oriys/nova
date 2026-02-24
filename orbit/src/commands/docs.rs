use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum DocsCmd {
    /// Manage function documentation
    FunctionDocs {
        #[command(subcommand)]
        cmd: FnDocsSubCmd,
    },
    /// Manage workflow documentation
    WorkflowDocs {
        #[command(subcommand)]
        cmd: WfDocsSubCmd,
    },
    /// Manage documentation shares
    Shares {
        #[command(subcommand)]
        cmd: SharesSubCmd,
    },
}

#[derive(Subcommand)]
pub enum FnDocsSubCmd {
    /// Get function documentation
    Get { name: String },
    /// Save function documentation
    Save {
        name: String,
        #[arg(long)]
        content: String,
    },
    /// Delete function documentation
    Delete { name: String },
}

#[derive(Subcommand)]
pub enum WfDocsSubCmd {
    /// Get workflow documentation
    Get { name: String },
    /// Save workflow documentation
    Save {
        name: String,
        #[arg(long)]
        content: String,
    },
    /// Delete workflow documentation
    Delete { name: String },
}

#[derive(Subcommand)]
pub enum SharesSubCmd {
    /// Create a documentation share
    Create {
        #[arg(long)]
        title: String,
        /// Comma-separated list of function names
        #[arg(long)]
        functions: String,
    },
    /// List documentation shares
    List,
    /// Delete a documentation share
    Delete { id: String },
}

const DOC_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Content", "content"),
    Column::new("Updated", "updated_at"),
];

const SHARE_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Title", "title"),
    Column::new("Functions", "functions"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: DocsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        DocsCmd::FunctionDocs { cmd } => run_fn_docs(cmd, client, output_format).await,
        DocsCmd::WorkflowDocs { cmd } => run_wf_docs(cmd, client, output_format).await,
        DocsCmd::Shares { cmd } => run_shares(cmd, client, output_format).await,
    }
}

async fn run_fn_docs(
    cmd: FnDocsSubCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        FnDocsSubCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/docs")).await?;
            output::render_single(&result, DOC_COLUMNS, output_format);
        }
        FnDocsSubCmd::Save { name, content } => {
            let body = json!({ "content": content });
            let result = client
                .put(&format!("/functions/{name}/docs"), &body)
                .await?;
            output::render_single(&result, DOC_COLUMNS, output_format);
        }
        FnDocsSubCmd::Delete { name } => {
            client.delete(&format!("/functions/{name}/docs")).await?;
            output::print_success(&format!("Documentation for function '{name}' deleted."));
        }
    }
    Ok(())
}

async fn run_wf_docs(
    cmd: WfDocsSubCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        WfDocsSubCmd::Get { name } => {
            let result = client.get(&format!("/workflows/{name}/docs")).await?;
            output::render_single(&result, DOC_COLUMNS, output_format);
        }
        WfDocsSubCmd::Save { name, content } => {
            let body = json!({ "content": content });
            let result = client
                .put(&format!("/workflows/{name}/docs"), &body)
                .await?;
            output::render_single(&result, DOC_COLUMNS, output_format);
        }
        WfDocsSubCmd::Delete { name } => {
            client.delete(&format!("/workflows/{name}/docs")).await?;
            output::print_success(&format!("Documentation for workflow '{name}' deleted."));
        }
    }
    Ok(())
}

async fn run_shares(
    cmd: SharesSubCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        SharesSubCmd::Create { title, functions } => {
            let fn_list: Vec<&str> = functions.split(',').map(|s| s.trim()).collect();
            let body = json!({ "title": title, "functions": fn_list });
            let result = client.post("/api-docs/shares", &body).await?;
            output::render_single(&result, SHARE_COLUMNS, output_format);
        }
        SharesSubCmd::List => {
            let result = client.get("/api-docs/shares").await?;
            output::render(&result, SHARE_COLUMNS, output_format);
        }
        SharesSubCmd::Delete { id } => {
            client.delete(&format!("/api-docs/shares/{id}")).await?;
            output::print_success(&format!("Share '{id}' deleted."));
        }
    }
    Ok(())
}
