package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ListNotificationsArgs struct {
	Status string `json:"status,omitempty" jsonschema:"Filter by status (read unread)"`
}
type GetUnreadCountArgs struct{}
type MarkNotificationReadArgs struct {
	ID string `json:"id" jsonschema:"Notification ID"`
}
type MarkAllReadArgs struct{}

func RegisterNotificationTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{
		Name:        "nova_list_notifications",
		Description: "List notifications",
	}, c, func(ctx context.Context, args ListNotificationsArgs, c *NovaClient) (json.RawMessage, error) {
		q := queryString(map[string]string{"status": args.Status})
		return c.Get(ctx, "/notifications"+q)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_get_unread_count",
		Description: "Get unread notification count",
	}, c, func(ctx context.Context, args GetUnreadCountArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Get(ctx, "/notifications/unread-count")
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_mark_notification_read",
		Description: "Mark a notification as read",
	}, c, func(ctx context.Context, args MarkNotificationReadArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, fmt.Sprintf("/notifications/%s/read", args.ID), nil)
	})

	addToolHelper(s, &mcp.Tool{
		Name:        "nova_mark_all_read",
		Description: "Mark all notifications as read",
	}, c, func(ctx context.Context, args MarkAllReadArgs, c *NovaClient) (json.RawMessage, error) {
		return c.Post(ctx, "/notifications/read-all", nil)
	})
}
