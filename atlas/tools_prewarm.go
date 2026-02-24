package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type PrewarmFunctionArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}

func RegisterPrewarmTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_prewarm_function",
		Description: "Pre-warm a function by starting a VM instance ahead of time",
	}, c, func(ctx context.Context, args PrewarmFunctionArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/functions/%s/prewarm", args.Name), map[string]any{})
	})
}
