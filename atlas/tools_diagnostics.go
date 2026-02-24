package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetDiagnosticsArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type AnalyzeDiagnosticsArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type GetRecommendationsArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type GetSLOStatusArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterDiagnosticsTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_diagnostics",
		Description: "Get diagnostics data for a function",
	}, c, func(ctx context.Context, args GetDiagnosticsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/diagnostics", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_analyze_diagnostics",
		Description: "Trigger diagnostic analysis for a function",
	}, c, func(ctx context.Context, args AnalyzeDiagnosticsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/functions/%s/diagnostics/analyze", args.Name), map[string]any{})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_recommendations",
		Description: "Get optimization recommendations for a function",
	}, c, func(ctx context.Context, args GetRecommendationsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/recommendations", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_slo_status",
		Description: "Get current SLO compliance status for a function",
	}, c, func(ctx context.Context, args GetSLOStatusArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/slo/status", args.Name))
	})
}
