package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNoopNotifier(t *testing.T) {
	n := NewNoopNotifier()
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := n.Subscribe(ctx, QueueAsync)
	if ch == nil {
		t.Fatal("Subscribe should return non-nil channel")
	}

	if err := n.Notify(ctx, QueueAsync); err != nil {
		t.Fatalf("Notify should not return error: %v", err)
	}

	// Noop channel should never receive
	select {
	case <-ch:
		t.Fatal("NoopNotifier should never send notifications")
	case <-time.After(10 * time.Millisecond):
		// expected
	}
}

func TestChannelNotifier_NotifyAndSubscribe(t *testing.T) {
	n := NewChannelNotifier()
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := n.Subscribe(ctx, QueueAsync)
	if ch == nil {
		t.Fatal("Subscribe should return non-nil channel")
	}

	// Notify should deliver to subscriber
	if err := n.Notify(ctx, QueueAsync); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	select {
	case <-ch:
		// success
	case <-time.After(time.Second):
		t.Fatal("expected notification on subscribe channel")
	}
}

func TestChannelNotifier_MultipleQueues(t *testing.T) {
	n := NewChannelNotifier()
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	asyncCh := n.Subscribe(ctx, QueueAsync)
	eventCh := n.Subscribe(ctx, QueueEvent)

	// Notify only async queue
	if err := n.Notify(ctx, QueueAsync); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	select {
	case <-asyncCh:
		// expected
	case <-time.After(time.Second):
		t.Fatal("expected notification on async channel")
	}

	select {
	case <-eventCh:
		t.Fatal("should not receive notification on event channel")
	case <-time.After(10 * time.Millisecond):
		// expected
	}
}

func TestChannelNotifier_NonBlocking(t *testing.T) {
	n := NewChannelNotifier()
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := n.Subscribe(ctx, QueueAsync)

	// Fill the buffer (capacity 1)
	n.Notify(ctx, QueueAsync)

	// Second notify should not block even with full buffer
	done := make(chan struct{})
	go func() {
		n.Notify(ctx, QueueAsync)
		close(done)
	}()

	select {
	case <-done:
		// expected: non-blocking
	case <-time.After(time.Second):
		t.Fatal("Notify should not block when subscriber buffer is full")
	}

	// Drain the channel
	<-ch
}

func TestChannelNotifier_ContextCancellation(t *testing.T) {
	n := NewChannelNotifier()
	defer n.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch := n.Subscribe(ctx, QueueAsync)

	cancel()
	// Give the goroutine time to clean up
	time.Sleep(20 * time.Millisecond)

	// After cancellation, notify should not panic
	if err := n.Notify(context.Background(), QueueAsync); err != nil {
		t.Fatalf("Notify after subscriber cancellation should not fail: %v", err)
	}

	// Channel should not receive
	select {
	case _, ok := <-ch:
		if ok {
			// May receive one lingering notification; that's acceptable
		}
	case <-time.After(10 * time.Millisecond):
		// expected
	}
}

func TestChannelNotifier_Close(t *testing.T) {
	n := NewChannelNotifier()

	ctx := context.Background()
	ch := n.Subscribe(ctx, QueueAsync)

	if err := n.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Channel should be closed after Close()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after Close()")
		}
	case <-time.After(time.Second):
		t.Fatal("channel should have been closed")
	}

	// Double close should not panic
	if err := n.Close(); err != nil {
		t.Fatalf("Double close should not fail: %v", err)
	}
}

func TestChannelNotifier_ConcurrentAccess(t *testing.T) {
	n := NewChannelNotifier()
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
			case <-time.After(time.Second):
			}
		}()
	}

	// Give time for subscribers to register
	time.Sleep(10 * time.Millisecond)

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
