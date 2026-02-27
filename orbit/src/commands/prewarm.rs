use crate::client::NovaClient;
use crate::error::Result;
use crate::output;
use indicatif::{ProgressBar, ProgressStyle};
use std::time::Duration;

pub async fn run(name: &str, client: &NovaClient) -> Result<()> {
    let spinner = ProgressBar::new_spinner();
    spinner.set_style(
        ProgressStyle::default_spinner()
            .template("{spinner:.cyan} Pre-warming {msg}...")
            .unwrap(),
    );
    spinner.set_message(name.to_string());
    spinner.enable_steady_tick(Duration::from_millis(80));

    client
        .post(
            &format!("/functions/{name}/prewarm"),
            &serde_json::json!({}),
        )
        .await?;
    spinner.finish_and_clear();
    output::print_success(&format!("Function '{name}' pre-warmed."));
    Ok(())
}
