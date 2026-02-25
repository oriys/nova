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
	"github.com/segmentio/kafka-go"
)

// KafkaConnector consumes messages from a Kafka topic and triggers functions.
type KafkaConnector struct {
	trigger  *Trigger
	handler  EventHandler
	brokers  []string
	topic    string
	group    string
	healthy  bool
	mu       sync.Mutex
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewKafkaConnector creates a new Kafka connector from trigger configuration.
// Required config keys: "brokers" (comma-separated), "topic", "group" (optional).
func NewKafkaConnector(trigger *Trigger, handler EventHandler) (*KafkaConnector, error) {
	brokersRaw, ok := trigger.Config["brokers"]
	if !ok {
		return nil, fmt.Errorf("kafka connector requires 'brokers' config")
	}
	brokers, ok := brokersRaw.(string)
	if !ok {
		return nil, fmt.Errorf("kafka 'brokers' must be a string")
	}

	topicRaw, ok := trigger.Config["topic"]
	if !ok {
		return nil, fmt.Errorf("kafka connector requires 'topic' config")
	}
	topic, ok := topicRaw.(string)
	if !ok {
		return nil, fmt.Errorf("kafka 'topic' must be a string")
	}

	group := trigger.Name + "-group"
	if g, ok := trigger.Config["group"].(string); ok && g != "" {
		group = g
	}

	return &KafkaConnector{
		trigger: trigger,
		handler: handler,
		brokers: strings.Split(brokers, ","),
		topic:   topic,
		group:   group,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}, nil
}

func (k *KafkaConnector) Start(ctx context.Context) error {
	k.mu.Lock()
	k.healthy = true
	k.mu.Unlock()

	go k.consumeLoop(ctx)
	logging.Op().Info("kafka connector started", "trigger", k.trigger.ID, "brokers", k.brokers, "topic", k.topic, "group", k.group)
	return nil
}

func (k *KafkaConnector) consumeLoop(ctx context.Context) {
	defer close(k.doneCh)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        k.brokers,
		Topic:          k.topic,
		GroupID:        k.group,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
		StartOffset:    kafka.LastOffset,
	})
	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-k.stopCh:
			return
		default:
		}

		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		msg, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Timeout is normal when there are no new messages
			if readCtx.Err() == context.DeadlineExceeded {
				continue
			}
			logging.Op().Warn("kafka: read error", "topic", k.topic, "error", err)
			k.mu.Lock()
			k.healthy = false
			k.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-k.stopCh:
				return
			case <-time.After(2 * time.Second):
			}
			k.mu.Lock()
			k.healthy = true
			k.mu.Unlock()
			continue
		}

		if err := k.dispatchMessage(ctx, msg.Key, msg.Value, msg.Offset, int32(msg.Partition)); err != nil {
			logging.Op().Warn("kafka: dispatch failed",
				"topic", k.topic, "offset", msg.Offset, "error", err)
		}
	}
}

// dispatchMessage converts a raw Kafka message into a TriggerEvent and
// hands it to the configured handler. Exported for testing and for real
// client integrations that call it from their own read loops.
func (k *KafkaConnector) dispatchMessage(ctx context.Context, key, value []byte, offset int64, partition int32) error {
	event := &TriggerEvent{
		TriggerID: k.trigger.ID,
		EventID:   uuid.New().String(),
		Source:    fmt.Sprintf("kafka://%s/%s", k.brokers[0], k.topic),
		Type:      "kafka.message",
		Data:      json.RawMessage(value),
		Metadata: map[string]interface{}{
			"key":       string(key),
			"offset":    offset,
			"partition": partition,
			"topic":     k.topic,
		},
		Timestamp: time.Now(),
	}
	return k.handler.Handle(ctx, event)
}

func (k *KafkaConnector) Stop() error {
	close(k.stopCh)
	<-k.doneCh
	k.mu.Lock()
	k.healthy = false
	k.mu.Unlock()
	logging.Op().Info("kafka connector stopped", "trigger", k.trigger.ID)
	return nil
}

func (k *KafkaConnector) Type() TriggerType { return TriggerTypeKafka }

func (k *KafkaConnector) IsHealthy() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.healthy
}
