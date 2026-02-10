package triggers

import (
"context"
"encoding/json"
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
