package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetFunctionDocsArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}
type SaveFunctionDocsArgs struct {
	Name    string `json:"name" jsonschema:"Function name"`
	Content string `json:"content" jsonschema:"Documentation content"`
}
type DeleteFunctionDocsArgs struct {
	Name string `json:"name" jsonschema:"Function name"`
}
type GetWorkflowDocsArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
}
type SaveWorkflowDocsArgs struct {
	Name    string `json:"name" jsonschema:"Workflow name"`
	Content string `json:"content" jsonschema:"Documentation content"`
}
type DeleteWorkflowDocsArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
}
type CreateDocShareArgs struct {
	Title         string   `json:"title" jsonschema:"Share title"`
	FunctionNames []string `json:"function_names" jsonschema:"Function names to include"`
}
type ListDocSharesArgs struct{}
type DeleteDocShareArgs struct {
	ID string `json:"id" jsonschema:"Share ID"`
}

func RegisterDocTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_docs",
		Description: "Get documentation for a function",
	}, c, func(ctx context.Context, args GetFunctionDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/functions/%s/docs", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_save_function_docs",
		Description: "Save documentation for a function",
	}, c, func(ctx context.Context, args SaveFunctionDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, fmt.Sprintf("/functions/%s/docs", args.Name), map[string]string{"content": args.Content})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_function_docs",
		Description: "Delete documentation for a function",
	}, c, func(ctx context.Context, args DeleteFunctionDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/functions/%s/docs", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_workflow_docs",
		Description: "Get documentation for a workflow",
	}, c, func(ctx context.Context, args GetWorkflowDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, fmt.Sprintf("/workflows/%s/docs", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_save_workflow_docs",
		Description: "Save documentation for a workflow",
	}, c, func(ctx context.Context, args SaveWorkflowDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Put(ctx, fmt.Sprintf("/workflows/%s/docs", args.Name), map[string]string{"content": args.Content})
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_workflow_docs",
		Description: "Delete documentation for a workflow",
	}, c, func(ctx context.Context, args DeleteWorkflowDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/workflows/%s/docs", args.Name))
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_create_doc_share",
		Description: "Create a documentation share link",
	}, c, func(ctx context.Context, args CreateDocShareArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/api-docs/shares", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_doc_shares",
		Description: "List all documentation shares",
	}, c, func(ctx context.Context, args ListDocSharesArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/api-docs/shares")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_delete_doc_share",
		Description: "Delete a documentation share",
	}, c, func(ctx context.Context, args DeleteDocShareArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Delete(ctx, fmt.Sprintf("/api-docs/shares/%s", args.ID))
	})
}
