use serde_json::json;
use crate::client::NovaClient;
use crate::commands::functions::ScalingSubCmd;
use crate::error::Result;
use crate::output::{self, Column};

const SCALING_COLUMNS: &[Column] = &[
    Column::new("Enabled", "enabled"),
    Column::new("Min Replicas", "min_replicas"),
    Column::new("Max Replicas", "max_replicas"),
    Column::new("Target Util", "target_utilization"),
    Column::new("Cooldown Up (s)", "cooldown_scale_up_s"),
    Column::new("Cooldown Down (s)", "cooldown_scale_down_s"),
];

pub async fn run(cmd: ScalingSubCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        ScalingSubCmd::Get { name } => {
            let result = client.get(&format!("/functions/{name}/scaling")).await?;
            output::render_single(&result, SCALING_COLUMNS, output_format);
        }
        ScalingSubCmd::Set {
            name,
            min_replicas,
            max_replicas,
            target_utilization,
            cooldown_up,
            cooldown_down,
        } => {
            let mut body = json!({ "enabled": true });
            if let Some(v) = min_replicas {
                body["min_replicas"] = json!(v);
            }
            if let Some(v) = max_replicas {
                body["max_replicas"] = json!(v);
            }
            if let Some(v) = target_utilization {
                body["target_utilization"] = json!(v);
            }
            if let Some(v) = cooldown_up {
                body["cooldown_scale_up_s"] = json!(v);
            }
            if let Some(v) = cooldown_down {
                body["cooldown_scale_down_s"] = json!(v);
            }
            let result = client.put(&format!("/functions/{name}/scaling"), &body).await?;
            output::render_single(&result, SCALING_COLUMNS, output_format);
        }
        ScalingSubCmd::Delete { name } => {
            client.delete(&format!("/functions/{name}/scaling")).await?;
            output::print_success(&format!("Scaling policy deleted for '{name}'."));
        }
    }
    Ok(())
}
