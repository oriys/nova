package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// textResult creates a CallToolResult with raw JSON text content.
func textResult(data json.RawMessage) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}

// errResult creates a CallToolResult with an error message.
func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: err.Error()},
		},
		IsError: true,
	}
}

// addToolHelper adds a tool with simplified handler that returns raw JSON.
func addToolHelper[In any](s *mcp.Server, tool *mcp.Tool, client *NovaClient, handler func(ctx context.Context, args In, client *NovaClient) (json.RawMessage, error)) {
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, args In) (*mcp.CallToolResult, any, error) {
		result, err := handler(ctx, args, client)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textResult(result), nil, nil
	})
}

// queryString builds a query string from optional parameters.
func queryString(params map[string]string) string {
	q := ""
	for k, v := range params {
		if v == "" {
			continue
		}
		if q == "" {
			q = "?"
		} else {
			q += "&"
		}
		q += k + "=" + v
	}
	return q
}

func intStr(v int) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%d", v)
}
