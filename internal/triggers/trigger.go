package triggers

import (
"context"
"encoding/json"
"fmt"
"sync"
"time"
)

// TriggerType defines the type of event trigger
type TriggerType string

const (
TriggerTypeFilesystem TriggerType = "filesystem" // S3/MinIO file changes
TriggerTypeKafka      TriggerType = "kafka"      // Kafka consumer
TriggerTypeRabbitMQ   TriggerType = "rabbitmq"   // RabbitMQ consumer
TriggerTypeRedis      TriggerType = "redis"      // Redis Stream consumer
TriggerTypeWebhook    TriggerType = "webhook"    // HTTP webhook receiver
)

// Trigger defines configuration for an event trigger
type Trigger struct {
ID           string                 `json:"id"`
TenantID     string                 `json:"tenant_id"`
Namespace    string                 `json:"namespace"`
Name         string                 `json:"name"`
Type         TriggerType            `json:"type"`
FunctionID   string                 `json:"function_id"`
FunctionName string                 `json:"function_name"`
Enabled      bool                   `json:"enabled"`
Config       map[string]interface{} `json:"config"` // Trigger-specific configuration
CreatedAt    time.Time              `json:"created_at"`
UpdatedAt    time.Time              `json:"updated_at"`
}

// TriggerEvent represents an event received by a trigger
type TriggerEvent struct {
TriggerID string                 `json:"trigger_id"`
EventID   string                 `json:"event_id"`
Source    string                 `json:"source"`
Type      string                 `json:"type"`
Data      json.RawMessage        `json:"data"`
Metadata  map[string]interface{} `json:"metadata"`
Timestamp time.Time              `json:"timestamp"`
}

// Connector defines the interface for event source connectors
type Connector interface {
// Start begins consuming events
Start(ctx context.Context) error

// Stop gracefully stops the connector
Stop() error

// Type returns the trigger type this connector handles
Type() TriggerType

// IsHealthy checks if the connector is operational
IsHealthy() bool
}

// EventHandler processes events from triggers
type EventHandler interface {
// Handle processes a trigger event
Handle(ctx context.Context, event *TriggerEvent) error
}

// BatchConfig configures batch processing for a trigger.
type BatchConfig struct {
	BatchSize    int `json:"batch_size"`     // Max records per batch (0 = no batching)
	WindowSeconds int `json:"window_seconds"` // Max wait time before flushing partial batch
}

// Batcher accumulates TriggerEvents and flushes them as a batch to the handler.
type Batcher struct {
	handler    EventHandler
	config     BatchConfig
	mu         sync.Mutex
	buffer     []*TriggerEvent
	timer      *time.Timer
	functionID string
}

// NewBatcher creates a batcher that wraps an EventHandler.
// If batchSize <= 1 or windowSeconds <= 0, it returns nil (no batching needed).
func NewBatcher(handler EventHandler, cfg BatchConfig, functionID string) *Batcher {
	if cfg.BatchSize <= 1 {
		return nil
	}
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = 5
	}
	return &Batcher{
		handler:    handler,
		config:     cfg,
		buffer:     make([]*TriggerEvent, 0, cfg.BatchSize),
		functionID: functionID,
	}
}

// Add adds an event to the batch buffer. When full or timer fires, it flushes.
func (b *Batcher) Add(ctx context.Context, event *TriggerEvent) error {
	b.mu.Lock()
	b.buffer = append(b.buffer, event)
	shouldFlush := len(b.buffer) >= b.config.BatchSize
	if len(b.buffer) == 1 {
		// Start window timer on first event in batch
		b.timer = time.AfterFunc(time.Duration(b.config.WindowSeconds)*time.Second, func() {
			b.Flush(ctx)
		})
	}
	b.mu.Unlock()

	if shouldFlush {
		return b.Flush(ctx)
	}
	return nil
}

// Flush sends all buffered events as a single batch event.
func (b *Batcher) Flush(ctx context.Context) error {
	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return nil
	}
	batch := b.buffer
	b.buffer = make([]*TriggerEvent, 0, b.config.BatchSize)
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()

	// Build batch payload
	records := make([]json.RawMessage, len(batch))
	for i, ev := range batch {
		records[i] = ev.Data
	}
	batchData, _ := json.Marshal(map[string]any{
		"records": records,
		"batch_metadata": map[string]any{
			"size":       len(batch),
			"max_size":   b.config.BatchSize,
			"window_s":   b.config.WindowSeconds,
			"trigger_id": batch[0].TriggerID,
			"source":     batch[0].Source,
		},
	})

	batchEvent := &TriggerEvent{
		TriggerID: batch[0].TriggerID,
		EventID:   fmt.Sprintf("batch-%s-%d", batch[0].TriggerID, time.Now().UnixMilli()),
		Source:    batch[0].Source,
		Type:      "batch",
		Data:      json.RawMessage(batchData),
		Timestamp: time.Now(),
	}
	return b.handler.Handle(ctx, batchEvent)
}
