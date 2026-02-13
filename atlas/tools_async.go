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
	Offset int    `json:"offset,omitempty" jsonschema:"Number of results to skip"`
	Status string `json:"status,omitempty" jsonschema:"Filter by status (queued running succeeded dlq paused)"`
}

type GetAsyncInvocationArgs struct {
	ID string `json:"id" jsonschema:"Async invocation ID"`
}

type RetryAsyncInvocationArgs struct {
	ID string `json:"id" jsonschema:"Async invocation ID to retry"`
}

type PauseAsyncInvocationArgs struct {
	ID string `json:"id" jsonschema:"Async invocation ID to pause"`
}

type ResumeAsyncInvocationArgs struct {
	ID string `json:"id" jsonschema:"Async invocation ID to resume"`
}

type DeleteAsyncInvocationArgs struct {
	ID string `json:"id" jsonschema:"Async invocation ID to delete"`
}

func RegisterAsyncInvocationTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_async_invocations",
		Description: "List async invocations. Optionally filter by function name and status.",
	}, c, func(ctx context.Context, args ListAsyncInvocationsArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"limit": intStr(args.Limit), "offset": intStr(args.Offset), "status": args.Status})
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

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_pause_async_invocation",
		Description: "Pause a queued async invocation to prevent it from being consumed",
	}, c, func(ctx context.Context, args PauseAsyncInvocationArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/async-invocations/%s/pause", args.ID), map[string]any{})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_resume_async_invocation",
		Description: "Resume a paused async invocation back to queued status",
	}, c, func(ctx context.Context, args ResumeAsyncInvocationArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/async-invocations/%s/resume", args.ID), map[string]any{})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_async_invocation",
		Description: "Delete an unconsumed async invocation (must be in queued or paused status)",
	}, c, func(ctx context.Context, args DeleteAsyncInvocationArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/async-invocations/%s", args.ID))
	})
}
