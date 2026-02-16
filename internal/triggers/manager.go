package triggers

import (
"context"
"fmt"
"sync"

"github.com/oriys/nova/internal/executor"
"github.com/oriys/nova/internal/logging"
"github.com/oriys/nova/internal/store"
)

// Manager coordinates all event triggers and their connectors
type Manager struct {
store      *store.Store
executor   *executor.Executor
connectors map[string]Connector
mu         sync.RWMutex
ctx        context.Context
cancel     context.CancelFunc
}

// NewManager creates a new trigger manager
func NewManager(s *store.Store, exec *executor.Executor) *Manager {
ctx, cancel := context.WithCancel(context.Background())
return &Manager{
store:      s,
executor:   exec,
connectors: make(map[string]Connector),
ctx:        ctx,
cancel:     cancel,
}
}

// RegisterTrigger registers and starts a new trigger
func (m *Manager) RegisterTrigger(trigger *Trigger) error {
m.mu.Lock()
defer m.mu.Unlock()

if _, exists := m.connectors[trigger.ID]; exists {
return fmt.Errorf("trigger %s already registered", trigger.ID)
}

var connector Connector
var err error

handler := &functionEventHandler{
executor:     m.executor,
functionName: trigger.FunctionName,
}

switch trigger.Type {
case TriggerTypeFilesystem:
connector, err = NewFilesystemConnector(trigger, handler)
case TriggerTypeKafka:
connector, err = NewKafkaConnector(trigger, handler)
case TriggerTypeRabbitMQ:
connector, err = NewRabbitMQConnector(trigger, handler)
case TriggerTypeRedis:
connector, err = NewRedisStreamConnector(trigger, handler)
default:
return fmt.Errorf("unsupported trigger type: %s", trigger.Type)
}

if err != nil {
return fmt.Errorf("create connector: %w", err)
}

if trigger.Enabled {
if err := connector.Start(m.ctx); err != nil {
return fmt.Errorf("start connector: %w", err)
}
}

m.connectors[trigger.ID] = connector
logging.Op().Info("trigger registered", "id", trigger.ID, "name", trigger.Name, "type", trigger.Type)
return nil
}

// UnregisterTrigger stops and removes a trigger
func (m *Manager) UnregisterTrigger(triggerID string) error {
m.mu.Lock()
defer m.mu.Unlock()

connector, exists := m.connectors[triggerID]
if !exists {
return fmt.Errorf("trigger %s not found", triggerID)
}

if err := connector.Stop(); err != nil {
logging.Op().Warn("failed to stop connector", "trigger", triggerID, "error", err)
}

delete(m.connectors, triggerID)
logging.Op().Info("trigger unregistered", "id", triggerID)
return nil
}

// Shutdown stops all connectors
func (m *Manager) Shutdown() error {
m.cancel()

m.mu.Lock()
defer m.mu.Unlock()

for id, connector := range m.connectors {
if err := connector.Stop(); err != nil {
logging.Op().Warn("failed to stop connector during shutdown", "trigger", id, "error", err)
}
}

m.connectors = make(map[string]Connector)
logging.Op().Info("trigger manager shutdown complete")
return nil
}

// TriggerStatus contains runtime status for a registered trigger.
type TriggerStatus struct {
	TriggerID string      `json:"trigger_id"`
	Type      TriggerType `json:"type"`
	Healthy   bool        `json:"healthy"`
}

// ListTriggerStatuses returns the runtime status of all registered triggers.
func (m *Manager) ListTriggerStatuses() []TriggerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]TriggerStatus, 0, len(m.connectors))
	for id, c := range m.connectors {
		statuses = append(statuses, TriggerStatus{
			TriggerID: id,
			Type:      c.Type(),
			Healthy:   c.IsHealthy(),
		})
	}
	return statuses
}

// GetTriggerStatus returns the runtime status of a single trigger.
func (m *Manager) GetTriggerStatus(triggerID string) (*TriggerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, exists := m.connectors[triggerID]
	if !exists {
		return nil, fmt.Errorf("trigger %s not found", triggerID)
	}
	return &TriggerStatus{
		TriggerID: triggerID,
		Type:      c.Type(),
		Healthy:   c.IsHealthy(),
	}, nil
}

// functionEventHandler implements EventHandler by invoking functions
type functionEventHandler struct {
executor     *executor.Executor
functionName string
}

func (h *functionEventHandler) Handle(ctx context.Context, event *TriggerEvent) error {
logging.Op().Info("handling trigger event", "trigger", event.TriggerID, "function", h.functionName)

_, err := h.executor.Invoke(ctx, h.functionName, event.Data)
if err != nil {
return fmt.Errorf("invoke function: %w", err)
}

return nil
}
