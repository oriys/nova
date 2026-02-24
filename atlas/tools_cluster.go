package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListClusterNodesArgs struct{}
type GetClusterNodeArgs struct {
	ID string `json:"id" jsonschema:"Node ID"`
}
type DeleteClusterNodeArgs struct {
	ID string `json:"id" jsonschema:"Node ID"`
}
type ListHealthyNodesArgs struct{}

func RegisterClusterTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_cluster_nodes",
		Description: "List all cluster nodes",
	}, c, func(ctx context.Context, args ListClusterNodesArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/cluster/nodes")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_cluster_node",
		Description: "Get details of a cluster node",
	}, c, func(ctx context.Context, args GetClusterNodeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/cluster/nodes/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_cluster_node",
		Description: "Delete a cluster node",
	}, c, func(ctx context.Context, args DeleteClusterNodeArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/cluster/nodes/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_healthy_nodes",
		Description: "List healthy cluster nodes",
	}, c, func(ctx context.Context, args ListHealthyNodesArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/cluster/nodes/healthy")
	})
}
