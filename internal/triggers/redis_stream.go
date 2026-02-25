package triggers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/logging"
	"github.com/redis/go-redis/v9"
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

	client := redis.NewClient(&redis.Options{Addr: rs.addr})
	defer client.Close()

	// Create consumer group if it doesn't exist (MKSTREAM creates the stream too)
	err := client.XGroupCreateMkStream(ctx, rs.stream, rs.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		logging.Op().Warn("redis stream: failed to create consumer group",
			"stream", rs.stream, "group", rs.group, "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-rs.stopCh:
			return
		default:
		}

		streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    rs.group,
			Consumer: rs.consumer,
			Streams:  []string{rs.stream, ">"},
			Count:    10,
			Block:    time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil || ctx.Err() != nil {
				continue
			}
			logging.Op().Warn("redis stream: read error", "stream", rs.stream, "error", err)
			rs.mu.Lock()
			rs.healthy = false
			rs.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-rs.stopCh:
				return
			case <-time.After(2 * time.Second):
			}
			rs.mu.Lock()
			rs.healthy = true
			rs.mu.Unlock()
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				values := make(map[string]interface{}, len(msg.Values))
				for k, v := range msg.Values {
					values[k] = v
				}
				if err := rs.dispatchMessage(ctx, msg.ID, values); err != nil {
					logging.Op().Warn("redis stream: dispatch failed",
						"stream", rs.stream, "message_id", msg.ID, "error", err)
					continue
				}
				client.XAck(ctx, rs.stream, rs.group, msg.ID)
			}
		}
	}
}

// dispatchMessage converts a raw Redis Stream message into a TriggerEvent.
func (rs *RedisStreamConnector) dispatchMessage(ctx context.Context, messageID string, values map[string]interface{}) error {
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal redis stream message: %w", err)
	}
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
