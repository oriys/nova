package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AIGenerateArgs struct {
	FunctionName string `json:"function_name" jsonschema:"Function name"`
	Prompt       string `json:"prompt" jsonschema:"Generation prompt"`
}
type AIReviewArgs struct {
	FunctionName string `json:"function_name" jsonschema:"Function name"`
}
type AIRewriteArgs struct {
	FunctionName string `json:"function_name" jsonschema:"Function name"`
	Instructions string `json:"instructions" jsonschema:"Rewrite instructions"`
}
type AIGenerateTestsArgs struct {
	FunctionName string `json:"function_name" jsonschema:"Function name"`
}
type AIGenerateDocsArgs struct {
	FunctionName string `json:"function_name" jsonschema:"Function name"`
}
type AIGenerateWorkflowDocsArgs struct {
	WorkflowName string `json:"workflow_name" jsonschema:"Workflow name"`
}
type AIStatusArgs struct{}
type AIListModelsArgs struct{}

func RegisterAITools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_generate",
		Description: "Generate function code using AI",
	}, c, func(ctx context.Context, args AIGenerateArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/ai/generate", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_review",
		Description: "Review function code using AI",
	}, c, func(ctx context.Context, args AIReviewArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/ai/review", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_rewrite",
		Description: "Rewrite function code using AI",
	}, c, func(ctx context.Context, args AIRewriteArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/ai/rewrite", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_generate_tests",
		Description: "Generate tests for a function using AI",
	}, c, func(ctx context.Context, args AIGenerateTestsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/ai/generate-tests", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_generate_docs",
		Description: "Generate documentation for a function using AI",
	}, c, func(ctx context.Context, args AIGenerateDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/ai/generate-docs", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_generate_workflow_docs",
		Description: "Generate documentation for a workflow using AI",
	}, c, func(ctx context.Context, args AIGenerateWorkflowDocsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/ai/generate-workflow-docs", args)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_status",
		Description: "Get AI service status",
	}, c, func(ctx context.Context, args AIStatusArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/ai/status")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_ai_list_models",
		Description: "List available AI models",
	}, c, func(ctx context.Context, args AIListModelsArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/ai/models")
	})
}
