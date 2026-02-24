use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum RateLimitCmd {
    /// Get rate limit template
    Get,
    /// Set rate limit template
    Set {
        #[arg(long)]
        requests_per_second: Option<i64>,
        #[arg(long)]
        burst_size: Option<i64>,
    },
}

const RATE_LIMIT_COLUMNS: &[Column] = &[
    Column::new("Requests/s", "requests_per_second"),
    Column::new("Burst Size", "burst_size"),
];

pub async fn run(cmd: RateLimitCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        RateLimitCmd::Get => {
            let result = client.get("/gateway/rate-limit-template").await?;
            output::render_single(&result, RATE_LIMIT_COLUMNS, output_format);
        }
        RateLimitCmd::Set {
            requests_per_second,
            burst_size,
        } => {
            let mut body = json!({});
            if let Some(r) = requests_per_second {
                body["requests_per_second"] = json!(r);
            }
            if let Some(b) = burst_size {
                body["burst_size"] = json!(b);
            }
            let result = client
                .put("/gateway/rate-limit-template", &body)
                .await?;
            output::render_single(&result, RATE_LIMIT_COLUMNS, output_format);
        }
    }
    Ok(())
}
