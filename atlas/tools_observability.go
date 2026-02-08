package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetGlobalMetricsArgs struct{}
type GetTimeseriesArgs struct {
	Range string `json:"range,omitempty" jsonschema:"Time range (e.g. 1h 5m 1d)"`
}
type GetGlobalHeatmapArgs struct {
	Weeks int `json:"weeks,omitempty" jsonschema:"Number of weeks (default 52)"`
}
type ListInvocationsArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"Max results (default 100 max 500)"`
}

func RegisterObservabilityTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_get_metrics", Description: "Get global metrics in JSON format"}, c,
		func(ctx context.Context, args GetGlobalMetricsArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/metrics")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_timeseries", Description: "Get time-series metrics with bucketed data"}, c,
		func(ctx context.Context, args GetTimeseriesArgs, c *NovaClient) (json.RawMessage, error) {
			q := queryString(map[string]string{"range": args.Range})
			return c.Get(ctx, "/metrics/timeseries"+q)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_heatmap", Description: "Get daily invocation heatmap for all functions"}, c,
		func(ctx context.Context, args GetGlobalHeatmapArgs, c *NovaClient) (json.RawMessage, error) {
			q := queryString(map[string]string{"weeks": intStr(args.Weeks)})
			return c.Get(ctx, "/metrics/heatmap"+q)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_invocations", Description: "List recent invocations across all functions"}, c,
		func(ctx context.Context, args ListInvocationsArgs, c *NovaClient) (json.RawMessage, error) {
			q := queryString(map[string]string{"limit": intStr(args.Limit)})
			return c.Get(ctx, "/invocations"+q)
		})
}
