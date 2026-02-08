package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetConfigArgs struct{}
type SetConfigArgs struct {
	Config json.RawMessage `json:"config" jsonschema:"Configuration object (JSON)"`
}

func RegisterConfigTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_get_config", Description: "Get global Nova configuration"}, c,
		func(ctx context.Context, args GetConfigArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/config")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_set_config", Description: "Update global Nova configuration"}, c,
		func(ctx context.Context, args SetConfigArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Put(ctx, "/config", args.Config)
		})
}
