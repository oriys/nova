use crate::client::NovaClient;
use crate::error::Result;
use crate::output::{self, Column};
use clap::Subcommand;
use serde_json::json;

#[derive(Subcommand)]
pub enum MenuPermsCmd {
    /// List menu permissions
    List { tenant_id: String },
    /// Set a menu permission
    Set {
        tenant_id: String,
        menu_key: String,
        #[arg(long)]
        visible: bool,
    },
    /// Delete a menu permission
    Delete {
        tenant_id: String,
        menu_key: String,
    },
}

#[derive(Subcommand)]
pub enum ButtonPermsCmd {
    /// List button permissions
    List { tenant_id: String },
    /// Set a button permission
    Set {
        tenant_id: String,
        permission_key: String,
        #[arg(long)]
        enabled: bool,
    },
    /// Delete a button permission
    Delete {
        tenant_id: String,
        permission_key: String,
    },
}

const MENU_PERM_COLUMNS: &[Column] = &[
    Column::new("Menu Key", "menu_key"),
    Column::new("Visible", "visible"),
];

const BUTTON_PERM_COLUMNS: &[Column] = &[
    Column::new("Permission Key", "permission_key"),
    Column::new("Enabled", "enabled"),
];

pub async fn run_menu(cmd: MenuPermsCmd, client: &NovaClient, output_format: &str) -> Result<()> {
    match cmd {
        MenuPermsCmd::List { tenant_id } => {
            let result = client
                .get(&format!("/tenants/{tenant_id}/menu-permissions"))
                .await?;
            output::render(&result, MENU_PERM_COLUMNS, output_format);
        }
        MenuPermsCmd::Set {
            tenant_id,
            menu_key,
            visible,
        } => {
            let body = json!({ "visible": visible });
            let result = client
                .put(
                    &format!("/tenants/{tenant_id}/menu-permissions/{menu_key}"),
                    &body,
                )
                .await?;
            output::render_single(&result, MENU_PERM_COLUMNS, output_format);
        }
        MenuPermsCmd::Delete {
            tenant_id,
            menu_key,
        } => {
            client
                .delete(&format!(
                    "/tenants/{tenant_id}/menu-permissions/{menu_key}"
                ))
                .await?;
            output::print_success(&format!("Menu permission '{menu_key}' deleted."));
        }
    }
    Ok(())
}

pub async fn run_button(
    cmd: ButtonPermsCmd,
    client: &NovaClient,
    output_format: &str,
) -> Result<()> {
    match cmd {
        ButtonPermsCmd::List { tenant_id } => {
            let result = client
                .get(&format!("/tenants/{tenant_id}/button-permissions"))
                .await?;
            output::render(&result, BUTTON_PERM_COLUMNS, output_format);
        }
        ButtonPermsCmd::Set {
            tenant_id,
            permission_key,
            enabled,
        } => {
            let body = json!({ "enabled": enabled });
            let result = client
                .put(
                    &format!("/tenants/{tenant_id}/button-permissions/{permission_key}"),
                    &body,
                )
                .await?;
            output::render_single(&result, BUTTON_PERM_COLUMNS, output_format);
        }
        ButtonPermsCmd::Delete {
            tenant_id,
            permission_key,
        } => {
            client
                .delete(&format!(
                    "/tenants/{tenant_id}/button-permissions/{permission_key}"
                ))
                .await?;
            output::print_success(&format!(
                "Button permission '{permission_key}' deleted."
            ));
        }
    }
    Ok(())
}
