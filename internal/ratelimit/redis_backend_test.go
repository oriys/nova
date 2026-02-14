package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping: %v", err)
	}
	t.Cleanup(func() {
		client.FlushDB(context.Background())
		client.Close()
	})
	return client
}

func TestRedisBackend_AllowRequest(t *testing.T) {
	client := newTestRedisClient(t)
	b := NewRedisBackend(client)
	ctx := context.Background()

	allowed, remaining, err := b.CheckRateLimit(ctx, "test:allow", 10, 10.0, 1)
	if err != nil {
		t.Fatalf("CheckRateLimit failed: %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}
	if remaining != 9 {
		t.Fatalf("expected 9 remaining, got %d", remaining)
	}
}

func TestRedisBackend_DenyWhenExhausted(t *testing.T) {
	client := newTestRedisClient(t)
	b := NewRedisBackend(client)
	ctx := context.Background()

	// Exhaust all tokens
	for i := 0; i < 5; i++ {
		b.CheckRateLimit(ctx, "test:deny", 5, 1.0, 1)
	}

	allowed, remaining, err := b.CheckRateLimit(ctx, "test:deny", 5, 1.0, 1)
	if err != nil {
		t.Fatalf("CheckRateLimit failed: %v", err)
	}
	if allowed {
		t.Fatal("request should be denied when tokens exhausted")
	}
	if remaining != 0 {
		t.Fatalf("expected 0 remaining, got %d", remaining)
	}
}

func TestRedisBackend_BurstRequests(t *testing.T) {
	client := newTestRedisClient(t)
	b := NewRedisBackend(client)
	ctx := context.Background()

	// Request more than one token at once
	allowed, remaining, err := b.CheckRateLimit(ctx, "test:burst", 10, 5.0, 5)
	if err != nil {
		t.Fatalf("CheckRateLimit failed: %v", err)
	}
	if !allowed {
		t.Fatal("burst request should be allowed")
	}
	if remaining != 5 {
		t.Fatalf("expected 5 remaining, got %d", remaining)
	}
}

func TestRedisBackend_Refill(t *testing.T) {
	client := newTestRedisClient(t)
	b := NewRedisBackend(client)
	ctx := context.Background()

	// Exhaust all tokens
	b.CheckRateLimit(ctx, "test:refill", 2, 100.0, 2)

	// Wait for refill (100 tokens/sec, need 1)
	time.Sleep(50 * time.Millisecond)

	allowed, _, err := b.CheckRateLimit(ctx, "test:refill", 2, 100.0, 1)
	if err != nil {
		t.Fatalf("CheckRateLimit failed: %v", err)
	}
	if !allowed {
		t.Fatal("request should be allowed after refill period")
	}
}

func TestRedisBackend_InterfaceCompliance(t *testing.T) {
	// Verify RedisBackend implements Backend
	var _ Backend = (*RedisBackend)(nil)
}
