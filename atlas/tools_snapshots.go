package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListSnapshotsArgs struct{}

type CreateSnapshotArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

type DeleteSnapshotArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterSnapshotTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_snapshots",
		Description: "List all VM snapshots",
	}, c, func(ctx context.Context, args ListSnapshotsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/snapshots")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_snapshot",
		Description: "Create a VM snapshot for a function (pause and save state)",
	}, c, func(ctx context.Context, args CreateSnapshotArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/functions/%s/snapshot", args.Name), map[string]any{})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_snapshot",
		Description: "Delete a VM snapshot for a function",
	}, c, func(ctx context.Context, args DeleteSnapshotArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s/snapshot", args.Name))
	})
}
