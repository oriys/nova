package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucketScript is a Redis Lua script that atomically performs
// token bucket rate limiting. It:
//  1. Reads the current bucket state (tokens + last_refill timestamp)
//  2. Refills tokens based on elapsed time
//  3. Checks if enough tokens are available for the request
//  4. Deducts tokens if allowed
//  5. Returns [allowed (0/1), remaining tokens]
//
// Keys: KEYS[1] = bucket key
// Args: ARGV[1] = max_tokens, ARGV[2] = refill_rate, ARGV[3] = requested, ARGV[4] = now (unix microseconds)
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local requested = tonumber(ARGV[3])
local now = tonumber(ARGV[4])

local bucket = redis.call("HMGET", key, "tokens", "last_refill")
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if tokens == nil then
    tokens = max_tokens
    last_refill = now
end

-- Refill tokens based on elapsed time (microseconds -> seconds)
local elapsed = (now - last_refill) / 1000000.0
if elapsed > 0 then
    tokens = math.min(max_tokens, tokens + elapsed * refill_rate)
end

local allowed = 0
if tokens >= requested then
    tokens = tokens - requested
    allowed = 1
end

redis.call("HMSET", key, "tokens", tostring(tokens), "last_refill", tostring(now))
-- Set TTL to auto-expire idle buckets (2x the time to fully refill)
local ttl = math.ceil(max_tokens / refill_rate * 2)
if ttl < 60 then ttl = 60 end
redis.call("EXPIRE", key, ttl)

return {allowed, math.floor(tokens)}
`)

// RedisBackend implements the Backend interface using Redis for
// high-performance distributed rate limiting. It uses a Lua script
// for atomic token bucket operations, providing throughput of tens
// of thousands of requests per second compared to hundreds with
// database-backed rate limiting.
type RedisBackend struct {
	client *redis.Client
	prefix string
}

// NewRedisBackend creates a new Redis-backed rate limiting backend.
func NewRedisBackend(client *redis.Client) *RedisBackend {
	return &RedisBackend{
		client: client,
		prefix: "nova:rl:",
	}
}

// CheckRateLimit performs an atomic token bucket check using a Redis Lua script.
func (b *RedisBackend) CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error) {
	bucketKey := b.prefix + key

	// Use microseconds for higher precision timing
	nowMicro := redisTimeNow()

	result, err := tokenBucketScript.Run(ctx, b.client, []string{bucketKey},
		maxTokens, refillRate, requested, nowMicro,
	).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("redis rate limit check: %w", err)
	}

	allowed := result[0] == 1
	remaining := int(result[1])
	return allowed, remaining, nil
}

// redisTimeNow returns the current time in microseconds for the Lua script.
var redisTimeNow = func() int64 {
	return time.Now().UnixMicro()
}
