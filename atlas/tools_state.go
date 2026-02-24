package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetFunctionStateArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type PutFunctionStateArgs struct {
	Name  string         `json:"name" jsonschema:"Function name"`
	State map[string]any `json:"state" jsonschema:"State data to store"`
}

type DeleteFunctionStateArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterStateTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_state",
		Description: "Get persisted state for a function",
	}, c, func(ctx context.Context, args GetFunctionStateArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/state", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_put_function_state",
		Description: "Set persisted state for a function",
	}, c, func(ctx context.Context, args PutFunctionStateArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, fmt.Sprintf("/functions/%s/state", args.Name), args.State)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_function_state",
		Description: "Delete persisted state for a function",
	}, c, func(ctx context.Context, args DeleteFunctionStateArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s/state", args.Name))
	})
}
