// Package cache defines an abstract caching interface for hot-path reads.
// Implementations may use in-memory maps (default), Redis, Memcached, or any
// other key-value store. The interface supports typed serialization via
// byte slices, leaving encoding to the caller.
package cache

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a key does not exist in the cache.
var ErrNotFound = errors.New("cache: key not found")

// Cache abstracts a key-value cache with TTL support.
// All operations are safe for concurrent use.
type Cache interface {
	// Get retrieves the value associated with key.
	// Returns ErrNotFound if the key does not exist or has expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with the given TTL. A zero TTL means the entry
	// does not expire (or uses the implementation's default expiration).
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key from the cache. It is not an error to delete
	// a key that does not exist.
	Delete(ctx context.Context, key string) error

	// Exists reports whether the key exists and has not expired.
	Exists(ctx context.Context, key string) (bool, error)

	// Ping verifies connectivity to the underlying cache backend.
	Ping(ctx context.Context) error

	// Close releases all resources held by the cache implementation.
	Close() error
}
