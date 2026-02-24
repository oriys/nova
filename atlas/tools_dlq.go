package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListDLQArgs struct{}

type RetryAllDLQArgs struct{}

func RegisterDLQTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_dlq",
		Description: "List all dead-letter queue entries",
	}, c, func(ctx context.Context, args ListDLQArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/async-invocations/dlq")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_retry_all_dlq",
		Description: "Retry all dead-letter queue entries",
	}, c, func(ctx context.Context, args RetryAllDLQArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/async-invocations/dlq/retry-all", map[string]any{})
	})
}
