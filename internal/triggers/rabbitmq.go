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

// RabbitMQConnector consumes messages from a RabbitMQ queue and triggers functions.
type RabbitMQConnector struct {
	trigger  *Trigger
	handler  EventHandler
	url      string
	queue    string
	exchange string
	healthy  bool
	mu       sync.Mutex
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewRabbitMQConnector creates a new RabbitMQ connector from trigger configuration.
// Required config keys: "url" (AMQP connection string), "queue".
// Optional: "exchange" (default "").
func NewRabbitMQConnector(trigger *Trigger, handler EventHandler) (*RabbitMQConnector, error) {
	urlRaw, ok := trigger.Config["url"]
	if !ok {
		return nil, fmt.Errorf("rabbitmq connector requires 'url' config")
	}
	url, ok := urlRaw.(string)
	if !ok {
		return nil, fmt.Errorf("rabbitmq 'url' must be a string")
	}

	queueRaw, ok := trigger.Config["queue"]
	if !ok {
		return nil, fmt.Errorf("rabbitmq connector requires 'queue' config")
	}
	queueName, ok := queueRaw.(string)
	if !ok {
		return nil, fmt.Errorf("rabbitmq 'queue' must be a string")
	}

	exchange := ""
	if e, ok := trigger.Config["exchange"].(string); ok {
		exchange = e
	}

	return &RabbitMQConnector{
		trigger:  trigger,
		handler:  handler,
		url:      url,
		queue:    queueName,
		exchange: exchange,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}, nil
}

func (r *RabbitMQConnector) Start(ctx context.Context) error {
	r.mu.Lock()
	r.healthy = true
	r.mu.Unlock()

	go r.consumeLoop(ctx)
	logging.Op().Info("rabbitmq connector started", "trigger", r.trigger.ID, "queue", r.queue)
	return nil
}

func (r *RabbitMQConnector) consumeLoop(ctx context.Context) {
	defer close(r.doneCh)

	// Poll-based consumer placeholder — in production this would use
	// amqp091-go to consume from the queue. The connector structure
	// allows swapping in a real AMQP client by changing this loop body.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			// Placeholder: a real implementation would call channel.Consume()
		}
	}
}

// dispatchMessage converts a raw AMQP delivery into a TriggerEvent.
func (r *RabbitMQConnector) dispatchMessage(ctx context.Context, body []byte, routingKey string, deliveryTag uint64) error {
	event := &TriggerEvent{
		TriggerID: r.trigger.ID,
		EventID:   uuid.New().String(),
		Source:    fmt.Sprintf("rabbitmq://%s/%s", r.queue, routingKey),
		Type:      "rabbitmq.message",
		Data:      json.RawMessage(body),
		Metadata: map[string]interface{}{
			"queue":        r.queue,
			"exchange":     r.exchange,
			"routing_key":  routingKey,
			"delivery_tag": deliveryTag,
		},
		Timestamp: time.Now(),
	}
	return r.handler.Handle(ctx, event)
}

func (r *RabbitMQConnector) Stop() error {
	close(r.stopCh)
	<-r.doneCh
	r.mu.Lock()
	r.healthy = false
	r.mu.Unlock()
	logging.Op().Info("rabbitmq connector stopped", "trigger", r.trigger.ID)
	return nil
}

func (r *RabbitMQConnector) Type() TriggerType { return TriggerTypeRabbitMQ }

func (r *RabbitMQConnector) IsHealthy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.healthy
}
