package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type HealthArgs struct{}
type StatsArgs struct{}

func RegisterHealthTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_health", Description: "Get Nova health status including postgres and pool stats"}, c,
		func(ctx context.Context, args HealthArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/health")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_health_live", Description: "Kubernetes liveness probe"}, c,
		func(ctx context.Context, args HealthArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/health/live")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_health_ready", Description: "Kubernetes readiness probe"}, c,
		func(ctx context.Context, args HealthArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/health/ready")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_health_startup", Description: "Kubernetes startup probe"}, c,
		func(ctx context.Context, args HealthArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/health/startup")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_stats", Description: "Get VM pool statistics"}, c,
		func(ctx context.Context, args StatsArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/stats")
		})
}
