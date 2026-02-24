use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum NotificationsCmd {
    /// List notifications
    List {
        #[arg(long)]
        status: Option<String>,
    },
    /// Get unread notification count
    UnreadCount,
    /// Mark a notification as read
    Read { id: String },
    /// Mark all notifications as read
    ReadAll,
}

const NOTIFICATION_COLUMNS: &[Column] = &[
    Column::new("ID", "id"),
    Column::new("Title", "title"),
    Column::new("Status", "status"),
    Column::new("Created At", "created_at"),
];

pub async fn run(
    cmd: NotificationsCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        NotificationsCmd::List { status } => {
            let path = match status {
                Some(s) => format!("/notifications?status={s}"),
                None => "/notifications".to_string(),
            };
            let result = client.get(&path).await?;
            output::render(&result, NOTIFICATION_COLUMNS, output_format);
        }
        NotificationsCmd::UnreadCount => {
            let result = client.get("/notifications/unread-count").await?;
            output::render_single(
                &result,
                &[Column::new("Unread Count", "unread_count")],
                output_format,
            );
        }
        NotificationsCmd::Read { id } => {
            let result = client
                .post(&format!("/notifications/{id}/read"), &json!({}))
                .await?;
            output::render_single(&result, NOTIFICATION_COLUMNS, output_format);
        }
        NotificationsCmd::ReadAll => {
            client.post("/notifications/read-all", &json!({})).await?;
            output::print_success("All notifications marked as read.");
        }
    }
    Ok(())
}
