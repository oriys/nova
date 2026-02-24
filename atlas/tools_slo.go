package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetSLOPolicyArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type SetSLOPolicyArgs struct {
	Name              string  `json:"name" jsonschema:"Function name"`
	TargetP99Ms       int     `json:"target_p99_ms" jsonschema:"Target p99 latency in milliseconds"`
	TargetSuccessRate float64 `json:"target_success_rate" jsonschema:"Target success rate (0-1)"`
	EvaluationWindowS int     `json:"evaluation_window_s" jsonschema:"Evaluation window in seconds"`
}

type DeleteSLOPolicyArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterSLOTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_slo_policy",
		Description: "Get the SLO policy for a function",
	}, c, func(ctx context.Context, args GetSLOPolicyArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/slo", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_slo_policy",
		Description: "Set the SLO policy for a function",
	}, c, func(ctx context.Context, args SetSLOPolicyArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{}
		if args.TargetP99Ms > 0 {
			body["target_p99_ms"] = args.TargetP99Ms
		}
		if args.TargetSuccessRate > 0 {
			body["target_success_rate"] = args.TargetSuccessRate
		}
		if args.EvaluationWindowS > 0 {
			body["evaluation_window_s"] = args.EvaluationWindowS
		}
		return c.Put(ctx, fmt.Sprintf("/functions/%s/slo", args.Name), body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_slo_policy",
		Description: "Delete the SLO policy for a function",
	}, c, func(ctx context.Context, args DeleteSLOPolicyArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s/slo", args.Name))
	})
}
