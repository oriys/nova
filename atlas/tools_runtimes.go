package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListRuntimesArgs struct{}

type CreateRuntimeArgs struct {
	Name    string `json:"name" jsonschema:"Runtime name"`
	Image   string `json:"image,omitempty" jsonschema:"Runtime image path"`
	Command string `json:"command,omitempty" jsonschema:"Command template"`
}

type DeleteRuntimeArgs struct {
	ID string `json:"id" jsonschema:"Runtime ID"`
}

func RegisterRuntimeTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_runtimes",
		Description: "List all available runtimes",
	}, c, func(ctx context.Context, args ListRuntimesArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/runtimes")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_runtime",
		Description: "Create a custom runtime",
	}, c, func(ctx context.Context, args CreateRuntimeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/runtimes", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_runtime",
		Description: "Delete a custom runtime",
	}, c, func(ctx context.Context, args DeleteRuntimeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/runtimes/%s", args.ID))
	})
}
