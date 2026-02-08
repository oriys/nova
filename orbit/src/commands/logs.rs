use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};

const LOG_COLUMNS: &[Column] = &[
    Column::new("Request ID", "request_id"),
    Column::new("Status", "status"),
    Column::new("Duration (ms)", "duration_ms"),
    Column::new("Cold Start", "cold_start"),
    Column::wide("Output", "output"),
    Column::wide("Error", "error"),
    Column::new("Timestamp", "timestamp"),
];

pub async fn run(
    name: &str,
    tail: Option<u32>,
    request_id: Option<String>,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    let mut path = format!("/functions/{name}/logs");
    let mut params = vec![];
    if let Some(t) = tail {
        params.push(format!("tail={t}"));
    }
    if let Some(rid) = request_id {
        params.push(format!("request_id={rid}"));
    }
    if !params.is_empty() {
        path = format!("{}?{}", path, params.join("&"));
    }
    let result = client.get(&path).await?;
    output::render(&result, LOG_COLUMNS, output_format);
    Ok(())
}
