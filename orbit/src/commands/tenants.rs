use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum TenantsCmd {
    /// List tenants
    List,
    /// Create a tenant
    Create {
        #[arg(long)]
        name: String,
        #[arg(long)]
        tier: Option<String>,
    },
    /// Update a tenant
    Update {
        id: String,
        #[arg(long)]
        name: Option<String>,
        #[arg(long)]
        status: Option<String>,
        #[arg(long)]
        tier: Option<String>,
    },
    /// Delete a tenant
    Delete { id: String },
    /// Manage namespaces
    Namespaces {
        #[command(subcommand)]
        cmd: NamespacesSubCmd,
    },
    /// Manage quotas
    Quotas {
        #[command(subcommand)]
        cmd: QuotasSubCmd,
    },
    /// Get tenant usage
    Usage { id: String },
}

#[derive(Subcommand)]
pub enum NamespacesSubCmd {
    /// List namespaces
    List { tenant_id: String },
    /// Create a namespace
    Create {
        tenant_id: String,
        #[arg(long)]
        name: String,
    },
    /// Update a namespace
    Update {
        tenant_id: String,
        namespace: String,
        #[arg(long)]
        name: Option<String>,
    },
    /// Delete a namespace
    Delete {
        tenant_id: String,
        namespace: String,
    },
}

#[derive(Subcommand)]
pub enum QuotasSubCmd {
    /// List tenant quotas
    List { tenant_id: String },
    /// Set a quota
    Set {
        tenant_id: String,
        dimension: String,
        #[arg(long)]
        limit: i64,
        #[arg(long)]
        window: Option<String>,
    },
    /// Delete a quota
    Delete {
        tenant_id: String,
        dimension: String,
    },
}

const TENANT_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Status", "status"),
    Column::new("Tier", "tier"),
    Column::new("Created", "created_at"),
];

const NS_COLUMNS: &[Column] = &[
    Column::new("Name", "name"),
    Column::new("Created", "created_at"),
];

const QUOTA_COLUMNS: &[Column] = &[
    Column::new("Dimension", "dimension"),
    Column::new("Limit", "limit"),
    Column::new("Window", "window"),
];

pub async fn run(cmd: TenantsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        TenantsCmd::List => {
            let result = client.get("/tenants").await?;
            output::render(&result, TENANT_COLUMNS, output_format);
        }
        TenantsCmd::Create { name, tier } => {
            let mut body = json!({ "name": name });
            if let Some(t) = tier {
                body["tier"] = json!(t);
            }
            let result = client.post("/tenants", &body).await?;
            output::render_single(&result, TENANT_COLUMNS, output_format);
        }
        TenantsCmd::Update {
            id,
            name,
            status,
            tier,
        } => {
            let mut body = json!({});
            if let Some(n) = name {
                body["name"] = json!(n);
            }
            if let Some(s) = status {
                body["status"] = json!(s);
            }
            if let Some(t) = tier {
                body["tier"] = json!(t);
            }
            let result = client.patch(&format!("/tenants/{id}"), &body).await?;
            output::render_single(&result, TENANT_COLUMNS, output_format);
        }
        TenantsCmd::Delete { id } => {
            client.delete(&format!("/tenants/{id}")).await?;
            output::print_success(&format!("Tenant '{id}' deleted."));
        }
        TenantsCmd::Namespaces { cmd } => match cmd {
            NamespacesSubCmd::List { tenant_id } => {
                let result = client
                    .get(&format!("/tenants/{tenant_id}/namespaces"))
                    .await?;
                output::render(&result, NS_COLUMNS, output_format);
            }
            NamespacesSubCmd::Create { tenant_id, name } => {
                let body = json!({ "name": name });
                let result = client
                    .post(&format!("/tenants/{tenant_id}/namespaces"), &body)
                    .await?;
                output::render_single(&result, NS_COLUMNS, output_format);
            }
            NamespacesSubCmd::Update {
                tenant_id,
                namespace,
                name,
            } => {
                let mut body = json!({});
                if let Some(n) = name {
                    body["name"] = json!(n);
                }
                let result = client
                    .patch(
                        &format!("/tenants/{tenant_id}/namespaces/{namespace}"),
                        &body,
                    )
                    .await?;
                output::render_single(&result, NS_COLUMNS, output_format);
            }
            NamespacesSubCmd::Delete {
                tenant_id,
                namespace,
            } => {
                client
                    .delete(&format!("/tenants/{tenant_id}/namespaces/{namespace}"))
                    .await?;
                output::print_success(&format!("Namespace '{namespace}' deleted."));
            }
        },
        TenantsCmd::Quotas { cmd } => match cmd {
            QuotasSubCmd::List { tenant_id } => {
                let result = client.get(&format!("/tenants/{tenant_id}/quotas")).await?;
                output::render(&result, QUOTA_COLUMNS, output_format);
            }
            QuotasSubCmd::Set {
                tenant_id,
                dimension,
                limit,
                window,
            } => {
                let mut body = json!({ "limit": limit });
                if let Some(w) = window {
                    body["window"] = json!(w);
                }
                let result = client
                    .put(&format!("/tenants/{tenant_id}/quotas/{dimension}"), &body)
                    .await?;
                output::render_single(&result, QUOTA_COLUMNS, output_format);
            }
            QuotasSubCmd::Delete {
                tenant_id,
                dimension,
            } => {
                client
                    .delete(&format!("/tenants/{tenant_id}/quotas/{dimension}"))
                    .await?;
                output::print_success(&format!("Quota '{dimension}' deleted."));
            }
        },
        TenantsCmd::Usage { id } => {
            let result = client.get(&format!("/tenants/{id}/usage")).await?;
            output::render_single(
                &result,
                &[
                    Column::new("Tenant", "tenant_id"),
                    Column::new("Functions", "functions_count"),
                    Column::new("Invocations", "invocations"),
                    Column::new("Async Queue", "async_queue_depth"),
                ],
                output_format,
            );
        }
    }
    Ok(())
}
