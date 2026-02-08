package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateTopicArgs struct {
	Name           string `json:"name" jsonschema:"Topic name"`
	Description    string `json:"description,omitempty" jsonschema:"Topic description"`
	RetentionHours int    `json:"retention_hours,omitempty" jsonschema:"Message retention hours"`
}
type ListTopicsArgs struct{}
type GetTopicArgs struct {
	Name string `json:"name" jsonschema:"Topic name"`
}
type DeleteTopicArgs struct {
	Name string `json:"name" jsonschema:"Topic name"`
}
type PublishEventArgs struct {
	Topic       string          `json:"topic" jsonschema:"Topic name"`
	Payload     json.RawMessage `json:"payload" jsonschema:"Event payload (JSON)"`
	OrderingKey string          `json:"ordering_key,omitempty" jsonschema:"Ordering key for message ordering"`
}
type ListMessagesArgs struct {
	Topic string `json:"topic" jsonschema:"Topic name"`
}
type CreateSubscriptionArgs struct {
	Topic        string `json:"topic" jsonschema:"Topic name"`
	Name         string `json:"name" jsonschema:"Subscription name"`
	FunctionName string `json:"function_name" jsonschema:"Function to invoke on events"`
	MaxAttempts  int    `json:"max_attempts,omitempty" jsonschema:"Max delivery attempts"`
	MaxInflight  int    `json:"max_inflight,omitempty" jsonschema:"Max concurrent deliveries"`
}
type ListTopicSubscriptionsArgs struct {
	Topic string `json:"topic" jsonschema:"Topic name"`
}
type GetSubscriptionArgs struct {
	ID string `json:"id" jsonschema:"Subscription ID"`
}
type UpdateSubscriptionArgs struct {
	ID          string `json:"id" jsonschema:"Subscription ID"`
	Enabled     *bool  `json:"enabled,omitempty" jsonschema:"Enable or disable"`
	MaxAttempts int    `json:"max_attempts,omitempty" jsonschema:"Max delivery attempts"`
	MaxInflight int    `json:"max_inflight,omitempty" jsonschema:"Max concurrent deliveries"`
}
type DeleteSubscriptionArgs struct {
	ID string `json:"id" jsonschema:"Subscription ID"`
}
type ListDeliveriesArgs struct {
	SubscriptionID string `json:"subscription_id" jsonschema:"Subscription ID"`
}
type ReplayEventsArgs struct {
	SubscriptionID string `json:"subscription_id" jsonschema:"Subscription ID"`
	FromSequence   int64  `json:"from_sequence,omitempty" jsonschema:"Replay from this sequence number"`
	FromTime       string `json:"from_time,omitempty" jsonschema:"Replay from this timestamp"`
}
type SeekSubscriptionArgs struct {
	SubscriptionID string `json:"subscription_id" jsonschema:"Subscription ID"`
	ToSequence     int64  `json:"to_sequence,omitempty" jsonschema:"Seek to sequence number"`
	ToTime         string `json:"to_time,omitempty" jsonschema:"Seek to timestamp"`
}
type GetDeliveryArgs struct {
	ID string `json:"id" jsonschema:"Delivery ID"`
}
type RetryDeliveryArgs struct {
	ID string `json:"id" jsonschema:"Delivery ID"`
}
type CreateOutboxArgs struct {
	Topic       string          `json:"topic" jsonschema:"Topic name"`
	Payload     json.RawMessage `json:"payload" jsonschema:"Outbox payload (JSON)"`
	OrderingKey string          `json:"ordering_key,omitempty" jsonschema:"Ordering key"`
}
type ListOutboxArgs struct {
	Topic  string `json:"topic" jsonschema:"Topic name"`
	Status string `json:"status,omitempty" jsonschema:"Filter by status"`
}
type RetryOutboxArgs struct {
	ID string `json:"id" jsonschema:"Outbox entry ID"`
}

func RegisterEventTools(s *mcp.Server, c *NovaClient) {
	addToolHelper(s, &mcp.Tool{Name: "nova_create_topic", Description: "Create an event topic"}, c,
		func(ctx context.Context, args CreateTopicArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, "/topics", args)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_topics", Description: "List all event topics"}, c,
		func(ctx context.Context, args ListTopicsArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, "/topics")
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_topic", Description: "Get event topic details"}, c,
		func(ctx context.Context, args GetTopicArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/topics/%s", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_delete_topic", Description: "Delete an event topic"}, c,
		func(ctx context.Context, args DeleteTopicArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Delete(ctx, fmt.Sprintf("/topics/%s", args.Name))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_publish_event", Description: "Publish an event to a topic"}, c,
		func(ctx context.Context, args PublishEventArgs, c *NovaClient) (json.RawMessage, error) {
			body := map[string]any{"payload": args.Payload}
			if args.OrderingKey != "" { body["ordering_key"] = args.OrderingKey }
			return c.Post(ctx, fmt.Sprintf("/topics/%s/publish", args.Topic), body)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_messages", Description: "List messages in a topic"}, c,
		func(ctx context.Context, args ListMessagesArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/topics/%s/messages", args.Topic))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_create_subscription", Description: "Subscribe a function to a topic"}, c,
		func(ctx context.Context, args CreateSubscriptionArgs, c *NovaClient) (json.RawMessage, error) {
			body := map[string]any{"name": args.Name, "function_name": args.FunctionName}
			if args.MaxAttempts > 0 { body["max_attempts"] = args.MaxAttempts }
			if args.MaxInflight > 0 { body["max_inflight"] = args.MaxInflight }
			return c.Post(ctx, fmt.Sprintf("/topics/%s/subscriptions", args.Topic), body)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_topic_subscriptions", Description: "List subscriptions for a topic"}, c,
		func(ctx context.Context, args ListTopicSubscriptionsArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/topics/%s/subscriptions", args.Topic))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_subscription", Description: "Get subscription details"}, c,
		func(ctx context.Context, args GetSubscriptionArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/subscriptions/%s", args.ID))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_update_subscription", Description: "Update a subscription"}, c,
		func(ctx context.Context, args UpdateSubscriptionArgs, c *NovaClient) (json.RawMessage, error) {
			body := map[string]any{}
			if args.Enabled != nil { body["enabled"] = *args.Enabled }
			if args.MaxAttempts > 0 { body["max_attempts"] = args.MaxAttempts }
			if args.MaxInflight > 0 { body["max_inflight"] = args.MaxInflight }
			return c.Patch(ctx, fmt.Sprintf("/subscriptions/%s", args.ID), body)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_delete_subscription", Description: "Delete a subscription"}, c,
		func(ctx context.Context, args DeleteSubscriptionArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Delete(ctx, fmt.Sprintf("/subscriptions/%s", args.ID))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_deliveries", Description: "List event deliveries for a subscription"}, c,
		func(ctx context.Context, args ListDeliveriesArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/subscriptions/%s/deliveries", args.SubscriptionID))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_replay_events", Description: "Replay events from a sequence or time"}, c,
		func(ctx context.Context, args ReplayEventsArgs, c *NovaClient) (json.RawMessage, error) {
			body := map[string]any{}
			if args.FromSequence > 0 { body["from_sequence"] = args.FromSequence }
			if args.FromTime != "" { body["from_time"] = args.FromTime }
			return c.Post(ctx, fmt.Sprintf("/subscriptions/%s/replay", args.SubscriptionID), body)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_seek_subscription", Description: "Seek subscription to a specific position"}, c,
		func(ctx context.Context, args SeekSubscriptionArgs, c *NovaClient) (json.RawMessage, error) {
			body := map[string]any{}
			if args.ToSequence > 0 { body["to_sequence"] = args.ToSequence }
			if args.ToTime != "" { body["to_time"] = args.ToTime }
			return c.Post(ctx, fmt.Sprintf("/subscriptions/%s/seek", args.SubscriptionID), body)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_get_delivery", Description: "Get delivery details"}, c,
		func(ctx context.Context, args GetDeliveryArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Get(ctx, fmt.Sprintf("/deliveries/%s", args.ID))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_retry_delivery", Description: "Retry a failed event delivery"}, c,
		func(ctx context.Context, args RetryDeliveryArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, fmt.Sprintf("/deliveries/%s/retry", args.ID), map[string]any{})
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_create_outbox", Description: "Create a transactional outbox entry"}, c,
		func(ctx context.Context, args CreateOutboxArgs, c *NovaClient) (json.RawMessage, error) {
			body := map[string]any{"payload": args.Payload}
			if args.OrderingKey != "" { body["ordering_key"] = args.OrderingKey }
			return c.Post(ctx, fmt.Sprintf("/topics/%s/outbox", args.Topic), body)
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_list_outbox", Description: "List outbox entries for a topic"}, c,
		func(ctx context.Context, args ListOutboxArgs, c *NovaClient) (json.RawMessage, error) {
			q := queryString(map[string]string{"status": args.Status})
			return c.Get(ctx, fmt.Sprintf("/topics/%s/outbox%s", args.Topic, q))
		})

	addToolHelper(s, &mcp.Tool{Name: "nova_retry_outbox", Description: "Retry a failed outbox entry"}, c,
		func(ctx context.Context, args RetryOutboxArgs, c *NovaClient) (json.RawMessage, error) {
			return c.Post(ctx, fmt.Sprintf("/outbox/%s/retry", args.ID), map[string]any{})
		})
}
