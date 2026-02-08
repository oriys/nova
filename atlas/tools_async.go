package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListAsyncInvocationsArgs struct {
	Name   string `json:"name,omitempty" jsonschema:"Function name (omit for all functions)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Max results"`
	Status string `json:"status,omitempty" jsonschema:"Filter by status (queued running succeeded dlq)"`
}

type GetAsyncInvocationArgs struct {
	ID string `json:"id" jsonschema:"Async invocation ID"`
}

type RetryAsyncInvocationArgs struct {
	ID string `json:"id" jsonschema:"Async invocation ID to retry"`
}

func RegisterAsyncInvocationTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_async_invocations",
		Description: "List async invocations. Optionally filter by function name and status.",
	}, c, func(ctx context.Context, args ListAsyncInvocationsArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"limit": intStr(args.Limit), "status": args.Status})
		if args.Name != "" {
			return c.Get(ctx, fmt.Sprintf("/functions/%s/async-invocations%s", args.Name, q))
		}
		return c.Get(ctx, "/async-invocations"+q)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_async_invocation",
		Description: "Get details of a specific async invocation",
	}, c, func(ctx context.Context, args GetAsyncInvocationArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/async-invocations/%s", args.ID))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_retry_async_invocation",
		Description: "Retry a failed async invocation from the DLQ",
	}, c, func(ctx context.Context, args RetryAsyncInvocationArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/async-invocations/%s/retry", args.ID), map[string]any{})
	})
}
