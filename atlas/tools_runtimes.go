package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListRuntimesArgs struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset int `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}

type CreateRuntimeArgs struct {
	Name    string `json:"name" jsonschema:"Runtime name"`
	Image   string `json:"image,omitempty" jsonschema:"Runtime image path"`
	Command string `json:"command,omitempty" jsonschema:"Command template"`
}

type DeleteRuntimeArgs struct {
	ID string `json:"id" jsonschema:"Runtime ID"`
}

type UploadRuntimeArgs struct {
	ImagePath string `json:"image_path" jsonschema:"Path to the runtime rootfs image file"`
}

func RegisterRuntimeTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_runtimes",
		Description: "List all available runtimes",
	}, c, func(ctx context.Context, args ListRuntimesArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
		return c.Get(ctx, "/runtimes"+q)
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

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_upload_runtime",
		Description: "Upload a runtime rootfs image",
	}, c, func(ctx context.Context, args UploadRuntimeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/runtimes/upload", map[string]any{"image_path": args.ImagePath})
	})
}
