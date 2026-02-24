package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CostSummaryArgs struct {
	Window int `json:"window,omitempty" jsonschema:"Time window in seconds (default 86400)"`
}

type FunctionCostArgs struct {
	Name   string `json:"name" jsonschema:"Function name"`
	Window int    `json:"window,omitempty" jsonschema:"Time window in seconds"`
}

func RegisterCostTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_cost_summary",
		Description: "Get cost summary across all functions",
	}, c, func(ctx context.Context, args CostSummaryArgs, c *NovaClient) (json.RawMessage, error) {
		window := args.Window
		if window == 0 {
			window = 86400
		}
		return c.Get(ctx, fmt.Sprintf("/cost/summary?window=%d", window))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_function_cost",
		Description: "Get cost breakdown for a specific function",
	}, c, func(ctx context.Context, args FunctionCostArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"window": intStr(args.Window)})
		return c.Get(ctx, fmt.Sprintf("/functions/%s/cost%s", args.Name, q))
	})
}
