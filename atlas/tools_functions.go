package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateFunctionArgs struct {
	Name     string            `json:"name" jsonschema:"Function name"`
	Runtime  string            `json:"runtime" jsonschema:"Runtime (python go node rust etc)"`
	Code     string            `json:"code,omitempty" jsonschema:"Inline source code"`
	CodePath string            `json:"code_path,omitempty" jsonschema:"Path to code file on server"`
	Handler  string            `json:"handler,omitempty" jsonschema:"Handler entry point"`
	MemoryMB int               `json:"memory_mb,omitempty" jsonschema:"Memory in MB"`
	TimeoutS int               `json:"timeout_s,omitempty" jsonschema:"Timeout in seconds"`
	Mode     string            `json:"mode,omitempty" jsonschema:"Execution mode: process or persistent"`
	EnvVars  map[string]string `json:"env_vars,omitempty" jsonschema:"Environment variables"`
}

type ListFunctionsArgs struct {
	Search string `json:"search,omitempty" jsonschema:"Search filter"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Max results to return"`
}

type GetFunctionArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type UpdateFunctionArgs struct {
	Name     string            `json:"name" jsonschema:"Function name"`
	Handler  string            `json:"handler,omitempty" jsonschema:"Handler entry point"`
	MemoryMB int               `json:"memory_mb,omitempty" jsonschema:"Memory in MB"`
	TimeoutS int               `json:"timeout_s,omitempty" jsonschema:"Timeout in seconds"`
	Code     string            `json:"code,omitempty" jsonschema:"Inline source code"`
	Mode     string            `json:"mode,omitempty" jsonschema:"Execution mode"`
	EnvVars  map[string]string `json:"env_vars,omitempty" jsonschema:"Environment variables"`
}

type DeleteFunctionArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterFunctionTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_function",
		Description: "Create a new serverless function",
	}, c, func(ctx context.Context, args CreateFunctionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/functions", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_functions",
		Description: "List all functions. Supports search and limit filters.",
	}, c, func(ctx context.Context, args ListFunctionsArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"search": args.Search, "limit": intStr(args.Limit)})
		return c.Get(ctx, "/functions"+q)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function",
		Description: "Get details of a specific function by name",
	}, c, func(ctx context.Context, args GetFunctionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_update_function",
		Description: "Update an existing function's configuration",
	}, c, func(ctx context.Context, args UpdateFunctionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Patch(ctx, fmt.Sprintf("/functions/%s", args.Name), args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_function",
		Description: "Delete a function by name",
	}, c, func(ctx context.Context, args DeleteFunctionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s", args.Name))
	})
}
