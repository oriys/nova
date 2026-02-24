package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateRoleArgs struct {
	Name        string `json:"name" jsonschema:"Role name"`
	Description string `json:"description,omitempty" jsonschema:"Role description"`
}
type ListRolesArgs struct{}
type GetRoleArgs struct {
	ID string `json:"id" jsonschema:"Role ID"`
}
type DeleteRoleArgs struct {
	ID string `json:"id" jsonschema:"Role ID"`
}
type CreatePermissionArgs struct {
	Name     string `json:"name" jsonschema:"Permission name"`
	Resource string `json:"resource" jsonschema:"Resource"`
	Action   string `json:"action" jsonschema:"Action"`
}
type ListPermissionsArgs struct{}
type AssignRolePermissionArgs struct {
	RoleID       string `json:"role_id" jsonschema:"Role ID"`
	PermissionID string `json:"permission_id" jsonschema:"Permission ID"`
}
type RevokeRolePermissionArgs struct {
	RoleID       string `json:"role_id" jsonschema:"Role ID"`
	PermissionID string `json:"permission_id" jsonschema:"Permission ID"`
}
type CreateRoleAssignmentArgs struct {
	RoleID      string `json:"role_id" jsonschema:"Role ID"`
	SubjectType string `json:"subject_type" jsonschema:"Subject type (user group)"`
	SubjectID   string `json:"subject_id" jsonschema:"Subject ID"`
}
type ListRoleAssignmentsArgs struct{}
type DeleteRoleAssignmentArgs struct {
	ID string `json:"id" jsonschema:"Assignment ID"`
}
type GetMyPermissionsArgs struct{}

func RegisterRBACTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_role",
		Description: "Create a new RBAC role",
	}, c, func(ctx context.Context, args CreateRoleArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/rbac/roles", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_roles",
		Description: "List all RBAC roles",
	}, c, func(ctx context.Context, args ListRolesArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/rbac/roles")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_role",
		Description: "Get details of an RBAC role",
	}, c, func(ctx context.Context, args GetRoleArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/rbac/roles/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_role",
		Description: "Delete an RBAC role",
	}, c, func(ctx context.Context, args DeleteRoleArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/rbac/roles/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_permission",
		Description: "Create a new RBAC permission",
	}, c, func(ctx context.Context, args CreatePermissionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/rbac/permissions", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_permissions",
		Description: "List all RBAC permissions",
	}, c, func(ctx context.Context, args ListPermissionsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/rbac/permissions")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_assign_role_permission",
		Description: "Assign a permission to an RBAC role",
	}, c, func(ctx context.Context, args AssignRolePermissionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/rbac/roles/%s/permissions", args.RoleID), map[string]string{"permission_id": args.PermissionID})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_revoke_role_permission",
		Description: "Revoke a permission from an RBAC role",
	}, c, func(ctx context.Context, args RevokeRolePermissionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/rbac/roles/%s/permissions/%s", args.RoleID, args.PermissionID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_role_assignment",
		Description: "Create an RBAC role assignment",
	}, c, func(ctx context.Context, args CreateRoleAssignmentArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/rbac/assignments", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_role_assignments",
		Description: "List all RBAC role assignments",
	}, c, func(ctx context.Context, args ListRoleAssignmentsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/rbac/assignments")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_role_assignment",
		Description: "Delete an RBAC role assignment",
	}, c, func(ctx context.Context, args DeleteRoleAssignmentArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/rbac/assignments/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_my_permissions",
		Description: "Get permissions for the current user",
	}, c, func(ctx context.Context, args GetMyPermissionsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/rbac/my-permissions")
	})
}
