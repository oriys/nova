package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetScalingArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type SetScalingArgs struct {
	Name              string  `json:"name" jsonschema:"Function name"`
	MinReplicas       int     `json:"min_replicas,omitempty" jsonschema:"Minimum replicas"`
	MaxReplicas       int     `json:"max_replicas,omitempty" jsonschema:"Maximum replicas"`
	TargetUtilization float64 `json:"target_utilization,omitempty" jsonschema:"Target utilization (0-1)"`
	CooldownScaleUpS  int     `json:"cooldown_scale_up_s,omitempty" jsonschema:"Cooldown seconds for scale up"`
	CooldownScaleDownS int    `json:"cooldown_scale_down_s,omitempty" jsonschema:"Cooldown seconds for scale down"`
}

type DeleteScalingArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterScalingTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_scaling_policy",
		Description: "Get the auto-scaling policy for a function",
	}, c, func(ctx context.Context, args GetScalingArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/scaling", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_scaling_policy",
		Description: "Set the auto-scaling policy for a function",
	}, c, func(ctx context.Context, args SetScalingArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{"enabled": true}
		if args.MinReplicas > 0 { body["min_replicas"] = args.MinReplicas }
		if args.MaxReplicas > 0 { body["max_replicas"] = args.MaxReplicas }
		if args.TargetUtilization > 0 { body["target_utilization"] = args.TargetUtilization }
		if args.CooldownScaleUpS > 0 { body["cooldown_scale_up_s"] = args.CooldownScaleUpS }
		if args.CooldownScaleDownS > 0 { body["cooldown_scale_down_s"] = args.CooldownScaleDownS }
		return c.Put(ctx, fmt.Sprintf("/functions/%s/scaling", args.Name), body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_scaling_policy",
		Description: "Delete the auto-scaling policy for a function",
	}, c, func(ctx context.Context, args DeleteScalingArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s/scaling", args.Name))
	})
}
