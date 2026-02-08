use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum TopicsCmd {
    /// Create a topic
    Create {
        #[arg(long)]
        name: String,
        #[arg(long)]
        description: Option<String>,
        #[arg(long)]
        retention_hours: Option<i64>,
    },
    /// List topics
    List,
    /// Get topic details
    Get { name: String },
    /// Delete a topic
    Delete { name: String },
    /// Publish an event
    Publish {
        name: String,
        /// JSON payload
        #[arg(long)]
        payload: String,
        /// Ordering key
        #[arg(long)]
        ordering_key: Option<String>,
    },
    /// List messages in a topic
    Messages { name: String },
    /// Manage subscriptions
    Subscriptions {
        #[command(subcommand)]
        cmd: TopicSubsCmd,
    },
    /// Manage outbox
    Outbox {
        #[command(subcommand)]
        cmd: OutboxSubCmd,
    },
}

#[derive(Subcommand)]
pub enum TopicSubsCmd {
    /// Create a subscription
    Create {
        topic: String,
        #[arg(long)]
        name: String,
        #[arg(long)]
        function: String,
        #[arg(long)]
        max_attempts: Option<i64>,
        #[arg(long)]
        max_inflight: Option<i64>,
    },
    /// List subscriptions for a topic
    List { topic: String },
}

#[derive(Subcommand)]
pub enum SubscriptionsCmd {
    /// Get subscription details
    Get { id: String },
    /// Update subscription
    Update {
        id: String,
        #[arg(long)]
        enabled: Option<bool>,
        #[arg(long)]
        max_attempts: Option<i64>,
        #[arg(long)]
        max_inflight: Option<i64>,
    },
    /// Delete subscription
    Delete { id: String },
    /// List deliveries for subscription
    Deliveries { id: String },
    /// Replay events
    Replay {
        id: String,
        #[arg(long)]
        from_sequence: Option<i64>,
        #[arg(long)]
        from_time: Option<String>,
    },
    /// Seek to position
    Seek {
        id: String,
        #[arg(long)]
        to_sequence: Option<i64>,
        #[arg(long)]
        to_time: Option<String>,
    },
}

#[derive(Subcommand)]
pub enum DeliveriesCmd {
    /// Get delivery details
    Get { id: String },
    /// Retry a failed delivery
    Retry { id: String },
}

#[derive(Subcommand)]
pub enum OutboxSubCmd {
    /// Create an outbox entry
    Create {
        topic: String,
        #[arg(long)]
        payload: String,
        #[arg(long)]
        ordering_key: Option<String>,
    },
    /// List outbox entries
    List {
        topic: String,
        #[arg(long)]
        status: Option<String>,
    },
    /// Retry a failed outbox entry
    Retry { id: String },
}

const TOPIC_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Description", "description"),
    Column::new("Retention (h)", "retention_hours"),
    Column::new("Created", "created_at"),
];

const SUB_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Name", "name"),
    Column::new("Topic", "topic_name"),
    Column::new("Function", "function_name"),
    Column::new("Enabled", "enabled"),
    Column::wide("Max Attempts", "max_attempts"),
    Column::wide("Max Inflight", "max_inflight"),
];

const DELIVERY_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Message", "message_id"),
    Column::new("Status", "status"),
    Column::new("Attempt", "attempt"),
    Column::wide("Error", "error"),
    Column::new("Delivered", "delivered_at"),
];

const MSG_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Sequence", "sequence"),
    Column::new("Key", "ordering_key"),
    Column::wide("Payload", "payload"),
    Column::new("Published", "published_at"),
];

const OUTBOX_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Topic", "topic_name"),
    Column::new("Status", "status"),
    Column::wide("Key", "ordering_key"),
    Column::new("Created", "created_at"),
];

pub async fn run_topics(cmd: TopicsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        TopicsCmd::Create {
            name,
            description,
            retention_hours,
        } => {
            let mut body = json!({ "name": name });
            if let Some(d) = description {
                body["description"] = json!(d);
            }
            if let Some(r) = retention_hours {
                body["retention_hours"] = json!(r);
            }
            let result = client.post("/topics", &body).await?;
            output::render_single(&result, TOPIC_COLUMNS, output_format);
        }
        TopicsCmd::List => {
            let result = client.get("/topics").await?;
            output::render(&result, TOPIC_COLUMNS, output_format);
        }
        TopicsCmd::Get { name } => {
            let result = client.get(&format!("/topics/{name}")).await?;
            output::render_single(&result, TOPIC_COLUMNS, output_format);
        }
        TopicsCmd::Delete { name } => {
            client.delete(&format!("/topics/{name}")).await?;
            output::print_success(&format!("Topic '{name}' deleted."));
        }
        TopicsCmd::Publish {
            name,
            payload,
            ordering_key,
        } => {
            let parsed: serde_json::Value = serde_json::from_str(&payload)
                .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON: {e}")))?;
            let mut body = json!({ "payload": parsed });
            if let Some(k) = ordering_key {
                body["ordering_key"] = json!(k);
            }
            let result = client
                .post(&format!("/topics/{name}/publish"), &body)
                .await?;
            output::render_single(&result, MSG_COLUMNS, output_format);
        }
        TopicsCmd::Messages { name } => {
            let result = client.get(&format!("/topics/{name}/messages")).await?;
            output::render(&result, MSG_COLUMNS, output_format);
        }
        TopicsCmd::Subscriptions { cmd } => match cmd {
            TopicSubsCmd::Create {
                topic,
                name,
                function,
                max_attempts,
                max_inflight,
            } => {
                let mut body = json!({
                    "name": name,
                    "function_name": function,
                });
                if let Some(m) = max_attempts {
                    body["max_attempts"] = json!(m);
                }
                if let Some(m) = max_inflight {
                    body["max_inflight"] = json!(m);
                }
                let result = client
                    .post(&format!("/topics/{topic}/subscriptions"), &body)
                    .await?;
                output::render_single(&result, SUB_COLUMNS, output_format);
            }
            TopicSubsCmd::List { topic } => {
                let result = client
                    .get(&format!("/topics/{topic}/subscriptions"))
                    .await?;
                output::render(&result, SUB_COLUMNS, output_format);
            }
        },
        TopicsCmd::Outbox { cmd } => match cmd {
            OutboxSubCmd::Create {
                topic,
                payload,
                ordering_key,
            } => {
                let parsed: serde_json::Value = serde_json::from_str(&payload)
                    .map_err(|e| crate::error::OrbitError::Input(format!("Invalid JSON: {e}")))?;
                let mut body = json!({ "payload": parsed });
                if let Some(k) = ordering_key {
                    body["ordering_key"] = json!(k);
                }
                let result = client
                    .post(&format!("/topics/{topic}/outbox"), &body)
                    .await?;
                output::render_single(&result, OUTBOX_COLUMNS, output_format);
            }
            OutboxSubCmd::List { topic, status } => {
                let mut path = format!("/topics/{topic}/outbox");
                if let Some(s) = status {
                    path = format!("{path}?status={s}");
                }
                let result = client.get(&path).await?;
                output::render(&result, OUTBOX_COLUMNS, output_format);
            }
            OutboxSubCmd::Retry { id } => {
                let result = client
                    .post(&format!("/outbox/{id}/retry"), &json!({}))
                    .await?;
                output::render_single(&result, OUTBOX_COLUMNS, output_format);
            }
        },
    }
    Ok(())
}

pub async fn run_subscriptions(
    cmd: SubscriptionsCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        SubscriptionsCmd::Get { id } => {
            let result = client.get(&format!("/subscriptions/{id}")).await?;
            output::render_single(&result, SUB_COLUMNS, output_format);
        }
        SubscriptionsCmd::Update {
            id,
            enabled,
            max_attempts,
            max_inflight,
        } => {
            let mut body = json!({});
            if let Some(e) = enabled {
                body["enabled"] = json!(e);
            }
            if let Some(m) = max_attempts {
                body["max_attempts"] = json!(m);
            }
            if let Some(m) = max_inflight {
                body["max_inflight"] = json!(m);
            }
            let result = client.patch(&format!("/subscriptions/{id}"), &body).await?;
            output::render_single(&result, SUB_COLUMNS, output_format);
        }
        SubscriptionsCmd::Delete { id } => {
            client.delete(&format!("/subscriptions/{id}")).await?;
            output::print_success(&format!("Subscription '{id}' deleted."));
        }
        SubscriptionsCmd::Deliveries { id } => {
            let result = client
                .get(&format!("/subscriptions/{id}/deliveries"))
                .await?;
            output::render(&result, DELIVERY_COLUMNS, output_format);
        }
        SubscriptionsCmd::Replay {
            id,
            from_sequence,
            from_time,
        } => {
            let mut body = json!({});
            if let Some(s) = from_sequence {
                body["from_sequence"] = json!(s);
            }
            if let Some(t) = from_time {
                body["from_time"] = json!(t);
            }
            let result = client
                .post(&format!("/subscriptions/{id}/replay"), &body)
                .await?;
            output::print_success("Replay initiated.");
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
        SubscriptionsCmd::Seek {
            id,
            to_sequence,
            to_time,
        } => {
            let mut body = json!({});
            if let Some(s) = to_sequence {
                body["to_sequence"] = json!(s);
            }
            if let Some(t) = to_time {
                body["to_time"] = json!(t);
            }
            let result = client
                .post(&format!("/subscriptions/{id}/seek"), &body)
                .await?;
            output::print_success("Seek completed.");
            if output_format == "json" || output_format == "yaml" {
                output::render_single(&result, &[], output_format);
            }
        }
    }
    Ok(())
}

pub async fn run_deliveries(
    cmd: DeliveriesCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        DeliveriesCmd::Get { id } => {
            let result = client.get(&format!("/deliveries/{id}")).await?;
            output::render_single(&result, DELIVERY_COLUMNS, output_format);
        }
        DeliveriesCmd::Retry { id } => {
            let result = client
                .post(&format!("/deliveries/{id}/retry"), &json!({}))
                .await?;
            output::render_single(&result, DELIVERY_COLUMNS, output_format);
        }
    }
    Ok(())
}
