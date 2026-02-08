use serde_json::json;
use crate::client::NovaClient;
use crate::commands::functions::SchedulesSubCmd;
use crate::error::Result;
use crate::output::{self, Column};

const SCHEDULE_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Cron", "cron_expression"),
    Column::new("Enabled", "enabled"),
    Column::wide("Input", "input"),
    Column::new("Created", "created_at"),
];

pub async fn run(cmd: SchedulesSubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        SchedulesSubCmd::Create { name, cron, input } => {
            let mut body = json!({ "cron_expression": cron });
            if let Some(inp) = input {
                let parsed: serde_json::Value = serde_json::from_str(&inp)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON input: {e}")))?;
                body["input"] = parsed;
            }
            let result = client.post(&format!("/functions/{name}/schedules"), &body).await?;
            output::render_single(&result, SCHEDULE_COLUMNS, output_format);
        }
        SchedulesSubCmd::List { name } => {
            let result = client.get(&format!("/functions/{name}/schedules")).await?;
            output::render(&result, SCHEDULE_COLUMNS, output_format);
        }
        SchedulesSubCmd::Delete { name, id } => {
            client.delete(&format!("/functions/{name}/schedules/{id}")).await?;
            output::print_success(&format!("Schedule '{id}' deleted."));
        }
        SchedulesSubCmd::Update { name, id, enabled } => {
            let mut body = json!({});
            if let Some(e) = enabled {
                body["enabled"] = json!(e);
            }
            let result = client
                .patch(&format!("/functions/{name}/schedules/{id}"), &body)
                .await?;
            output::render_single(&result, SCHEDULE_COLUMNS, output_format);
        }
    }
    Ok(())
}
