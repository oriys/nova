package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newTestRedisClient creates a Redis client for testing.
// Tests that require a running Redis instance are skipped automatically.
func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // use a separate DB for tests
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestRedisNotifier_NotifyAndSubscribe(t *testing.T) {
	client := newTestRedisClient(t)
	n := NewRedisNotifier(client)
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := n.Subscribe(ctx, QueueAsync)
	if ch == nil {
		t.Fatal("Subscribe should return non-nil channel")
	}

	// Allow subscription to establish
	time.Sleep(50 * time.Millisecond)

	if err := n.Notify(ctx, QueueAsync); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	select {
	case <-ch:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("expected notification on subscribe channel")
	}
}

func TestRedisNotifier_MultipleQueues(t *testing.T) {
	client := newTestRedisClient(t)
	n := NewRedisNotifier(client)
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	asyncCh := n.Subscribe(ctx, QueueAsync)
	eventCh := n.Subscribe(ctx, QueueEvent)

	time.Sleep(50 * time.Millisecond)

	// Notify only async queue
	if err := n.Notify(ctx, QueueAsync); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	select {
	case <-asyncCh:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("expected notification on async channel")
	}

	select {
	case <-eventCh:
		t.Fatal("should not receive notification on event channel")
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}

func TestRedisNotifier_NonBlocking(t *testing.T) {
	client := newTestRedisClient(t)
	n := NewRedisNotifier(client)
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = n.Subscribe(ctx, QueueAsync)

	time.Sleep(50 * time.Millisecond)

	// Multiple rapid notifications should not block
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			n.Notify(ctx, QueueAsync)
		}
		close(done)
	}()

	select {
	case <-done:
		// expected: non-blocking
	case <-time.After(2 * time.Second):
		t.Fatal("Notify should not block")
	}
}

func TestRedisNotifier_Close(t *testing.T) {
	client := newTestRedisClient(t)
	n := NewRedisNotifier(client)

	ctx := context.Background()
	ch := n.Subscribe(ctx, QueueAsync)

	// Allow subscription goroutine to start
	time.Sleep(50 * time.Millisecond)

	if err := n.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Channel should be closed after Close() (goroutine reacts to context cancel)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after Close()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel should have been closed")
	}

	// Double close should not panic
	if err := n.Close(); err != nil {
		t.Fatalf("Double close should not fail: %v", err)
	}
}

func TestRedisNotifier_ConcurrentAccess(t *testing.T) {
	client := newTestRedisClient(t)
	n := NewRedisNotifier(client)
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const goroutines = 10
	var wg sync.WaitGroup

	// Concurrent subscribers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := n.Subscribe(ctx, QueueAsync)
			select {
			case <-ch:
			case <-time.After(2 * time.Second):
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)

	// Concurrent notifications
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.Notify(ctx, QueueAsync)
		}()
	}

	wg.Wait()
}

func TestRedisNotifier_SubscribeAfterClose(t *testing.T) {
	client := newTestRedisClient(t)
	n := NewRedisNotifier(client)
	n.Close()

	ctx := context.Background()
	ch := n.Subscribe(ctx, QueueAsync)

	// Channel should be immediately closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed when subscribing after Close()")
		}
	case <-time.After(time.Second):
		t.Fatal("channel should have been closed immediately")
	}
}
