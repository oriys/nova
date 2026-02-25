package triggers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/logging"
	amqp "github.com/rabbitmq/amqp091-go"
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

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		default:
		}

		if err := r.consumeOnce(ctx); err != nil {
			logging.Op().Warn("rabbitmq: connection error, reconnecting",
				"queue", r.queue, "error", err)
			r.mu.Lock()
			r.healthy = false
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-r.stopCh:
				return
			case <-time.After(3 * time.Second):
			}
			r.mu.Lock()
			r.healthy = true
			r.mu.Unlock()
		}
	}
}

// consumeOnce establishes one AMQP connection and consumes until error or stop.
func (r *RabbitMQConnector) consumeOnce(ctx context.Context) error {
	conn, err := amqp.Dial(r.url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("channel: %w", err)
	}
	defer ch.Close()

	// Ensure queue exists
	_, err = ch.QueueDeclare(r.queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("queue declare: %w", err)
	}

	if err := ch.Qos(10, 0, false); err != nil {
		return fmt.Errorf("qos: %w", err)
	}

	deliveries, err := ch.Consume(r.queue, r.trigger.ID, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.stopCh:
			return nil
		case amqpErr := <-connClosed:
			if amqpErr != nil {
				return fmt.Errorf("connection closed: %s", amqpErr.Reason)
			}
			return fmt.Errorf("connection closed")
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			if err := r.dispatchMessage(ctx, d.Body, d.RoutingKey, d.DeliveryTag); err != nil {
				logging.Op().Warn("rabbitmq: dispatch failed",
					"queue", r.queue, "delivery_tag", d.DeliveryTag, "error", err)
				_ = d.Nack(false, true) // requeue on failure
				continue
			}
			_ = d.Ack(false)
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
