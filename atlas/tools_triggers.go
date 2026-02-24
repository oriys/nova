package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateTriggerArgs struct {
	Name         string         `json:"name" jsonschema:"Trigger name"`
	FunctionName string         `json:"function_name" jsonschema:"Function to invoke"`
	Type         string         `json:"type" jsonschema:"Trigger type"`
	Config       map[string]any `json:"config,omitempty" jsonschema:"Trigger configuration"`
}

type ListTriggersArgs struct {
	FunctionName string `json:"function_name,omitempty" jsonschema:"Filter by function name"`
}

type GetTriggerArgs struct {
	ID string `json:"id" jsonschema:"Trigger ID"`
}

type UpdateTriggerArgs struct {
	ID      string         `json:"id" jsonschema:"Trigger ID"`
	Enabled *bool          `json:"enabled,omitempty" jsonschema:"Enable or disable"`
	Config  map[string]any `json:"config,omitempty" jsonschema:"Trigger configuration"`
}

type DeleteTriggerArgs struct {
	ID string `json:"id" jsonschema:"Trigger ID"`
}

func RegisterTriggerTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_trigger",
		Description: "Create a trigger for a function",
	}, c, func(ctx context.Context, args CreateTriggerArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{
			"name":          args.Name,
			"function_name": args.FunctionName,
			"type":          args.Type,
		}
		if args.Config != nil {
			body["config"] = args.Config
		}
		return c.Post(ctx, "/triggers", body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_triggers",
		Description: "List triggers, optionally filtered by function name",
	}, c, func(ctx context.Context, args ListTriggersArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"function_name": args.FunctionName})
		return c.Get(ctx, "/triggers"+q)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_trigger",
		Description: "Get trigger details",
	}, c, func(ctx context.Context, args GetTriggerArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/triggers/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_update_trigger",
		Description: "Update a trigger",
	}, c, func(ctx context.Context, args UpdateTriggerArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{}
		if args.Enabled != nil {
			body["enabled"] = *args.Enabled
		}
		if args.Config != nil {
			body["config"] = args.Config
		}
		return c.Patch(ctx, fmt.Sprintf("/triggers/%s", args.ID), body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_trigger",
		Description: "Delete a trigger",
	}, c, func(ctx context.Context, args DeleteTriggerArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/triggers/%s", args.ID))
	})
}
