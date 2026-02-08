package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	cfg := LoadConfig()
	client := NewNovaClient(cfg)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "atlas",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Instructions: "Atlas is the MCP server for the Nova serverless platform. " +
			"It exposes Nova's full API as tools, enabling LLM-driven management of " +
			"serverless functions, runtimes, events, workflows, and more. " +
			"All tools are prefixed with nova_ for clear namespacing.",
	})

	// Register all tool groups
	RegisterFunctionTools(server, client)
	RegisterCodeTools(server, client)
	RegisterVersionTools(server, client)
	RegisterInvokeTools(server, client)
	RegisterAsyncInvocationTools(server, client)
	RegisterLogTools(server, client)
	RegisterMetricTools(server, client)
	RegisterScalingTools(server, client)
	RegisterCapacityTools(server, client)
	RegisterScheduleTools(server, client)
	RegisterSnapshotTools(server, client)
	RegisterRuntimeTools(server, client)
	RegisterTenantTools(server, client)
	RegisterEventTools(server, client)
	RegisterWorkflowTools(server, client)
	RegisterGatewayTools(server, client)
	RegisterLayerTools(server, client)
	RegisterApiKeyTools(server, client)
	RegisterSecretTools(server, client)
	RegisterConfigTools(server, client)
	RegisterHealthTools(server, client)
	RegisterObservabilityTools(server, client)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Atlas server failed: %v", err)
	}
}
