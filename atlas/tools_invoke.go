package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type InvokeFunctionArgs struct {
	Name    string          `json:"name" jsonschema:"Function name"`
	Payload json.RawMessage `json:"payload,omitempty" jsonschema:"JSON payload to send to the function"`
}

type InvokeAsyncArgs struct {
	Name           string          `json:"name" jsonschema:"Function name"`
	Payload        json.RawMessage `json:"payload,omitempty" jsonschema:"JSON payload"`
	MaxAttempts    int             `json:"max_attempts,omitempty" jsonschema:"Max retry attempts"`
	IdempotencyKey string          `json:"idempotency_key,omitempty" jsonschema:"Idempotency key for deduplication"`
}

func RegisterInvokeTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_invoke_function",
		Description: "Invoke a function synchronously and return the result",
	}, c, func(ctx context.Context, args InvokeFunctionArgs, c *NovaClient) (json.RawMessage, error) {
		payload := args.Payload
		if payload == nil {
			payload = json.RawMessage(`{}`)
		}
		return c.Post(ctx, fmt.Sprintf("/functions/%s/invoke", args.Name), payload)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_invoke_function_async",
		Description: "Invoke a function asynchronously. Returns an async invocation ID.",
	}, c, func(ctx context.Context, args InvokeAsyncArgs, c *NovaClient) (json.RawMessage, error) {
		body := map[string]any{}
		if args.Payload != nil {
			body["payload"] = args.Payload
		}
		if args.MaxAttempts > 0 {
			body["max_attempts"] = args.MaxAttempts
		}
		if args.IdempotencyKey != "" {
			body["idempotency_key"] = args.IdempotencyKey
		}
		return c.Post(ctx, fmt.Sprintf("/functions/%s/invoke-async", args.Name), body)
	})
}
