package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListTenantsArgs struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset int `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}
type CreateTenantArgs struct {
	Name string `json:"name" jsonschema:"Tenant name"`
	Tier string `json:"tier,omitempty" jsonschema:"Tier (free pro enterprise)"`
}
type UpdateTenantArgs struct {
	ID     string `json:"id" jsonschema:"Tenant ID"`
	Name   string `json:"name,omitempty" jsonschema:"New name"`
	Status string `json:"status,omitempty" jsonschema:"Status (active suspended archived)"`
	Tier   string `json:"tier,omitempty" jsonschema:"Tier"`
}
type DeleteTenantArgs struct {
	ID string `json:"id" jsonschema:"Tenant ID"`
}
type ListNamespacesArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset   int    `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}
type CreateNamespaceArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
	Name     string `json:"name" jsonschema:"Namespace name"`
}
type UpdateNamespaceArgs struct {
	TenantID  string `json:"tenant_id" jsonschema:"Tenant ID"`
	Namespace string `json:"namespace" jsonschema:"Namespace name"`
	Name      string `json:"name,omitempty" jsonschema:"New name"`
}
type DeleteNamespaceArgs struct {
	TenantID  string `json:"tenant_id" jsonschema:"Tenant ID"`
	Namespace string `json:"namespace" jsonschema:"Namespace name"`
}
type ListQuotasArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset   int    `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}
type SetQuotaArgs struct {
	TenantID  string `json:"tenant_id" jsonschema:"Tenant ID"`
	Dimension string `json:"dimension" jsonschema:"Quota dimension (invocations functions_count etc)"`
	Limit     int    `json:"limit" jsonschema:"Quota limit value"`
	Window    string `json:"window,omitempty" jsonschema:"Window (per_second per_hour absolute)"`
}
type DeleteQuotaArgs struct {
	TenantID  string `json:"tenant_id" jsonschema:"Tenant ID"`
	Dimension string `json:"dimension" jsonschema:"Quota dimension"`
}
type GetUsageArgs struct {
	TenantID string `json:"tenant_id" jsonschema:"Tenant ID"`
}

func RegisterTenantTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_tenants",
		Description: "List all tenants",
	}, c, func(ctx context.Context, args ListTenantsArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
		return c.Get(ctx, "/tenants"+q)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_tenant",
		Description: "Create a new tenant",
	}, c, func(ctx context.Context, args CreateTenantArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/tenants", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_update_tenant",
		Description: "Update a tenant",
	}, c, func(ctx context.Context, args UpdateTenantArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Patch(ctx, fmt.Sprintf("/tenants/%s", args.ID), args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_tenant",
		Description: "Delete a tenant",
	}, c, func(ctx context.Context, args DeleteTenantArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/tenants/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_namespaces",
		Description: "List namespaces in a tenant",
	}, c, func(ctx context.Context, args ListNamespacesArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
		return c.Get(ctx, fmt.Sprintf("/tenants/%s/namespaces%s", args.TenantID, q))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_namespace",
		Description: "Create a namespace in a tenant",
	}, c, func(ctx context.Context, args CreateNamespaceArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/tenants/%s/namespaces", args.TenantID), map[string]string{"name": args.Name})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_update_namespace",
		Description: "Update a namespace",
	}, c, func(ctx context.Context, args UpdateNamespaceArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Patch(ctx, fmt.Sprintf("/tenants/%s/namespaces/%s", args.TenantID, args.Namespace), args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_namespace",
		Description: "Delete a namespace",
	}, c, func(ctx context.Context, args DeleteNamespaceArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/tenants/%s/namespaces/%s", args.TenantID, args.Namespace))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_quotas",
		Description: "List quotas for a tenant",
	}, c, func(ctx context.Context, args ListQuotasArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
		return c.Get(ctx, fmt.Sprintf("/tenants/%s/quotas%s", args.TenantID, q))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_quota",
		Description: "Set a quota for a tenant",
	}, c, func(ctx context.Context, args SetQuotaArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{"limit": args.Limit}
		if args.Window != "" {
			body["window"] = args.Window
		}
		return c.Put(ctx, fmt.Sprintf("/tenants/%s/quotas/%s", args.TenantID, args.Dimension), body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_quota",
		Description: "Delete a quota for a tenant",
	}, c, func(ctx context.Context, args DeleteQuotaArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/tenants/%s/quotas/%s", args.TenantID, args.Dimension))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_tenant_usage",
		Description: "Get resource usage for a tenant",
	}, c, func(ctx context.Context, args GetUsageArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/tenants/%s/usage", args.TenantID))
	})
}
