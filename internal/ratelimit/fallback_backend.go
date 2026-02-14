package ratelimit

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oriys/nova/internal/logging"
)

// FallbackBackend wraps a primary Backend (typically Redis) with an in-memory
// local token bucket fallback. When the primary backend returns an error, it
// automatically degrades to local rate limiting and periodically probes the
// primary to restore distributed behaviour once connectivity recovers.
type FallbackBackend struct {
	primary       Backend
	local         *LocalTokenBucketBackend
	degraded      atomic.Bool
	probeMu       sync.Mutex
	lastProbeTime atomic.Value // time.Time — throttle probe frequency
}

// NewFallbackBackend creates a rate-limit backend that falls back to local
// in-memory token buckets when the primary backend is unavailable.
func NewFallbackBackend(primary Backend) *FallbackBackend {
	fb := &FallbackBackend{
		primary: primary,
		local:   NewLocalTokenBucketBackend(),
	}
	fb.lastProbeTime.Store(time.Time{})
	return fb
}

// probeInterval is the minimum time between health probes of the primary backend.
const probeInterval = 5 * time.Second

func (f *FallbackBackend) CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error) {
	if f.degraded.Load() {
		// In degraded mode – probe primary at most every probeInterval.
		if last, ok := f.lastProbeTime.Load().(time.Time); ok && time.Since(last) > probeInterval {
			go f.probeAndRecover(ctx)
		}
		return f.local.CheckRateLimit(ctx, key, maxTokens, refillRate, requested)
	}

	allowed, remaining, err := f.primary.CheckRateLimit(ctx, key, maxTokens, refillRate, requested)
	if err != nil {
		logging.Op().Warn("rate-limit primary backend error, degrading to local", "error", err)
		f.degraded.Store(true)
		f.lastProbeTime.Store(time.Now())
		return f.local.CheckRateLimit(ctx, key, maxTokens, refillRate, requested)
	}
	return allowed, remaining, nil
}

// probeAndRecover periodically checks if the primary backend has recovered.
func (f *FallbackBackend) probeAndRecover(ctx context.Context) {
	if !f.probeMu.TryLock() {
		return // another goroutine is already probing
	}
	defer f.probeMu.Unlock()

	f.lastProbeTime.Store(time.Now())

	// Use a small test check to see if primary is healthy
	_, _, err := f.primary.CheckRateLimit(ctx, "nova:rl:probe:health", 1000, 1000, 0)
	if err == nil {
		logging.Op().Info("rate-limit primary backend recovered, resuming distributed mode")
		f.degraded.Store(false)
	}
}

// Degraded reports whether the backend is currently in degraded (local) mode.
func (f *FallbackBackend) Degraded() bool {
	return f.degraded.Load()
}

// LocalTokenBucketBackend implements Backend using in-memory token buckets.
// It is used as a fallback when the distributed backend is unavailable.
type LocalTokenBucketBackend struct {
	mu      sync.Mutex
	buckets map[string]*localBucket
}

type localBucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewLocalTokenBucketBackend creates a local in-memory token bucket backend.
func NewLocalTokenBucketBackend() *LocalTokenBucketBackend {
	return &LocalTokenBucketBackend{
		buckets: make(map[string]*localBucket),
	}
}

func (l *LocalTokenBucketBackend) CheckRateLimit(_ context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &localBucket{
			tokens:     float64(maxTokens),
			lastRefill: now,
		}
		l.buckets[key] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(float64(maxTokens), b.tokens+elapsed*refillRate)
		b.lastRefill = now
	}

	if b.tokens >= float64(requested) {
		b.tokens -= float64(requested)
		return true, int(b.tokens), nil
	}
	return false, int(b.tokens), nil
}
