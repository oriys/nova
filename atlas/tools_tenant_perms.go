package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListMenuPermissionsArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
}
type SetMenuPermissionArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
	MenuKey  string `json:"menu_key" jsonschema:"Menu key"`
	Visible  bool   `json:"visible" jsonschema:"Whether the menu item is visible"`
}
type DeleteMenuPermissionArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
	MenuKey  string `json:"menu_key" jsonschema:"Menu key"`
}
type ListButtonPermissionsArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
}
type SetButtonPermissionArgs struct {
	TenantID      string `json:"tenant_id" jsonschema:"Tenant ID"`
	PermissionKey string `json:"permission_key" jsonschema:"Permission key"`
	Enabled       bool   `json:"enabled" jsonschema:"Whether the button is enabled"`
}
type DeleteButtonPermissionArgs struct {
	TenantID      string `json:"tenant_id" jsonschema:"Tenant ID"`
	PermissionKey string `json:"permission_key" jsonschema:"Permission key"`
}

func RegisterTenantPermTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_menu_permissions",
		Description: "List menu permissions for a tenant",
	}, c, func(ctx context.Context, args ListMenuPermissionsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/tenants/%s/menu-permissions", args.TenantID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_menu_permission",
		Description: "Set a menu permission for a tenant",
	}, c, func(ctx context.Context, args SetMenuPermissionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, fmt.Sprintf("/tenants/%s/menu-permissions/%s", args.TenantID, args.MenuKey), map[string]any{"visible": args.Visible})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_menu_permission",
		Description: "Delete a menu permission for a tenant",
	}, c, func(ctx context.Context, args DeleteMenuPermissionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/tenants/%s/menu-permissions/%s", args.TenantID, args.MenuKey))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_button_permissions",
		Description: "List button permissions for a tenant",
	}, c, func(ctx context.Context, args ListButtonPermissionsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/tenants/%s/button-permissions", args.TenantID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_button_permission",
		Description: "Set a button permission for a tenant",
	}, c, func(ctx context.Context, args SetButtonPermissionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, fmt.Sprintf("/tenants/%s/button-permissions/%s", args.TenantID, args.PermissionKey), map[string]any{"enabled": args.Enabled})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_button_permission",
		Description: "Delete a button permission for a tenant",
	}, c, func(ctx context.Context, args DeleteButtonPermissionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/tenants/%s/button-permissions/%s", args.TenantID, args.PermissionKey))
	})
}
