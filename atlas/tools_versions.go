package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListVersionsArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type GetVersionArgs struct {
	Name    string `json:"name" jsonschema:"Function name"`
	Version int    `json:"version" jsonschema:"Version number"`
}

func RegisterVersionTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_function_versions",
		Description: "List all versions of a function",
	}, c, func(ctx context.Context, args ListVersionsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/versions", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_version",
		Description: "Get a specific version of a function",
	}, c, func(ctx context.Context, args GetVersionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/versions/%d", args.Name, args.Version))
	})
}
