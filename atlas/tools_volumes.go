package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateVolumeArgs struct {
	Name        string `json:"name" jsonschema:"Volume name"`
	SizeMB      int    `json:"size_mb" jsonschema:"Volume size in megabytes"`
	Description string `json:"description,omitempty" jsonschema:"Volume description"`
}

type ListVolumesArgs struct{}

type GetVolumeArgs struct {
	Name string `json:"name" jsonschema:"Volume name"`
}

type DeleteVolumeArgs struct {
	Name string `json:"name" jsonschema:"Volume name"`
}

type SetFunctionMountsArgs struct {
	Name   string              `json:"name" jsonschema:"Function name"`
	Mounts []map[string]string `json:"mounts" jsonschema:"Array of mount configurations"`
}

type GetFunctionMountsArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterVolumeTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_volume",
		Description: "Create a persistent volume",
	}, c, func(ctx context.Context, args CreateVolumeArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{"name": args.Name, "size_mb": args.SizeMB}
		if args.Description != "" {
			body["description"] = args.Description
		}
		return c.Post(ctx, "/volumes", body)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_volumes",
		Description: "List all volumes",
	}, c, func(ctx context.Context, args ListVolumesArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/volumes")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_volume",
		Description: "Get volume details",
	}, c, func(ctx context.Context, args GetVolumeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/volumes/%s", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_volume",
		Description: "Delete a volume",
	}, c, func(ctx context.Context, args DeleteVolumeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/volumes/%s", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_set_function_mounts",
		Description: "Set volume mounts for a function",
	}, c, func(ctx context.Context, args SetFunctionMountsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, fmt.Sprintf("/functions/%s/mounts", args.Name), map[string]any{"mounts": args.Mounts})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_mounts",
		Description: "Get volume mounts for a function",
	}, c, func(ctx context.Context, args GetFunctionMountsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/mounts", args.Name))
	})
}
