use serde_json::json;
use crate::client::NovaClient;
use crate::commands::functions::CapacitySubCmd;
use crate::error::Result;
use crate::output::{self, Column};

const CAPACITY_COLUMNS: &[Column] = &[
    Column::new("Enabled", "enabled"),
    Column::new("Max Inflight", "max_inflight"),
    Column::new("Max Queue", "max_queue_depth"),
    Column::new("Max Wait (ms)", "max_queue_wait_ms"),
    Column::new("Shed Code", "shed_status_code"),
    Column::wide("Retry After", "retry_after_s"),
];

pub async fn run(cmd: CapacitySubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        CapacitySubCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/capacity")).await?;
            output::render_single(&result, CAPACITY_COLUMNS, output_format);
        }
        CapacitySubCmd::Set {
            name,
            max_inflight,
            max_queue_depth,
            max_queue_wait_ms,
            shed_status_code,
        } => {
            let mut body = json!({ "enabled": true });
            if let Some(v) = max_inflight {
                body["max_inflight"] = json!(v);
            }
            if let Some(v) = max_queue_depth {
                body["max_queue_depth"] = json!(v);
            }
            if let Some(v) = max_queue_wait_ms {
                body["max_queue_wait_ms"] = json!(v);
            }
            if let Some(v) = shed_status_code {
                body["shed_status_code"] = json!(v);
            }
            let result = client.put(&format!("/functions/{name}/capacity"), &body).await?;
            output::render_single(&result, CAPACITY_COLUMNS, output_format);
        }
        CapacitySubCmd::Delete { name } => {
            client.delete(&format!("/functions/{name}/capacity")).await?;
            output::print_success(&format!("Capacity policy deleted for '{name}'."));
        }
    }
    Ok(())
}
