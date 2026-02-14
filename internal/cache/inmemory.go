package cache

import (
	"context"
	"sync"
	"time"
)

// InMemoryCache is a simple in-memory cache implementation that satisfies
// the Cache interface. It can serve as a default cache backend when no
// external cache (e.g., Redis) is available.
type InMemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*memEntry
	closed  bool
}

type memEntry struct {
	value     []byte
	expiresAt time.Time
}

func (e *memEntry) expired() bool {
	return !e.expiresAt.IsZero() && time.Now().After(e.expiresAt)
}

// NewInMemoryCache creates a new in-memory cache with periodic eviction.
func NewInMemoryCache() *InMemoryCache {
	c := &InMemoryCache{
		entries: make(map[string]*memEntry),
	}
	go c.evictLoop()
	return c
}

func (c *InMemoryCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || entry.expired() {
		return nil, ErrNotFound
	}
	// Return a copy to prevent mutation
	cp := make([]byte, len(entry.value))
	copy(cp, entry.value)
	return cp, nil
}

func (c *InMemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	c.entries[key] = &memEntry{value: cp, expiresAt: expiresAt}
	return nil
}

func (c *InMemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	return nil
}

func (c *InMemoryCache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	return ok && !entry.expired(), nil
}

func (c *InMemoryCache) Ping(_ context.Context) error { return nil }

func (c *InMemoryCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.entries = nil
	return nil
}

func (c *InMemoryCache) evictLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		for key, entry := range c.entries {
			if entry.expired() {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}
