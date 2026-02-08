mod client;
mod commands;
mod config;
mod error;
mod output;

use clap::{Parser, Subcommand};
use commands::{
    apikeys::ApiKeysCmd,
    async_invocations::GlobalAsyncCmd,
    config_cmd::ConfigCmd,
    events::{DeliveriesCmd, SubscriptionsCmd, TopicsCmd},
    functions::FunctionsCmd,
    gateway::GatewayCmd,
    health::HealthCmd,
    layers::LayersCmd,
    metrics::MetricsCmd,
    runtimes::RuntimesCmd,
    secrets::SecretsCmd,
    tenants::TenantsCmd,
    workflows::WorkflowsCmd,
};

#[derive(Parser)]
#[command(name = "orbit", version, about = "CLI for the Nova serverless platform")]
struct Cli {
    /// Nova server URL
    #[arg(long, env = "NOVA_URL", global = true)]
    server: Option<String>,

    /// API key for authentication
    #[arg(long, env = "NOVA_API_KEY", global = true)]
    api_key: Option<String>,

    /// Tenant ID
    #[arg(long, env = "NOVA_TENANT", global = true)]
    tenant: Option<String>,

    /// Namespace
    #[arg(long, env = "NOVA_NAMESPACE", global = true)]
    namespace: Option<String>,

    /// Output format: table, wide, json, yaml
    #[arg(short, long, env = "NOVA_OUTPUT", global = true)]
    output: Option<String>,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Manage functions
    #[command(alias = "fn")]
    Functions {
        #[command(subcommand)]
        cmd: FunctionsCmd,
    },
    /// List all snapshots
    Snapshots,
    /// Manage runtimes
    #[command(alias = "rt")]
    Runtimes {
        #[command(subcommand)]
        cmd: RuntimesCmd,
    },
    /// Manage tenants
    Tenants {
        #[command(subcommand)]
        cmd: TenantsCmd,
    },
    /// Manage event topics
    Topics {
        #[command(subcommand)]
        cmd: TopicsCmd,
    },
    /// Manage subscriptions
    Subscriptions {
        #[command(subcommand)]
        cmd: SubscriptionsCmd,
    },
    /// Manage deliveries
    Deliveries {
        #[command(subcommand)]
        cmd: DeliveriesCmd,
    },
    /// Manage workflows
    #[command(alias = "wf")]
    Workflows {
        #[command(subcommand)]
        cmd: WorkflowsCmd,
    },
    /// Manage API gateway
    #[command(alias = "gw")]
    Gateway {
        #[command(subcommand)]
        cmd: GatewayCmd,
    },
    /// Manage shared layers
    Layers {
        #[command(subcommand)]
        cmd: LayersCmd,
    },
    /// Manage API keys
    Apikeys {
        #[command(subcommand)]
        cmd: ApiKeysCmd,
    },
    /// Manage secrets
    Secrets {
        #[command(subcommand)]
        cmd: SecretsCmd,
    },
    /// Manage local configuration
    Config {
        #[command(subcommand)]
        cmd: ConfigCmd,
    },
    /// Health checks
    Health {
        #[command(subcommand)]
        cmd: HealthCmd,
    },
    /// Pool statistics
    Stats,
    /// Global metrics
    Metrics {
        #[command(subcommand)]
        cmd: MetricsCmd,
    },
    /// List recent invocations
    Invocations {
        #[arg(long)]
        limit: Option<u32>,
    },
    /// Manage async invocations (global)
    AsyncInvocations {
        #[command(subcommand)]
        cmd: GlobalAsyncCmd,
    },
    /// Show version
    Version,
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();
    let cfg = config::OrbitConfig::load();

    let server = cli
        .server
        .or(cfg.server)
        .unwrap_or_else(|| "http://localhost:9000".into());
    let api_key = cli.api_key.or(cfg.api_key);
    let tenant = cli.tenant.or(cfg.tenant);
    let namespace = cli.namespace.or(cfg.namespace);
    let output_format = cli
        .output
        .or(cfg.output)
        .unwrap_or_else(|| "table".into());

    let nova = client::NovaClient::new(server, api_key, tenant, namespace);

    let result = match cli.command {
        Commands::Functions { cmd } => commands::functions::run(cmd, &nova, &output_format).await,
        Commands::Snapshots => commands::snapshots::run_list(&nova, &output_format).await,
        Commands::Runtimes { cmd } => commands::runtimes::run(cmd, &nova, &output_format).await,
        Commands::Tenants { cmd } => commands::tenants::run(cmd, &nova, &output_format).await,
        Commands::Topics { cmd } => commands::events::run_topics(cmd, &nova, &output_format).await,
        Commands::Subscriptions { cmd } => {
            commands::events::run_subscriptions(cmd, &nova, &output_format).await
        }
        Commands::Deliveries { cmd } => {
            commands::events::run_deliveries(cmd, &nova, &output_format).await
        }
        Commands::Workflows { cmd } => {
            commands::workflows::run(cmd, &nova, &output_format).await
        }
        Commands::Gateway { cmd } => commands::gateway::run(cmd, &nova, &output_format).await,
        Commands::Layers { cmd } => commands::layers::run(cmd, &nova, &output_format).await,
        Commands::Apikeys { cmd } => commands::apikeys::run(cmd, &nova, &output_format).await,
        Commands::Secrets { cmd } => commands::secrets::run(cmd, &nova, &output_format).await,
        Commands::Config { cmd } => commands::config_cmd::run(cmd, &nova, &output_format).await,
        Commands::Health { cmd } => commands::health::run(cmd, &nova, &output_format).await,
        Commands::Stats => commands::health::run_stats(&nova, &output_format).await,
        Commands::Metrics { cmd } => {
            commands::metrics::run_global(cmd, &nova, &output_format).await
        }
        Commands::Invocations { limit } => {
            commands::health::run_invocations(limit, &nova, &output_format).await
        }
        Commands::AsyncInvocations { cmd } => {
            commands::async_invocations::run_global(cmd, &nova, &output_format).await
        }
        Commands::Version => {
            println!("orbit {}", env!("CARGO_PKG_VERSION"));
            Ok(())
        }
    };

    if let Err(e) = result {
        output::print_error(&e.to_string());
        std::process::exit(1);
    }
}
