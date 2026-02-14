// Package queue provides a push-based notification layer for async queue processing.
// Instead of relying solely on database polling (which causes contention under
// high concurrency), workers can subscribe to notifications and wake up immediately
// when new tasks are enqueued.
//
// Implementations:
//   - NoopNotifier: a no-op notifier that never sends notifications; workers rely purely on polling
//   - ChannelNotifier: in-process channel-based notifier suitable for single-instance deployments
//
// When a task is enqueued, the producer calls Notify(). Subscribed workers receive
// the signal via their subscription channel and immediately poll the database,
// reducing latency from PollInterval (e.g. 500ms) to near-zero.
package queue

import (
	"context"
	"sync"
)

// QueueType identifies a named queue for notification routing.
type QueueType string

const (
	QueueAsync       QueueType = "async"
	QueueEvent       QueueType = "event"
	QueueOutbox      QueueType = "outbox"
)

// Notifier provides push-based notifications for queue consumers.
// It complements (not replaces) the database-backed queue, reducing
// the reliance on polling and lowering end-to-end latency.
type Notifier interface {
	// Notify signals that new work is available on the given queue.
	Notify(ctx context.Context, queue QueueType) error

	// Subscribe returns a channel that receives signals when new work
	// is available on the given queue. The channel is closed when the
	// context is cancelled or Close is called.
	Subscribe(ctx context.Context, queue QueueType) <-chan struct{}

	// Close releases all resources held by the notifier.
	Close() error
}

// NoopNotifier is a no-op implementation that never sends notifications.
// Workers fall back to pure polling when this notifier is used.
type NoopNotifier struct{}

func NewNoopNotifier() *NoopNotifier { return &NoopNotifier{} }

func (n *NoopNotifier) Notify(_ context.Context, _ QueueType) error { return nil }

func (n *NoopNotifier) Subscribe(ctx context.Context, _ QueueType) <-chan struct{} {
	// Return a channel that is never written to; workers rely on ticker.
	// The channel is closed when the context is cancelled to prevent goroutine leaks.
	ch := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}

func (n *NoopNotifier) Close() error { return nil }

// ChannelNotifier is an in-process, channel-based notifier suitable for
// single-instance deployments. It provides near-zero latency notification
// without requiring external infrastructure like Redis.
type ChannelNotifier struct {
	mu          sync.Mutex
	subscribers map[QueueType][]chan struct{}
	closed      bool
}

func NewChannelNotifier() *ChannelNotifier {
	return &ChannelNotifier{
		subscribers: make(map[QueueType][]chan struct{}),
	}
}

func (n *ChannelNotifier) Notify(_ context.Context, queue QueueType) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	for _, ch := range n.subscribers[queue] {
		select {
		case ch <- struct{}{}:
		default:
			// Non-blocking: subscriber already has a pending notification
		}
	}
	return nil
}

func (n *ChannelNotifier) Subscribe(ctx context.Context, queue QueueType) <-chan struct{} {
	ch := make(chan struct{}, 1)

	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		close(ch)
		return ch
	}
	n.subscribers[queue] = append(n.subscribers[queue], ch)
	n.mu.Unlock()

	go func() {
		<-ctx.Done()
		n.mu.Lock()
		defer n.mu.Unlock()
		subs := n.subscribers[queue]
		for i, s := range subs {
			if s == ch {
				n.subscribers[queue] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}()

	return ch
}

func (n *ChannelNotifier) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true
	for _, subs := range n.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	n.subscribers = nil
	return nil
}
