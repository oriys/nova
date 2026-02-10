package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateSecretArgs struct {
	Name  string `json:"name" jsonschema:"Secret name"`
	Value string `json:"value" jsonschema:"Secret value"`
}
type ListSecretsArgs struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset int `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}
type DeleteSecretArgs struct {
	Name string `json:"name" jsonschema:"Secret name"`
}

func RegisterSecretTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_create_secret", Description: "Create a secret"}, c,
		func(ctx context.Context, args CreateSecretArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, "/secrets", args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_secrets", Description: "List all secrets (names only, no values)"}, c,
		func(ctx context.Context, args ListSecretsArgs, c *NovaClient) (json.RawMessage, error) {
			q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset)})
			return c.Get(ctx, "/secrets"+q)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_delete_secret", Description: "Delete a secret"}, c,
		func(ctx context.Context, args DeleteSecretArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Delete(ctx, fmt.Sprintf("/secrets/%s", args.Name))
		})
}
