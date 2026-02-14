package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRedisListNotifier_NotifyAndSubscribe(t *testing.T) {
	client := newTestRedisClient(t)
	// Clean up the test key before starting
	client.Del(context.Background(), redisListPrefix+string(QueueAsync))

	n := NewRedisListNotifier(client)
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := n.Subscribe(ctx, QueueAsync)
	if ch == nil {
		t.Fatal("Subscribe should return non-nil channel")
	}

	// Allow subscription goroutine to start BRPOP
	time.Sleep(50 * time.Millisecond)

	if err := n.Notify(ctx, QueueAsync); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	select {
	case <-ch:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("expected notification on subscribe channel")
	}
}

func TestRedisListNotifier_MultipleQueues(t *testing.T) {
	client := newTestRedisClient(t)
	client.Del(context.Background(), redisListPrefix+string(QueueAsync))
	client.Del(context.Background(), redisListPrefix+string(QueueEvent))

	n := NewRedisListNotifier(client)
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
	case <-time.After(3 * time.Second):
		t.Fatal("expected notification on async channel")
	}

	select {
	case <-eventCh:
		t.Fatal("should not receive notification on event channel")
	case <-time.After(200 * time.Millisecond):
		// expected
	}
}

func TestRedisListNotifier_LoadBalancing(t *testing.T) {
	client := newTestRedisClient(t)
	client.Del(context.Background(), redisListPrefix+string(QueueAsync))

	n := NewRedisListNotifier(client)
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Two subscribers competing for signals
	ch1 := n.Subscribe(ctx, QueueAsync)
	ch2 := n.Subscribe(ctx, QueueAsync)

	time.Sleep(50 * time.Millisecond)

	// Send one notification — only one subscriber should receive it
	if err := n.Notify(ctx, QueueAsync); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	received := 0
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for received < 2 {
		select {
		case <-ch1:
			received++
		case <-ch2:
			received++
		case <-timer.C:
			goto done
		}
	}
done:
	if received != 1 {
		t.Fatalf("expected exactly 1 subscriber to receive the signal, got %d", received)
	}
}

func TestRedisListNotifier_NonBlocking(t *testing.T) {
	client := newTestRedisClient(t)
	client.Del(context.Background(), redisListPrefix+string(QueueAsync))

	n := NewRedisListNotifier(client)
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

func TestRedisListNotifier_Close(t *testing.T) {
	client := newTestRedisClient(t)
	client.Del(context.Background(), redisListPrefix+string(QueueAsync))

	n := NewRedisListNotifier(client)

	ctx := context.Background()
	ch := n.Subscribe(ctx, QueueAsync)

	// Allow subscription goroutine to start
	time.Sleep(50 * time.Millisecond)

	if err := n.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Channel should be closed after Close()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after Close()")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel should have been closed")
	}

	// Double close should not panic
	if err := n.Close(); err != nil {
		t.Fatalf("Double close should not fail: %v", err)
	}
}

func TestRedisListNotifier_ConcurrentAccess(t *testing.T) {
	client := newTestRedisClient(t)
	client.Del(context.Background(), redisListPrefix+string(QueueAsync))

	n := NewRedisListNotifier(client)
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
			case <-time.After(3 * time.Second):
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

func TestRedisListNotifier_SubscribeAfterClose(t *testing.T) {
	client := newTestRedisClient(t)
	n := NewRedisListNotifier(client)
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

func TestRedisListNotifier_SignalPersistence(t *testing.T) {
	client := newTestRedisClient(t)
	client.Del(context.Background(), redisListPrefix+string(QueueAsync))

	n := NewRedisListNotifier(client)
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Push signals BEFORE any subscriber is listening — these must not be lost
	for i := 0; i < 3; i++ {
		if err := n.Notify(ctx, QueueAsync); err != nil {
			t.Fatalf("Notify failed: %v", err)
		}
	}

	// Now subscribe — should pick up the queued signals
	ch := n.Subscribe(ctx, QueueAsync)
	received := 0
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	for received < 3 {
		select {
		case <-ch:
			received++
		case <-timer.C:
			t.Fatalf("expected 3 notifications, got %d", received)
		}
	}
}
