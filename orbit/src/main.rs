mod client;
mod commands;
mod config;
mod error;
mod output;

use clap::{Parser, Subcommand};
use commands::{
    ai::AiCmd,
    apikeys::ApiKeysCmd,
    async_invocations::GlobalAsyncCmd,
    cluster::ClusterCmd,
    config_cmd::ConfigCmd,
    cost::CostCmd,
    diagnostics::DiagnosticsCmd,
    dlq::DlqCmd,
    docs::DocsCmd,
    events::{DeliveriesCmd, SubscriptionsCmd, TopicsCmd},
    functions::FunctionsCmd,
    gateway::GatewayCmd,
    health::HealthCmd,
    layers::LayersCmd,
    metrics::MetricsCmd,
    notifications::NotificationsCmd,
    rate_limit::RateLimitCmd,
    rbac::RbacCmd,
    runtimes::RuntimesCmd,
    secrets::SecretsCmd,
    slo::SloCmd,
    state::StateCmd,
    tenant_perms::{ButtonPermsCmd, MenuPermsCmd},
    tenants::TenantsCmd,
    triggers::TriggersCmd,
    volumes::{MountsCmd, VolumesCmd},
    workflows::WorkflowsCmd,
};

#[derive(Parser)]
#[command(
    name = "orbit",
    version,
    about = "CLI for the Nova serverless platform"
)]
struct Cli {
    /// Zenith gateway URL (or Nova-compatible API endpoint)
    #[arg(long, env = "ZENITH_URL", global = true)]
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
    /// Cost intelligence
    Cost {
        #[command(subcommand)]
        cmd: CostCmd,
    },
    /// SLO policy management
    Slo {
        #[command(subcommand)]
        cmd: SloCmd,
    },
    /// Volume management
    Volumes {
        #[command(subcommand)]
        cmd: VolumesCmd,
    },
    /// Function volume mounts
    Mounts {
        #[command(subcommand)]
        cmd: MountsCmd,
    },
    /// Trigger management
    Triggers {
        #[command(subcommand)]
        cmd: TriggersCmd,
    },
    /// Function diagnostics
    Diagnostics {
        #[command(subcommand)]
        cmd: DiagnosticsCmd,
    },
    /// Function state management
    State {
        #[command(subcommand)]
        cmd: StateCmd,
    },
    /// Dead letter queue
    Dlq {
        #[command(subcommand)]
        cmd: DlqCmd,
    },
    /// List backends
    Backends,
    /// Manage tenant menu permissions
    MenuPerms {
        #[command(subcommand)]
        cmd: MenuPermsCmd,
    },
    /// Manage tenant button permissions
    ButtonPerms {
        #[command(subcommand)]
        cmd: ButtonPermsCmd,
    },
    /// Manage cluster nodes
    Cluster {
        #[command(subcommand)]
        cmd: ClusterCmd,
    },
    /// Manage RBAC roles, permissions, and assignments
    Rbac {
        #[command(subcommand)]
        cmd: RbacCmd,
    },
    /// Manage notifications
    Notifications {
        #[command(subcommand)]
        cmd: NotificationsCmd,
    },
    /// AI operations
    Ai {
        #[command(subcommand)]
        cmd: AiCmd,
    },
    /// Manage documentation
    Docs {
        #[command(subcommand)]
        cmd: DocsCmd,
    },
    /// Manage rate limit template
    RateLimit {
        #[command(subcommand)]
        cmd: RateLimitCmd,
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
    let output_format = cli.output.or(cfg.output).unwrap_or_else(|| "table".into());

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
        Commands::Workflows { cmd } => commands::workflows::run(cmd, &nova, &output_format).await,
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
        Commands::Cost { cmd } => commands::cost::run(cmd, &nova, &output_format).await,
        Commands::Slo { cmd } => commands::slo::run(cmd, &nova, &output_format).await,
        Commands::Volumes { cmd } => commands::volumes::run(cmd, &nova, &output_format).await,
        Commands::Mounts { cmd } => commands::volumes::run_mounts(cmd, &nova, &output_format).await,
        Commands::Triggers { cmd } => commands::triggers::run(cmd, &nova, &output_format).await,
        Commands::Diagnostics { cmd } => {
            commands::diagnostics::run(cmd, &nova, &output_format).await
        }
        Commands::State { cmd } => commands::state::run(cmd, &nova, &output_format).await,
        Commands::Dlq { cmd } => commands::dlq::run(cmd, &nova, &output_format).await,
        Commands::Backends => commands::backends::run(&nova, &output_format).await,
        Commands::MenuPerms { cmd } => {
            commands::tenant_perms::run_menu(cmd, &nova, &output_format).await
        }
        Commands::ButtonPerms { cmd } => {
            commands::tenant_perms::run_button(cmd, &nova, &output_format).await
        }
        Commands::Cluster { cmd } => commands::cluster::run(cmd, &nova, &output_format).await,
        Commands::Rbac { cmd } => commands::rbac::run(cmd, &nova, &output_format).await,
        Commands::Notifications { cmd } => {
            commands::notifications::run(cmd, &nova, &output_format).await
        }
        Commands::Ai { cmd } => commands::ai::run(cmd, &nova, &output_format).await,
        Commands::Docs { cmd } => commands::docs::run(cmd, &nova, &output_format).await,
        Commands::RateLimit { cmd } => {
            commands::rate_limit::run(cmd, &nova, &output_format).await
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
