package cluster

import (
"context"
"fmt"
"sync"
"time"

"github.com/oriys/nova/internal/logging"
"github.com/oriys/nova/internal/store"
)

// Registry manages the cluster node registry
type Registry struct {
store            *store.Store
localNodeID      string
nodes            map[string]*Node
mu               sync.RWMutex
heartbeatTicker  *time.Ticker
healthCheckInterval time.Duration
heartbeatTimeout time.Duration
stopCh           chan struct{}
}

// Config holds cluster registry configuration
type Config struct {
NodeID              string
HeartbeatInterval   time.Duration
HealthCheckInterval time.Duration
HeartbeatTimeout    time.Duration
}

// DefaultConfig returns default cluster configuration
func DefaultConfig(nodeID string) *Config {
return &Config{
NodeID:              nodeID,
HeartbeatInterval:   10 * time.Second,
HealthCheckInterval: 30 * time.Second,
HeartbeatTimeout:    60 * time.Second,
}
}

// NewRegistry creates a new node registry
func NewRegistry(s *store.Store, cfg *Config) *Registry {
if cfg == nil {
cfg = DefaultConfig("node-local")
}

return &Registry{
store:               s,
localNodeID:         cfg.NodeID,
nodes:               make(map[string]*Node),
healthCheckInterval: cfg.HealthCheckInterval,
heartbeatTimeout:    cfg.HeartbeatTimeout,
stopCh:              make(chan struct{}),
}
}

// RegisterNode registers a node in the cluster
func (r *Registry) RegisterNode(ctx context.Context, node *Node) error {
r.mu.Lock()
defer r.mu.Unlock()

node.UpdatedAt = time.Now()
node.LastHeartbeat = time.Now()

// Store in database (implement this in store layer)
// For now, just store in memory
r.nodes[node.ID] = node

logging.Op().Info("node registered", "id", node.ID, "name", node.Name, "address", node.Address)
return nil
}

// UpdateHeartbeat updates the heartbeat timestamp for a node
func (r *Registry) UpdateHeartbeat(ctx context.Context, nodeID string, metrics *NodeMetrics) error {
r.mu.Lock()
defer r.mu.Unlock()

node, exists := r.nodes[nodeID]
if !exists {
return fmt.Errorf("node %s not found", nodeID)
}

node.LastHeartbeat = time.Now()
if metrics != nil {
node.ActiveVMs = metrics.ActiveVMs
node.QueueDepth = metrics.QueueDepth
node.CPUUsage = metrics.CPUUsage
node.MemoryUsage = metrics.MemoryUsage
node.IOPressure = metrics.IOPressure
node.MemoryPressure = metrics.MemoryPressure
}

return nil
}

// GetNode retrieves a node by ID
func (r *Registry) GetNode(nodeID string) (*Node, error) {
r.mu.RLock()
defer r.mu.RUnlock()

node, exists := r.nodes[nodeID]
if !exists {
return nil, fmt.Errorf("node %s not found", nodeID)
}

return node, nil
}

// ListNodes returns all registered nodes
func (r *Registry) ListNodes() []*Node {
r.mu.RLock()
defer r.mu.RUnlock()

nodes := make([]*Node, 0, len(r.nodes))
for _, node := range r.nodes {
nodes = append(nodes, node)
}

return nodes
}

// ListHealthyNodes returns all healthy nodes
func (r *Registry) ListHealthyNodes() []*Node {
r.mu.RLock()
defer r.mu.RUnlock()

nodes := make([]*Node, 0)
for _, node := range r.nodes {
if node.IsHealthy(r.heartbeatTimeout) {
nodes = append(nodes, node)
}
}

return nodes
}

// RemoveNode removes a node from the cluster
func (r *Registry) RemoveNode(ctx context.Context, nodeID string) error {
r.mu.Lock()
defer r.mu.Unlock()

delete(r.nodes, nodeID)
logging.Op().Info("node removed", "id", nodeID)
return nil
}

// StartHealthChecker starts the background health checker
func (r *Registry) StartHealthChecker(ctx context.Context) {
ticker := time.NewTicker(r.healthCheckInterval)
defer ticker.Stop()

for {
select {
case <-ctx.Done():
return
case <-r.stopCh:
return
case <-ticker.C:
r.checkNodeHealth()
}
}
}

func (r *Registry) checkNodeHealth() {
r.mu.Lock()
defer r.mu.Unlock()

for id, node := range r.nodes {
if !node.IsHealthy(r.heartbeatTimeout) && node.State == NodeStateActive {
logging.Op().Warn("node became unhealthy", "id", id, "last_heartbeat", node.LastHeartbeat)
node.State = NodeStateInactive
}
}
}

// Stop stops the registry
func (r *Registry) Stop() {
close(r.stopCh)
}
