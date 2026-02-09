package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateLayerArgs struct {
	Name    string `json:"name" jsonschema:"Layer name"`
	Runtime string `json:"runtime" jsonschema:"Runtime this layer is for"`
	Version string `json:"version,omitempty" jsonschema:"Layer version"`
}
type ListLayersArgs struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset int `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}
type GetLayerArgs struct {
	Name string `json:"name" jsonschema:"Layer name"`
}
type DeleteLayerArgs struct {
	Name string `json:"name" jsonschema:"Layer name"`
}
type SetFunctionLayersArgs struct {
	Name   string   `json:"name" jsonschema:"Function name"`
	Layers []string `json:"layers" jsonschema:"Layer names to attach"`
}
type GetFunctionLayersArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterLayerTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_create_layer", Description: "Create a shared dependency layer"}, c,
		func(ctx context.Context, args CreateLayerArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, "/layers", args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_layers", Description: "List all shared layers"}, c,
		func(ctx context.Context, args ListLayersArgs, c *NovaClient) (json.RawMessage, error) {
			q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
			return c.Get(ctx, "/layers"+q)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_layer", Description: "Get layer details"}, c,
		func(ctx context.Context, args GetLayerArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/layers/%s", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_delete_layer", Description: "Delete a shared layer"}, c,
		func(ctx context.Context, args DeleteLayerArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Delete(ctx, fmt.Sprintf("/layers/%s", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_set_function_layers", Description: "Set layers for a function"}, c,
		func(ctx context.Context, args SetFunctionLayersArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Put(ctx, fmt.Sprintf("/functions/%s/layers", args.Name), map[string]any{"layers": args.Layers})
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_function_layers", Description: "Get layers attached to a function"}, c,
		func(ctx context.Context, args GetFunctionLayersArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/functions/%s/layers", args.Name))
		})
}
