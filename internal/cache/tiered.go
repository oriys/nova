package cache

import (
	"context"
	"time"
)

// TieredCache implements Cache with a fast L1 (in-memory) cache backed
// by a shared L2 (typically Redis) cache. Reads check L1 first, falling
// through to L2 on miss and populating L1 on L2 hit. Writes go to both
// layers. This provides low-latency reads with cross-instance consistency
// when combined with cache invalidation via CacheInvalidator.
type TieredCache struct {
	l1    Cache
	l2    Cache
	l1TTL time.Duration // TTL for L1 entries (should be shorter than L2)
}

// NewTieredCache creates a two-level cache.
// l1TTL controls how long items live in the L1 cache (default: 10s).
func NewTieredCache(l1, l2 Cache, l1TTL time.Duration) *TieredCache {
	if l1TTL <= 0 {
		l1TTL = 10 * time.Second
	}
	return &TieredCache{l1: l1, l2: l2, l1TTL: l1TTL}
}

func (t *TieredCache) Get(ctx context.Context, key string) ([]byte, error) {
	// Try L1 first
	val, err := t.l1.Get(ctx, key)
	if err == nil {
		return val, nil
	}

	// L1 miss — try L2
	val, err = t.l2.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// Populate L1 on L2 hit
	_ = t.l1.Set(ctx, key, val, t.l1TTL)
	return val, nil
}

func (t *TieredCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Write to both layers
	_ = t.l1.Set(ctx, key, value, t.l1TTL)
	return t.l2.Set(ctx, key, value, ttl)
}

func (t *TieredCache) Delete(ctx context.Context, key string) error {
	_ = t.l1.Delete(ctx, key)
	return t.l2.Delete(ctx, key)
}

func (t *TieredCache) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := t.l1.Exists(ctx, key)
	if err == nil && ok {
		return true, nil
	}
	return t.l2.Exists(ctx, key)
}

func (t *TieredCache) Ping(ctx context.Context) error {
	if err := t.l1.Ping(ctx); err != nil {
		return err
	}
	return t.l2.Ping(ctx)
}

func (t *TieredCache) Close() error {
	_ = t.l1.Close()
	return t.l2.Close()
}
