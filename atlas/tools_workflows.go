package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateWorkflowArgs struct {
	Name        string          `json:"name" jsonschema:"Workflow name"`
	Description string          `json:"description,omitempty" jsonschema:"Workflow description"`
	Definition  json.RawMessage `json:"definition,omitempty" jsonschema:"Workflow definition (JSON DAG)"`
}
type ListWorkflowsArgs struct{}
type GetWorkflowArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
}
type UpdateWorkflowArgs struct {
	Name        string          `json:"name" jsonschema:"Workflow name"`
	Description string          `json:"description,omitempty" jsonschema:"Updated description"`
	Definition  json.RawMessage `json:"definition,omitempty" jsonschema:"Updated definition"`
}
type DeleteWorkflowArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
}
type PublishWorkflowVersionArgs struct {
	Name       string          `json:"name" jsonschema:"Workflow name"`
	Definition json.RawMessage `json:"definition,omitempty" jsonschema:"Version definition"`
}
type ListWorkflowVersionsArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
}
type GetWorkflowVersionArgs struct {
	Name    string `json:"name" jsonschema:"Workflow name"`
	Version int    `json:"version" jsonschema:"Version number"`
}
type RunWorkflowArgs struct {
	Name  string          `json:"name" jsonschema:"Workflow name"`
	Input json.RawMessage `json:"input,omitempty" jsonschema:"Input JSON"`
}
type ListWorkflowRunsArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
}
type GetWorkflowRunArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
	ID   string `json:"id" jsonschema:"Run ID"`
}
type CancelWorkflowRunArgs struct {
	Name string `json:"name" jsonschema:"Workflow name"`
	ID   string `json:"id" jsonschema:"Run ID"`
}

func RegisterWorkflowTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_create_workflow", Description: "Create a new workflow (DAG)"}, c,
		func(ctx context.Context, args CreateWorkflowArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, "/workflows", args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_workflows", Description: "List all workflows"}, c,
		func(ctx context.Context, args ListWorkflowsArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/workflows")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_workflow", Description: "Get workflow details"}, c,
		func(ctx context.Context, args GetWorkflowArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/workflows/%s", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_update_workflow", Description: "Update a workflow"}, c,
		func(ctx context.Context, args UpdateWorkflowArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Put(ctx, fmt.Sprintf("/workflows/%s", args.Name), args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_delete_workflow", Description: "Delete a workflow"}, c,
		func(ctx context.Context, args DeleteWorkflowArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Delete(ctx, fmt.Sprintf("/workflows/%s", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_publish_workflow_version", Description: "Publish a new workflow version"}, c,
		func(ctx context.Context, args PublishWorkflowVersionArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, fmt.Sprintf("/workflows/%s/versions", args.Name), args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_workflow_versions", Description: "List workflow versions"}, c,
		func(ctx context.Context, args ListWorkflowVersionsArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/workflows/%s/versions", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_workflow_version", Description: "Get a specific workflow version"}, c,
		func(ctx context.Context, args GetWorkflowVersionArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/workflows/%s/versions/%d", args.Name, args.Version))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_run_workflow", Description: "Trigger a workflow run"}, c,
		func(ctx context.Context, args RunWorkflowArgs, c *NovaClient) (json.RawMessage, error) {
			body := map[string]any{}
			if args.Input != nil { body["input"] = args.Input }
			return c.Post(ctx, fmt.Sprintf("/workflows/%s/run", args.Name), body)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_workflow_runs", Description: "List runs of a workflow"}, c,
		func(ctx context.Context, args ListWorkflowRunsArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/workflows/%s/runs", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_workflow_run", Description: "Get details of a workflow run"}, c,
		func(ctx context.Context, args GetWorkflowRunArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/workflows/%s/runs/%s", args.Name, args.ID))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_cancel_workflow_run", Description: "Cancel a running workflow"}, c,
		func(ctx context.Context, args CancelWorkflowRunArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, fmt.Sprintf("/workflows/%s/runs/%s/cancel", args.Name, args.ID), map[string]any{})
		})
}
