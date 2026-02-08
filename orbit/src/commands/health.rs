use clap::Subcommand;
use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

#[derive(Subcommand)]
pub enum HealthCmd {
    /// Full health status
    Status,
    /// Liveness probe
    Live,
    /// Readiness probe
    Ready,
    /// Startup probe
    Startup,
}

const HEALTH_COLUMNS: &[Column] = &[
    Column::new("Status", "status"),
    Column::new("Uptime (s)", "uptime_seconds"),
    Column::new("Postgres", "components.postgres"),
    Column::new("Active VMs", "components.pool.active_vms"),
    Column::new("Total Pools", "components.pool.total_pools"),
];

pub async fn run(cmd: HealthCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        HealthCmd::Status => {
            let result = client.get("/health").await?;
            output::render_single(&result, HEALTH_COLUMNS, output_format);
        }
        HealthCmd::Live => {
            let result = client.get("/health/live").await?;
            output::print_success("Liveness: OK");
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
        HealthCmd::Ready => {
            let result = client.get("/health/ready").await?;
            output::print_success("Readiness: OK");
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
        HealthCmd::Startup => {
            let result = client.get("/health/startup").await?;
            output::print_success("Startup: OK");
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
    }
    Ok(())
}

pub async fn run_stats(client: &NovaClient, output_format: &str) -> Result<()> {
    let result = client.get("/stats").await?;
    output::render_single(
        &result,
        &[
            Column::new("Active VMs", "active_vms"),
            Column::new("Total Pools", "total_pools"),
        ],
        output_format,
    );
    Ok(())
}

pub async fn run_invocations(
    limit: Option<u32>,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    let mut path = "/invocations".to_string();
    if let Some(l) = limit {
        path = format!("{path}?limit={l}");
    }
    let result = client.get(&path).await?;
    output::render(
        &result,
        &[
            Column::new("Request ID", "request_id"),
            Column::new("Function", "function_name"),
            Column::new("Status", "status"),
            Column::new("Duration (ms)", "duration_ms"),
            Column::new("Cold Start", "cold_start"),
            Column::new("Timestamp", "timestamp"),
        ],
        output_format,
    );
    Ok(())
}
