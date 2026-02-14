package queue

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisListPrefix = "nova:queue:list:"

// RedisListNotifier is a distributed, Redis-backed notifier that uses
// LPUSH/BRPOP (push-pull pattern) instead of PUBLISH/SUBSCRIBE.
//
// Advantages over pure Pub/Sub:
//   - No message loss: Redis lists persist signals even when no consumer is listening.
//   - Natural load balancing: BRPOP delivers each signal to exactly one consumer,
//     distributing work evenly across multiple Nebula instances.
//   - Backpressure-friendly: unprocessed signals queue up in Redis rather than being dropped.
//
// Each subscriber goroutine blocks on BRPOP with a short timeout, which provides
// near-zero latency delivery while periodically allowing context cancellation checks.
type RedisListNotifier struct {
	client *redis.Client
	mu     sync.Mutex
	subs   map[QueueType][]*redisListSub
	closed bool
}

type redisListSub struct {
	ch     chan struct{}
	cancel context.CancelFunc
}

// NewRedisListNotifier creates a new Redis list-backed notifier.
func NewRedisListNotifier(client *redis.Client) *RedisListNotifier {
	return &RedisListNotifier{
		client: client,
		subs:   make(map[QueueType][]*redisListSub),
	}
}

// Notify pushes a signal to the Redis list for the given queue type.
// Exactly one subscriber will receive each signal via BRPOP.
func (n *RedisListNotifier) Notify(ctx context.Context, queue QueueType) error {
	key := redisListPrefix + string(queue)
	return n.client.LPush(ctx, key, "1").Err()
}

// Subscribe returns a channel that receives signals when new work is
// available on the given queue. A background goroutine uses BRPOP to
// block-wait for signals from the Redis list.
func (n *RedisListNotifier) Subscribe(ctx context.Context, queue QueueType) <-chan struct{} {
	ch := make(chan struct{}, 1)

	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		close(ch)
		return ch
	}

	subCtx, cancel := context.WithCancel(ctx)
	rs := &redisListSub{ch: ch, cancel: cancel}
	n.subs[queue] = append(n.subs[queue], rs)
	n.mu.Unlock()

	key := redisListPrefix + string(queue)

	go func() {
		defer func() {
			n.removeListSub(queue, rs)
			select {
			case <-ch:
			default:
			}
			close(ch)
		}()

		for {
			select {
			case <-subCtx.Done():
				return
			default:
			}

			// BRPOP with a 1-second timeout to allow periodic context checks.
			result, err := n.client.BRPop(subCtx, 1*time.Second, key).Result()
			if err != nil {
				if err == redis.Nil {
					// Timeout with no data — loop back and check context
					continue
				}
				if subCtx.Err() != nil {
					return
				}
				// Transient Redis error — back off briefly, then retry
				select {
				case <-subCtx.Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
				continue
			}

			if len(result) >= 2 {
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

// Close releases all resources held by the notifier, cancelling all
// background goroutines which will close their subscriber channels.
func (n *RedisListNotifier) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true
	for _, subs := range n.subs {
		for _, s := range subs {
			s.cancel()
		}
	}
	n.subs = nil
	return nil
}

func (n *RedisListNotifier) removeListSub(queue QueueType, target *redisListSub) {
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
