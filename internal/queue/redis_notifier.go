package queue

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
)

const redisChannelPrefix = "nova:queue:notify:"

// RedisNotifier is a distributed, Redis-backed notifier that uses
// PUBLISH/SUBSCRIBE to broadcast queue signals across multiple
// Nebula instances. This enables horizontal scaling: when a task is
// enqueued on one node, all nodes are notified immediately.
type RedisNotifier struct {
	client *redis.Client
	mu     sync.Mutex
	subs   map[QueueType][]*redisSub
	closed bool
}

type redisSub struct {
	ch     chan struct{}
	cancel context.CancelFunc
}

// NewRedisNotifier creates a new Redis-backed notifier.
func NewRedisNotifier(client *redis.Client) *RedisNotifier {
	return &RedisNotifier{
		client: client,
		subs:   make(map[QueueType][]*redisSub),
	}
}

// Notify publishes a signal to the Redis channel for the given queue type.
// All subscribed Nebula instances will receive this notification.
func (n *RedisNotifier) Notify(ctx context.Context, queue QueueType) error {
	channel := redisChannelPrefix + string(queue)
	return n.client.Publish(ctx, channel, "1").Err()
}

// Subscribe returns a channel that receives signals when new work is
// available on the given queue. A background goroutine listens on the
// Redis PubSub channel and forwards notifications to the returned channel.
func (n *RedisNotifier) Subscribe(ctx context.Context, queue QueueType) <-chan struct{} {
	ch := make(chan struct{}, 1)

	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		close(ch)
		return ch
	}

	subCtx, cancel := context.WithCancel(ctx)
	rs := &redisSub{ch: ch, cancel: cancel}
	n.subs[queue] = append(n.subs[queue], rs)
	n.mu.Unlock()

	channel := redisChannelPrefix + string(queue)
	pubsub := n.client.Subscribe(subCtx, channel)

	go func() {
		defer pubsub.Close()
		msgCh := pubsub.Channel()
		for {
			select {
			case <-subCtx.Done():
				n.removeSub(queue, rs)
				return
			case _, ok := <-msgCh:
				if !ok {
					return
				}
				select {
				case ch <- struct{}{}:
				default:
					// Non-blocking: subscriber already has a pending notification
				}
			}
		}
	}()

	return ch
}

// Close releases all resources held by the notifier, closing all
// subscriber channels and cancelling background goroutines.
func (n *RedisNotifier) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true
	for _, subs := range n.subs {
		for _, s := range subs {
			s.cancel()
			close(s.ch)
		}
	}
	n.subs = nil
	return nil
}

func (n *RedisNotifier) removeSub(queue QueueType, target *redisSub) {
	n.mu.Lock()
	defer n.mu.Unlock()
	subs := n.subs[queue]
	for i, s := range subs {
		if s == target {
			n.subs[queue] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}
