package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetRateLimitTemplateArgs struct{}
type SetRateLimitTemplateArgs struct {
	RequestsPerSecond int `json:"requests_per_second" jsonschema:"Requests per second"`
	BurstSize         int `json:"burst_size" jsonschema:"Burst size"`
}

func RegisterRateLimitTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_rate_limit_template",
		Description: "Get the rate limit template",
	}, c, func(ctx context.Context, args GetRateLimitTemplateArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/gateway/rate-limit-template")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_rate_limit_template",
		Description: "Set the rate limit template",
	}, c, func(ctx context.Context, args SetRateLimitTemplateArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, "/gateway/rate-limit-template", args)
	})
}
