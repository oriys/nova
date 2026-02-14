package cache

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
)

const (
	// InvalidationChannel is the Redis Pub/Sub channel used for cache
	// invalidation signals. When a control-plane node updates function
	// metadata it publishes the affected cache key to this channel. All
	// subscribed nodes delete the key from their L1 cache, ensuring
	// cross-instance consistency without waiting for TTL expiry.
	InvalidationChannel = "nova:cache:invalidate"
)

// CacheInvalidator listens for CACHE_INVALIDATE signals over Redis Pub/Sub
// and evicts the corresponding keys from a local cache (typically the L1
// in-memory cache in a tiered setup).
type CacheInvalidator struct {
	local  Cache
	client *redis.Client
	mu     sync.Mutex
	cancel context.CancelFunc
	closed bool
}

// NewCacheInvalidator creates a cache invalidator that subscribes to Redis
// Pub/Sub and invalidates keys in the local cache when signals arrive.
func NewCacheInvalidator(local Cache, client *redis.Client) *CacheInvalidator {
	return &CacheInvalidator{
		local:  local,
		client: client,
	}
}

// Start begins listening for invalidation signals. It blocks until the
// context is cancelled or Close is called.
func (ci *CacheInvalidator) Start(ctx context.Context) {
	subCtx, cancel := context.WithCancel(ctx)
	ci.mu.Lock()
	ci.cancel = cancel
	ci.mu.Unlock()

	pubsub := ci.client.Subscribe(subCtx, InvalidationChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-subCtx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// msg.Payload is the cache key to invalidate
			_ = ci.local.Delete(subCtx, msg.Payload)
		}
	}
}

// PublishInvalidation publishes a cache invalidation signal for the given key.
// This is called by the control plane when function metadata is updated.
func (ci *CacheInvalidator) PublishInvalidation(ctx context.Context, key string) error {
	return ci.client.Publish(ctx, InvalidationChannel, key).Err()
}

// Close stops the invalidation listener.
func (ci *CacheInvalidator) Close() error {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	if ci.closed {
		return nil
	}
	ci.closed = true
	if ci.cancel != nil {
		ci.cancel()
	}
	return nil
}
