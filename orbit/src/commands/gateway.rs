use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum GatewayCmd {
    /// Manage routes
    Routes {
        #[command(subcommand)]
        cmd: RoutesCmd,
    },
}

#[derive(Subcommand)]
pub enum RoutesCmd {
    /// Create a route
    Create {
        #[arg(long)]
        domain: String,
        #[arg(long)]
        path: String,
        #[arg(long)]
        function: String,
        #[arg(long)]
        methods: Vec<String>,
        #[arg(long)]
        auth: Option<String>,
    },
    /// List routes
    List,
    /// Get route details
    Get { id: String },
    /// Update a route
    Update {
        id: String,
        #[arg(long)]
        domain: Option<String>,
        #[arg(long)]
        path: Option<String>,
        #[arg(long)]
        function: Option<String>,
        #[arg(long)]
        enabled: Option<bool>,
    },
    /// Delete a route
    Delete { id: String },
}

const ROUTE_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Domain", "domain"),
    Column::new("Path", "path"),
    Column::new("Methods", "methods"),
    Column::new("Function", "function_name"),
    Column::new("Auth", "auth_strategy"),
    Column::wide("Enabled", "enabled"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: GatewayCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        GatewayCmd::Routes { cmd } => run_routes(cmd, client, output_format).await,
    }
}

async fn run_routes(cmd: RoutesCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        RoutesCmd::Create {
            domain,
            path,
            function,
            methods,
            auth,
        } => {
            let mut body = json!({
                "domain": domain,
                "path": path,
                "function_name": function,
            });
            if !methods.is_empty() {
                body["methods"] = json!(methods);
            }
            if let Some(a) = auth {
                body["auth_strategy"] = json!(a);
            }
            let result = client.post("/gateway/routes", &body).await?;
            output::render_single(&result, ROUTE_COLUMNS, output_format);
        }
        RoutesCmd::List => {
            let result = client.get("/gateway/routes").await?;
            output::render(&result, ROUTE_COLUMNS, output_format);
        }
        RoutesCmd::Get { id } => {
            let result = client.get(&format!("/gateway/routes/{id}")).await?;
            output::render_single(&result, ROUTE_COLUMNS, output_format);
        }
        RoutesCmd::Update {
            id,
            domain,
            path,
            function,
            enabled,
        } => {
            let mut body = json!({});
            if let Some(d) = domain {
                body["domain"] = json!(d);
            }
            if let Some(p) = path {
                body["path"] = json!(p);
            }
            if let Some(f) = function {
                body["function_name"] = json!(f);
            }
            if let Some(e) = enabled {
                body["enabled"] = json!(e);
            }
            let result = client
                .patch(&format!("/gateway/routes/{id}"), &body)
                .await?;
            output::render_single(&result, ROUTE_COLUMNS, output_format);
        }
        RoutesCmd::Delete { id } => {
            client.delete(&format!("/gateway/routes/{id}")).await?;
            output::print_success(&format!("Route '{id}' deleted."));
        }
    }
    Ok(())
}
