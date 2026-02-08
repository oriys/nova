package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetCapacityArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type SetCapacityArgs struct {
	Name           string `json:"name" jsonschema:"Function name"`
	MaxInflight    int    `json:"max_inflight,omitempty" jsonschema:"Max in-flight requests"`
	MaxQueueDepth  int    `json:"max_queue_depth,omitempty" jsonschema:"Max queue depth"`
	MaxQueueWaitMs int64  `json:"max_queue_wait_ms,omitempty" jsonschema:"Max queue wait time in ms"`
	ShedStatusCode int    `json:"shed_status_code,omitempty" jsonschema:"HTTP status code for shed requests (429 or 503)"`
}

type DeleteCapacityArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterCapacityTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_capacity_policy",
		Description: "Get the capacity/admission control policy for a function",
	}, c, func(ctx context.Context, args GetCapacityArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/capacity", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_capacity_policy",
		Description: "Set the capacity/admission control policy for a function",
	}, c, func(ctx context.Context, args SetCapacityArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{"enabled": true}
		if args.MaxInflight > 0 { body["max_inflight"] = args.MaxInflight }
		if args.MaxQueueDepth > 0 { body["max_queue_depth"] = args.MaxQueueDepth }
		if args.MaxQueueWaitMs > 0 { body["max_queue_wait_ms"] = args.MaxQueueWaitMs }
		if args.ShedStatusCode > 0 { body["shed_status_code"] = args.ShedStatusCode }
		return c.Put(ctx, fmt.Sprintf("/functions/%s/capacity", args.Name), body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_capacity_policy",
		Description: "Delete the capacity policy for a function",
	}, c, func(ctx context.Context, args DeleteCapacityArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s/capacity", args.Name))
	})
}
