package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetCodeArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type UpdateCodeArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
	Code string `json:"code" jsonschema:"New source code"`
}

type ListFilesArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterCodeTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_code",
		Description: "Get the source code of a function",
	}, c, func(ctx context.Context, args GetCodeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/code", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_update_function_code",
		Description: "Update the source code of a function",
	}, c, func(ctx context.Context, args UpdateCodeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, fmt.Sprintf("/functions/%s/code", args.Name), map[string]string{"code": args.Code})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_function_files",
		Description: "List files in a function's code directory",
	}, c, func(ctx context.Context, args ListFilesArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/files", args.Name))
	})
}
