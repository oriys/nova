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

	// Poll-based consumer simulation — in production this would use a real
	// Kafka client library (e.g. confluent-kafka-go or segmentio/kafka-go).
	// The connector structure is wired so that swapping in a real client
	// requires only changing this loop body.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-k.stopCh:
			return
		case <-ticker.C:
			// Placeholder: a real implementation would call consumer.ReadMessage()
			// and dispatch each message to the handler.
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
