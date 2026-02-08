use crate::client::NovaClient;
use crate::error::Result;
use crate::output;
use clap::Subcommand;

#[derive(Subcommand)]
pub enum ConfigCmd {
    /// Get current configuration
    Get,
    /// Set a configuration value
    Set {
        /// Key to set (server, api_key, tenant, namespace, output)
        key: String,
        /// Value
        value: String,
    },
}

pub async fn run(cmd: ConfigCmd, _client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        ConfigCmd::Get => {
            let config = crate::config::OrbitConfig::load();
            let value = serde_json::to_value(&config)?;
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&value, &[], output_format);
            } else {
                println!(
                    "server:    {}",
                    config.server.as_deref().unwrap_or("(not set)")
                );
                println!(
                    "api_key:   {}",
                    if config.api_key.is_some() {
                        "***"
                    } else {
                        "(not set)"
                    }
                );
                println!(
                    "tenant:    {}",
                    config.tenant.as_deref().unwrap_or("(not set)")
                );
                println!(
                    "namespace: {}",
                    config.namespace.as_deref().unwrap_or("(not set)")
                );
                println!("output:    {}", config.output.as_deref().unwrap_or("table"));
            }
        }
        ConfigCmd::Set { key, value } => {
            let mut config = crate::config::OrbitConfig::load();
            match key.as_str() {
                "server" => config.server = Some(value),
                "api_key" | "api-key" => config.api_key = Some(value),
                "tenant" => config.tenant = Some(value),
                "namespace" => config.namespace = Some(value),
                "output" => config.output = Some(value),
                _ => {
                    return Err(crate::error::OrbitError::Input(format!(
                        "Unknown key '{key}'. Valid keys: server, api_key, tenant, namespace, output"
                    )));
                }
            }
            config.save()?;
            output::print_success(&format!("Set '{key}' in ~/.orbit/config.toml"));
        }
    }
    Ok(())
}
