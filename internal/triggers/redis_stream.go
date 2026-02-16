package triggers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/logging"
)

// RedisStreamConnector consumes messages from a Redis Stream and triggers functions.
type RedisStreamConnector struct {
	trigger  *Trigger
	handler  EventHandler
	addr     string
	stream   string
	group    string
	consumer string
	healthy  bool
	mu       sync.Mutex
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewRedisStreamConnector creates a new Redis Stream connector from trigger configuration.
// Required config keys: "addr" (Redis address), "stream" (stream key).
// Optional: "group" (consumer group name), "consumer" (consumer name).
func NewRedisStreamConnector(trigger *Trigger, handler EventHandler) (*RedisStreamConnector, error) {
	addrRaw, ok := trigger.Config["addr"]
	if !ok {
		return nil, fmt.Errorf("redis stream connector requires 'addr' config")
	}
	addr, ok := addrRaw.(string)
	if !ok {
		return nil, fmt.Errorf("redis 'addr' must be a string")
	}

	streamRaw, ok := trigger.Config["stream"]
	if !ok {
		return nil, fmt.Errorf("redis stream connector requires 'stream' config")
	}
	stream, ok := streamRaw.(string)
	if !ok {
		return nil, fmt.Errorf("redis 'stream' must be a string")
	}

	group := trigger.Name + "-group"
	if g, ok := trigger.Config["group"].(string); ok && g != "" {
		group = g
	}

	consumer := trigger.ID
	if c, ok := trigger.Config["consumer"].(string); ok && c != "" {
		consumer = c
	}

	return &RedisStreamConnector{
		trigger:  trigger,
		handler:  handler,
		addr:     addr,
		stream:   stream,
		group:    group,
		consumer: consumer,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}, nil
}

func (rs *RedisStreamConnector) Start(ctx context.Context) error {
	rs.mu.Lock()
	rs.healthy = true
	rs.mu.Unlock()

	go rs.consumeLoop(ctx)
	logging.Op().Info("redis stream connector started", "trigger", rs.trigger.ID, "stream", rs.stream, "group", rs.group)
	return nil
}

func (rs *RedisStreamConnector) consumeLoop(ctx context.Context) {
	defer close(rs.doneCh)

	// Poll-based consumer placeholder — in production this would use
	// go-redis XREADGROUP. The connector structure allows swapping in
	// a real Redis client by changing this loop body.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rs.stopCh:
			return
		case <-ticker.C:
			// Placeholder: a real implementation would call
			// client.XReadGroup() and dispatch each message.
		}
	}
}

// dispatchMessage converts a raw Redis Stream message into a TriggerEvent.
func (rs *RedisStreamConnector) dispatchMessage(ctx context.Context, messageID string, values map[string]interface{}) error {
	data, _ := json.Marshal(values)
	event := &TriggerEvent{
		TriggerID: rs.trigger.ID,
		EventID:   uuid.New().String(),
		Source:    fmt.Sprintf("redis://%s/%s", rs.addr, rs.stream),
		Type:      "redis.stream.message",
		Data:      json.RawMessage(data),
		Metadata: map[string]interface{}{
			"stream":     rs.stream,
			"group":      rs.group,
			"consumer":   rs.consumer,
			"message_id": messageID,
		},
		Timestamp: time.Now(),
	}
	return rs.handler.Handle(ctx, event)
}

func (rs *RedisStreamConnector) Stop() error {
	close(rs.stopCh)
	<-rs.doneCh
	rs.mu.Lock()
	rs.healthy = false
	rs.mu.Unlock()
	logging.Op().Info("redis stream connector stopped", "trigger", rs.trigger.ID)
	return nil
}

func (rs *RedisStreamConnector) Type() TriggerType { return TriggerTypeRedis }

func (rs *RedisStreamConnector) IsHealthy() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.healthy
}
