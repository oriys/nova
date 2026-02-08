package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateApiKeyArgs struct {
	Name   string   `json:"name" jsonschema:"API key name"`
	Scopes []string `json:"scopes,omitempty" jsonschema:"Permission scopes"`
}
type ListApiKeysArgs struct{}
type DeleteApiKeyArgs struct {
	ID string `json:"id" jsonschema:"API key ID"`
}
type UpdateApiKeyArgs struct {
	ID     string   `json:"id" jsonschema:"API key ID"`
	Name   string   `json:"name,omitempty" jsonschema:"New name"`
	Scopes []string `json:"scopes,omitempty" jsonschema:"Updated scopes"`
}

func RegisterApiKeyTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_create_apikey", Description: "Create a new API key"}, c,
		func(ctx context.Context, args CreateApiKeyArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, "/api-keys", args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_apikeys", Description: "List all API keys"}, c,
		func(ctx context.Context, args ListApiKeysArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/api-keys")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_delete_apikey", Description: "Revoke an API key"}, c,
		func(ctx context.Context, args DeleteApiKeyArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Delete(ctx, fmt.Sprintf("/api-keys/%s", args.ID))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_update_apikey", Description: "Update an API key"}, c,
		func(ctx context.Context, args UpdateApiKeyArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Patch(ctx, fmt.Sprintf("/api-keys/%s", args.ID), args)
		})
}
