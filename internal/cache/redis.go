package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements Cache backed by Redis, suitable for use as a
// distributed L2 cache. Combined with InMemoryCache (L1), it provides
// shared metadata caching across multiple Nova/Comet instances.
type RedisCache struct {
	client *redis.Client
	prefix string
}

// RedisCacheConfig holds configuration for the Redis cache.
type RedisCacheConfig struct {
	Addr      string // Redis address (e.g. "localhost:6379")
	Password  string // Redis password
	DB        int    // Redis database number
	KeyPrefix string // Key prefix for namespacing (default: "nova:cache:")
}

// NewRedisCache creates a new Redis-backed cache.
func NewRedisCache(cfg RedisCacheConfig) *RedisCache {
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "nova:cache:"
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return &RedisCache{
		client: client,
		prefix: prefix,
	}
}

// NewRedisCacheFromClient creates a Redis cache using an existing client.
func NewRedisCacheFromClient(client *redis.Client, prefix string) *RedisCache {
	if prefix == "" {
		prefix = "nova:cache:"
	}
	return &RedisCache{
		client: client,
		prefix: prefix,
	}
}

func (c *RedisCache) key(k string) string {
	return c.prefix + k
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, c.key(key)).Bytes()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, c.key(key), value, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, c.key(key)).Err()
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, c.key(key)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}
