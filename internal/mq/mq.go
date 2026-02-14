// Package mq defines an abstract message queue interface for async invocations
// and event delivery. Implementations may use PostgreSQL polling (default),
// Redis Streams, NATS JetStream, RabbitMQ, or any other message broker.
package mq

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNoMessage is returned when no message is available for consumption.
var ErrNoMessage = errors.New("mq: no message available")

// Message represents a single message in the queue.
type Message struct {
	ID        string          `json:"id"`
	Topic     string          `json:"topic"`
	Payload   json.RawMessage `json:"payload"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Attempt   int             `json:"attempt"`
	CreatedAt time.Time       `json:"created_at"`
}

// PublishOptions configures message publishing behavior.
type PublishOptions struct {
	// DelayUntil defers message visibility until the specified time.
	DelayUntil time.Time
	// IdempotencyKey prevents duplicate messages within the dedup window.
	IdempotencyKey string
	// IdempotencyTTL is the deduplication window for the idempotency key.
	IdempotencyTTL time.Duration
}

// ConsumeOptions configures message consumption behavior.
type ConsumeOptions struct {
	// LeaseDuration is how long the consumer holds the message before it
	// becomes available again for redelivery.
	LeaseDuration time.Duration
	// WorkerID identifies the consumer instance for lease tracking.
	WorkerID string
}

// MessageQueue abstracts a durable message queue with at-least-once delivery.
// Implementations must guarantee that acknowledged messages are not redelivered
// and that unacknowledged messages become available again after the lease expires.
type MessageQueue interface {
	// Publish enqueues a message onto the given topic.
	Publish(ctx context.Context, topic string, payload json.RawMessage, opts *PublishOptions) (messageID string, err error)

	// Consume retrieves the next available message from the given topic.
	// Returns ErrNoMessage when the topic is empty or all messages are leased.
	Consume(ctx context.Context, topic string, opts *ConsumeOptions) (*Message, error)

	// Ack acknowledges successful processing of a message, removing it from
	// the queue or marking it as completed.
	Ack(ctx context.Context, messageID string) error

	// Nack signals that message processing failed. The message is scheduled
	// for redelivery at nextRetry, or moved to the dead-letter queue if
	// attempts are exhausted.
	Nack(ctx context.Context, messageID string, reason string, nextRetry time.Time) error

	// Dead-letter: move a message to the dead-letter topic after all retries
	// are exhausted.
	DeadLetter(ctx context.Context, messageID string, reason string) error

	// Ping verifies connectivity to the underlying broker.
	Ping(ctx context.Context) error

	// Close releases all resources held by the queue implementation.
	Close() error
}
