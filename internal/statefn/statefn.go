// Package statefn defines the abstraction for durable per-function state
// management. This enables stateful function patterns where invocations can
// persist and retrieve state across calls, supporting use cases such as
// session tracking, counters, aggregations, and actor-model workflows.
//
// Implementations may use PostgreSQL, Redis, DynamoDB, or any other durable
// key-value store.
package statefn

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrStateNotFound is returned when the requested state key does not exist.
var ErrStateNotFound = errors.New("statefn: state not found")

// Entry represents a single state entry associated with a function.
type Entry struct {
	FunctionID string          `json:"function_id"`
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	Version    int64           `json:"version"` // monotonic version for optimistic concurrency
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
}

// PutOptions configures state write behavior.
type PutOptions struct {
	// TTL sets an expiration for the state entry. Zero means no expiration.
	TTL time.Duration
	// ExpectedVersion enables optimistic concurrency control. If non-zero,
	// the write succeeds only when the current version matches.
	ExpectedVersion int64
}

// ListOptions configures state listing behavior.
type ListOptions struct {
	// Prefix filters keys by a common prefix (e.g. "session:").
	Prefix string
	// Limit caps the number of returned entries.
	Limit int
	// Offset skips the first N matching entries.
	Offset int
}

// StateStore provides durable key-value state scoped to individual functions.
// All keys are namespaced by function ID, ensuring isolation between functions.
type StateStore interface {
	// Get retrieves the state entry for the given function and key.
	// Returns ErrStateNotFound if the key does not exist or has expired.
	Get(ctx context.Context, functionID, key string) (*Entry, error)

	// Put creates or updates a state entry. When PutOptions.ExpectedVersion
	// is set, the write is conditional on the current version matching.
	Put(ctx context.Context, functionID, key string, value json.RawMessage, opts *PutOptions) (*Entry, error)

	// Delete removes a state entry. It is not an error to delete a key
	// that does not exist.
	Delete(ctx context.Context, functionID, key string) error

	// List returns state entries for a function, optionally filtered by
	// prefix. Results are ordered by key.
	List(ctx context.Context, functionID string, opts *ListOptions) ([]*Entry, error)

	// Ping verifies connectivity to the underlying state backend.
	Ping(ctx context.Context) error

	// Close releases all resources held by the state store implementation.
	Close() error
}
