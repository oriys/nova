package cluster

import (
"time"
)

// NodeState represents the state of a node in the cluster
type NodeState string

const (
NodeStateActive   NodeState = "active"   // Node is healthy and accepting work
NodeStateInactive NodeState = "inactive" // Node is not responding
NodeStateDrained  NodeState = "drained"  // Node is being drained (no new work)
)

// Node represents a worker node in the cluster
type Node struct {
ID         string    `json:"id"`
Name       string    `json:"name"`
Address    string    `json:"address"` // HTTP address for routing requests
State      NodeState `json:"state"`
CPUCores   int       `json:"cpu_cores"`
MemoryMB   int       `json:"memory_mb"`
MaxVMs     int       `json:"max_vms"`      // Maximum concurrent VMs
ActiveVMs  int       `json:"active_vms"`   // Current number of active VMs
QueueDepth int       `json:"queue_depth"`  // Current queue depth
Version    string    `json:"version"`      // Nova version
Labels     map[string]string `json:"labels"` // Metadata labels
LastHeartbeat time.Time `json:"last_heartbeat"`
CreatedAt     time.Time `json:"created_at"`
UpdatedAt     time.Time `json:"updated_at"`

// Resource pressure metrics reported by Comet heartbeats.
CPUUsage       float64 `json:"cpu_usage"`        // 0-100
MemoryUsage    float64 `json:"memory_usage"`     // 0-100
IOPressure     float64 `json:"io_pressure"`      // 0-100
MemoryPressure float64 `json:"memory_pressure"`  // 0-100
}

// NodeMetrics contains runtime metrics for a node
type NodeMetrics struct {
NodeID         string  `json:"node_id"`
CPUUsage       float64 `json:"cpu_usage"`        // 0-100
MemoryUsage    float64 `json:"memory_usage"`     // 0-100
ActiveVMs      int     `json:"active_vms"`
QueueDepth     int     `json:"queue_depth"`
Invocations1m  int64   `json:"invocations_1m"`  // Invocations in last minute
AvgLatencyMs   int64   `json:"avg_latency_ms"`
ErrorRate      float64 `json:"error_rate"`       // 0-1
IOPressure     float64 `json:"io_pressure"`      // 0-100: IO wait percentage
MemoryPressure float64 `json:"memory_pressure"`  // 0-100: memory pressure (e.g. from /proc/pressure/memory)
Timestamp      time.Time `json:"timestamp"`
}

// NodeHealth contains health check results for a node
type NodeHealth struct {
NodeID      string    `json:"node_id"`
Healthy     bool      `json:"healthy"`
LastCheck   time.Time `json:"last_check"`
CheckCount  int       `json:"check_count"`
FailCount   int       `json:"fail_count"`
Message     string    `json:"message,omitempty"`
}

// IsHealthy checks if a node is considered healthy based on heartbeat
func (n *Node) IsHealthy(timeout time.Duration) bool {
if n.State != NodeStateActive {
return false
}
return time.Since(n.LastHeartbeat) < timeout
}

// AvailableCapacity returns the available VM capacity on this node
func (n *Node) AvailableCapacity() int {
if n.MaxVMs <= 0 {
return 0
}
return n.MaxVMs - n.ActiveVMs
}

// LoadFactor returns a value 0-1 representing how loaded the node is
func (n *Node) LoadFactor() float64 {
if n.MaxVMs <= 0 {
return 1.0
}
return float64(n.ActiveVMs) / float64(n.MaxVMs)
}

// ResourcePressureScore returns a composite pressure score (0-1) based on
// CPU, memory, and IO pressure. Higher values indicate a more stressed node.
// The scheduler uses this to avoid placing work on nodes experiencing resource
// pressure (e.g. near-OOM or IO-blocked).
func (n *Node) ResourcePressureScore() float64 {
	// Weighted composite: CPU 40%, Memory 35%, IO 25%
	score := (n.CPUUsage*0.4 + n.MemoryUsage*0.35 + n.IOPressure*0.25) / 100.0
	if score > 1.0 {
		return 1.0
	}
	if score < 0 {
		return 0
	}
	return score
}
