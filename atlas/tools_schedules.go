package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateScheduleArgs struct {
	Name string          `json:"name" jsonschema:"Function name"`
	Cron string          `json:"cron_expression" jsonschema:"Cron expression"`
	Input json.RawMessage `json:"input,omitempty" jsonschema:"Input JSON payload"`
}

type ListSchedulesArgs struct {
	Name   string `json:"name" jsonschema:"Function name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset int    `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}

type DeleteScheduleArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
	ID   string `json:"id" jsonschema:"Schedule ID"`
}

type UpdateScheduleArgs struct {
	Name    string `json:"name" jsonschema:"Function name"`
	ID      string `json:"id" jsonschema:"Schedule ID"`
	Enabled *bool  `json:"enabled,omitempty" jsonschema:"Enable or disable the schedule"`
}

func RegisterScheduleTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_schedule",
		Description: "Create a cron schedule for a function",
	}, c, func(ctx context.Context, args CreateScheduleArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{"cron_expression": args.Cron}
		if args.Input != nil {
			body["input"] = args.Input
		}
		return c.Post(ctx, fmt.Sprintf("/functions/%s/schedules", args.Name), body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_schedules",
		Description: "List all schedules for a function",
	}, c, func(ctx context.Context, args ListSchedulesArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
		return c.Get(ctx, fmt.Sprintf("/functions/%s/schedules%s", args.Name, q))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_schedule",
		Description: "Delete a schedule",
	}, c, func(ctx context.Context, args DeleteScheduleArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s/schedules/%s", args.Name, args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_update_schedule",
		Description: "Toggle a schedule on or off",
	}, c, func(ctx context.Context, args UpdateScheduleArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{}
		if args.Enabled != nil {
			body["enabled"] = *args.Enabled
		}
		return c.Patch(ctx, fmt.Sprintf("/functions/%s/schedules/%s", args.Name, args.ID), body)
	})
}
