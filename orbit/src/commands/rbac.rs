use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum RbacCmd {
    /// Manage roles
    Roles {
        #[command(subcommand)]
        cmd: RolesSubCmd,
    },
    /// Manage permissions
    Permissions {
        #[command(subcommand)]
        cmd: PermsSubCmd,
    },
    /// Manage role assignments
    Assignments {
        #[command(subcommand)]
        cmd: AssignSubCmd,
    },
    /// Show my permissions
    MyPermissions,
}

#[derive(Subcommand)]
pub enum RolesSubCmd {
    /// Create a role
    Create {
        name: String,
        #[arg(long)]
        description: Option<String>,
    },
    /// List roles
    List,
    /// Get a role
    Get { id: String },
    /// Delete a role
    Delete { id: String },
}

#[derive(Subcommand)]
pub enum PermsSubCmd {
    /// Create a permission
    Create {
        name: String,
        #[arg(long)]
        resource: Option<String>,
        #[arg(long)]
        action: Option<String>,
    },
    /// List permissions
    List,
}

#[derive(Subcommand)]
pub enum AssignSubCmd {
    /// Create a role assignment
    Create {
        #[arg(long)]
        role_id: String,
        #[arg(long)]
        subject_type: String,
        #[arg(long)]
        subject_id: String,
    },
    /// List role assignments
    List,
    /// Delete a role assignment
    Delete { id: String },
}

const ROLE_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Description", "description"),
    Column::new("Created", "created_at"),
];

const PERM_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Resource", "resource"),
    Column::new("Action", "action"),
];

const ASSIGN_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Role ID", "role_id"),
    Column::new("Subject Type", "subject_type"),
    Column::new("Subject ID", "subject_id"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: RbacCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        RbacCmd::Roles { cmd } => run_roles(cmd, client, output_format).await,
        RbacCmd::Permissions { cmd } => run_permissions(cmd, client, output_format).await,
        RbacCmd::Assignments { cmd } => run_assignments(cmd, client, output_format).await,
        RbacCmd::MyPermissions => {
            let result = client.get("/rbac/my-permissions").await?;
            output::render(&result, PERM_COLUMNS, output_format);
            Ok(())
        }
    }
}

async fn run_roles(cmd: RolesSubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        RolesSubCmd::Create { name, description } => {
            let mut body = json!({ "name": name });
            if let Some(d) = description {
                body["description"] = json!(d);
            }
            let result = client.post("/rbac/roles", &body).await?;
            output::render_single(&result, ROLE_COLUMNS, output_format);
        }
        RolesSubCmd::List => {
            let result = client.get("/rbac/roles").await?;
            output::render(&result, ROLE_COLUMNS, output_format);
        }
        RolesSubCmd::Get { id } => {
            let result = client.get(&format!("/rbac/roles/{id}")).await?;
            output::render_single(&result, ROLE_COLUMNS, output_format);
        }
        RolesSubCmd::Delete { id } => {
            client.delete(&format!("/rbac/roles/{id}")).await?;
            output::print_success(&format!("Role '{id}' deleted."));
        }
    }
    Ok(())
}

async fn run_permissions(
    cmd: PermsSubCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        PermsSubCmd::Create {
            name,
            resource,
            action,
        } => {
            let mut body = json!({ "name": name });
            if let Some(r) = resource {
                body["resource"] = json!(r);
            }
            if let Some(a) = action {
                body["action"] = json!(a);
            }
            let result = client.post("/rbac/permissions", &body).await?;
            output::render_single(&result, PERM_COLUMNS, output_format);
        }
        PermsSubCmd::List => {
            let result = client.get("/rbac/permissions").await?;
            output::render(&result, PERM_COLUMNS, output_format);
        }
    }
    Ok(())
}

async fn run_assignments(
    cmd: AssignSubCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        AssignSubCmd::Create {
            role_id,
            subject_type,
            subject_id,
        } => {
            let body = json!({
                "role_id": role_id,
                "subject_type": subject_type,
                "subject_id": subject_id,
            });
            let result = client.post("/rbac/assignments", &body).await?;
            output::render_single(&result, ASSIGN_COLUMNS, output_format);
        }
        AssignSubCmd::List => {
            let result = client.get("/rbac/assignments").await?;
            output::render(&result, ASSIGN_COLUMNS, output_format);
        }
        AssignSubCmd::Delete { id } => {
            client.delete(&format!("/rbac/assignments/{id}")).await?;
            output::print_success(&format!("Assignment '{id}' deleted."));
        }
    }
    Ok(())
}
