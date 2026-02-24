package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListBackendsArgs struct{}

func RegisterBackendTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_backends",
		Description: "List available execution backends",
	}, c, func(ctx context.Context, args ListBackendsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/backends")
	})
}
