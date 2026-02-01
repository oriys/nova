package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// TokenBucket Lua script for atomic rate limiting
// KEYS[1] = bucket key
// ARGV[1] = max_tokens (burst size)
// ARGV[2] = refill_rate (tokens per second)
// ARGV[3] = now (current timestamp in seconds)
// ARGV[4] = requested (tokens to consume)
// Returns: {allowed (0/1), remaining_tokens}
var tokenBucketScript = redis.NewScript(`
local bucket = redis.call('HMGET', KEYS[1], 'tokens', 'last_refill')
local tokens = tonumber(bucket[1]) or tonumber(ARGV[1])
local last = tonumber(bucket[2]) or tonumber(ARGV[3])

-- Refill tokens based on elapsed time
local elapsed = tonumber(ARGV[3]) - last
tokens = math.min(tonumber(ARGV[1]), tokens + elapsed * tonumber(ARGV[2]))

local allowed = 0
if tokens >= tonumber(ARGV[4]) then
    tokens = tokens - tonumber(ARGV[4])
    allowed = 1
end

redis.call('HMSET', KEYS[1], 'tokens', tokens, 'last_refill', ARGV[3])
-- Set expiry slightly longer than time to refill bucket
redis.call('EXPIRE', KEYS[1], math.ceil(tonumber(ARGV[1]) / tonumber(ARGV[2])) + 10)

return {allowed, math.floor(tokens)}
`)

// TierConfig holds rate limit configuration for a tier
type TierConfig struct {
	RequestsPerSecond float64
	BurstSize         int
}

// Limiter implements Redis-based token bucket rate limiting
type Limiter struct {
	redis   *redis.Client
	tiers   map[string]TierConfig
	default_ TierConfig
}

// New creates a new rate limiter
func New(redis *redis.Client, tiers map[string]TierConfig, defaultTier TierConfig) *Limiter {
	if tiers == nil {
		tiers = make(map[string]TierConfig)
	}
	return &Limiter{
		redis:   redis,
		tiers:   tiers,
		default_: defaultTier,
	}
}

// Result contains the result of a rate limit check
type Result struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time
}

// Allow checks if a request is allowed for the given key and tier
func (l *Limiter) Allow(ctx context.Context, key, tier string) (Result, error) {
	return l.AllowN(ctx, key, tier, 1)
}

// AllowN checks if N requests are allowed
func (l *Limiter) AllowN(ctx context.Context, key, tier string, n int) (Result, error) {
	cfg := l.getTierConfig(tier)

	now := float64(time.Now().Unix())

	result, err := tokenBucketScript.Run(ctx, l.redis, []string{key},
		cfg.BurstSize,          // ARGV[1] max_tokens
		cfg.RequestsPerSecond,  // ARGV[2] refill_rate
		now,                    // ARGV[3] now
		n,                      // ARGV[4] requested
	).Slice()

	if err != nil {
		return Result{}, fmt.Errorf("rate limit check: %w", err)
	}

	if len(result) != 2 {
		return Result{}, fmt.Errorf("unexpected result length: %d", len(result))
	}

	allowed, _ := result[0].(int64)
	remaining, _ := result[1].(int64)

	// Calculate when bucket will be full again
	tokensNeeded := float64(cfg.BurstSize) - float64(remaining)
	refillSeconds := tokensNeeded / cfg.RequestsPerSecond
	resetAt := time.Now().Add(time.Duration(refillSeconds) * time.Second)

	return Result{
		Allowed:   allowed == 1,
		Remaining: int(remaining),
		ResetAt:   resetAt,
	}, nil
}

// getTierConfig returns the config for a tier, falling back to default
func (l *Limiter) getTierConfig(tier string) TierConfig {
	if cfg, ok := l.tiers[tier]; ok {
		return cfg
	}
	return l.default_
}

// KeyForAPIKey returns the rate limit key for an API key
func KeyForAPIKey(name string) string {
	return "nova:rl:apikey:" + name
}

// KeyForIP returns the rate limit key for an IP address
func KeyForIP(ip string) string {
	return "nova:rl:ip:" + ip
}

// KeyForGlobal returns the rate limit key for anonymous/global requests
func KeyForGlobal(ip string) string {
	return "nova:rl:global:" + ip
}
