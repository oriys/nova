package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetLogsArgs struct {
	Name      string `json:"name" jsonschema:"Function name"`
	Tail      int    `json:"tail,omitempty" jsonschema:"Last N logs"`
	RequestID string `json:"request_id,omitempty" jsonschema:"Filter by request ID"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results to return"`
	Offset    int    `json:"offset,omitempty" jsonschema:"Number of results to skip"`
}

func RegisterLogTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_function_logs",
		Description: "Get invocation logs for a function",
	}, c, func(ctx context.Context, args GetLogsArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"tail": intStr(args.Tail), "request_id": args.RequestID, "limit": intStr(args.Limit), "offset": intStr(args.Offset)})
		return c.Get(ctx, fmt.Sprintf("/functions/%s/logs%s", args.Name, q))
	})
}
