package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetFnMetricsArgs struct {
	Name  string `json:"name" jsonschema:"Function name"`
	Range string `json:"range,omitempty" jsonschema:"Time range (e.g. 1h 5m 1d)"`
}

type GetFnHeatmapArgs struct {
	Name  string `json:"name" jsonschema:"Function name"`
	Weeks int    `json:"weeks,omitempty" jsonschema:"Number of weeks (default 52)"`
}

func RegisterMetricTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_metrics",
		Description: "Get metrics for a specific function including invocation counts and latency",
	}, c, func(ctx context.Context, args GetFnMetricsArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"range": args.Range})
		return c.Get(ctx, fmt.Sprintf("/functions/%s/metrics%s", args.Name, q))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_heatmap",
		Description: "Get daily invocation heatmap for a function",
	}, c, func(ctx context.Context, args GetFnHeatmapArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"weeks": intStr(args.Weeks)})
		return c.Get(ctx, fmt.Sprintf("/functions/%s/heatmap%s", args.Name, q))
	})
}
