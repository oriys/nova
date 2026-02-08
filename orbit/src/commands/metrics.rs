use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;

#[derive(Subcommand)]
pub enum MetricsCmd {
    /// Get global metrics (JSON)
    Json,
    /// Get Prometheus metrics
    Prometheus,
    /// Get time-series metrics
    Timeseries {
        /// Time range (e.g. 1h, 5m, 1d)
        #[arg(long, default_value = "1h")]
        range: String,
    },
    /// Get invocation heatmap
    Heatmap {
        /// Number of weeks
        #[arg(long, default_value = "52")]
        weeks: u32,
    },
}

const TIMESERIES_COLUMNS: &[Column] = &[
    Column::new("Timestamp", "timestamp"),
    Column::new("Invocations", "invocations"),
    Column::new("Errors", "errors"),
    Column::new("Avg Duration", "avg_duration_ms"),
    Column::wide("P50", "p50_ms"),
    Column::wide("P99", "p99_ms"),
];

const HEATMAP_COLUMNS: &[Column] = &[Column::new("Date", "date"), Column::new("Count", "count")];

pub async fn run_global(cmd: MetricsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        MetricsCmd::Json => {
            let result = client.get("/metrics").await?;
            println!("{}", serde_json::to_string_pretty(&result)?);
        }
        MetricsCmd::Prometheus => {
            let result = client.get("/metrics/prometheus").await?;
            if let Some(s) = result.as_str() {
                println!("{s}");
            } else {
                println!("{}", serde_json::to_string_pretty(&result)?);
            }
        }
        MetricsCmd::Timeseries { range } => {
            let result = client
                .get(&format!("/metrics/timeseries?range={range}"))
                .await?;
            output::render(&result, TIMESERIES_COLUMNS, output_format);
        }
        MetricsCmd::Heatmap { weeks } => {
            let result = client
                .get(&format!("/metrics/heatmap?weeks={weeks}"))
                .await?;
            output::render(&result, HEATMAP_COLUMNS, output_format);
        }
    }
    Ok(())
}

pub async fn run_fn_metrics(
    name: &str,
    range: Option<String>,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    let mut path = format!("/functions/{name}/metrics");
    if let Some(r) = range {
        path = format!("{path}?range={r}");
    }
    let result = client.get(&path).await?;
    output::render_single(
        &result,
        &[
            Column::new("Function", "function_name"),
            Column::new("Invocations", "invocations"),
            Column::new("Errors", "errors"),
            Column::new("Avg Duration", "avg_duration_ms"),
            Column::new("Pool Size", "pool.size"),
        ],
        output_format,
    );
    Ok(())
}

pub async fn run_fn_heatmap(
    name: &str,
    weeks: u32,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    let result = client
        .get(&format!("/functions/{name}/heatmap?weeks={weeks}"))
        .await?;
    output::render(&result, HEATMAP_COLUMNS, output_format);
    Ok(())
}
