package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateRouteArgs struct {
	Domain       string   `json:"domain" jsonschema:"Domain (e.g. api.example.com)"`
	Path         string   `json:"path" jsonschema:"Path pattern (e.g. /v1/users)"`
	FunctionName string   `json:"function_name" jsonschema:"Function to route to"`
	Methods      []string `json:"methods,omitempty" jsonschema:"HTTP methods (empty = all)"`
	AuthStrategy string   `json:"auth_strategy,omitempty" jsonschema:"Auth strategy (none inherit apikey jwt)"`
}
type ListRoutesArgs struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset int `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}
type GetRouteArgs struct {
	ID string `json:"id" jsonschema:"Route ID"`
}
type UpdateRouteArgs struct {
	ID           string `json:"id" jsonschema:"Route ID"`
	Domain       string `json:"domain,omitempty" jsonschema:"Domain"`
	Path         string `json:"path,omitempty" jsonschema:"Path pattern"`
	FunctionName string `json:"function_name,omitempty" jsonschema:"Function name"`
	Enabled      *bool  `json:"enabled,omitempty" jsonschema:"Enable or disable"`
}
type DeleteRouteArgs struct {
	ID string `json:"id" jsonschema:"Route ID"`
}

func RegisterGatewayTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_create_gateway_route", Description: "Create an API gateway route"}, c,
		func(ctx context.Context, args CreateRouteArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, "/gateway/routes", args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_gateway_routes", Description: "List all API gateway routes"}, c,
		func(ctx context.Context, args ListRoutesArgs, c *NovaClient) (json.RawMessage, error) {
			q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
			return c.Get(ctx, "/gateway/routes"+q)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_gateway_route", Description: "Get gateway route details"}, c,
		func(ctx context.Context, args GetRouteArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/gateway/routes/%s", args.ID))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_update_gateway_route", Description: "Update an API gateway route"}, c,
		func(ctx context.Context, args UpdateRouteArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Patch(ctx, fmt.Sprintf("/gateway/routes/%s", args.ID), args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_delete_gateway_route", Description: "Delete an API gateway route"}, c,
		func(ctx context.Context, args DeleteRouteArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Delete(ctx, fmt.Sprintf("/gateway/routes/%s", args.ID))
		})
}
