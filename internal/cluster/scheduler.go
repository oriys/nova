package cluster

import (
"fmt"
"math/rand"
)

// SchedulingStrategy defines how to select nodes for workload placement
type SchedulingStrategy string

const (
StrategyRoundRobin     SchedulingStrategy = "round-robin"
StrategyLeastLoaded    SchedulingStrategy = "least-loaded"
StrategyRandom         SchedulingStrategy = "random"
StrategyResourceAware  SchedulingStrategy = "resource-aware"
)

// Scheduler selects nodes for workload placement
type Scheduler struct {
registry *Registry
strategy SchedulingStrategy
rrIndex  int
}

// NewScheduler creates a new cluster scheduler
func NewScheduler(registry *Registry, strategy SchedulingStrategy) *Scheduler {
if strategy == "" {
strategy = StrategyLeastLoaded
}

return &Scheduler{
registry: registry,
strategy: strategy,
}
}

// SelectNode selects the best node for a new invocation
func (s *Scheduler) SelectNode() (*Node, error) {
nodes := s.registry.ListHealthyNodes()
if len(nodes) == 0 {
return nil, fmt.Errorf("no healthy nodes available")
}

switch s.strategy {
case StrategyRoundRobin:
return s.selectRoundRobin(nodes), nil
case StrategyLeastLoaded:
return s.selectLeastLoaded(nodes), nil
case StrategyRandom:
return s.selectRandom(nodes), nil
case StrategyResourceAware:
return s.selectResourceAware(nodes), nil
default:
return s.selectLeastLoaded(nodes), nil
}
}

func (s *Scheduler) selectRoundRobin(nodes []*Node) *Node {
if len(nodes) == 0 {
return nil
}

s.rrIndex = (s.rrIndex + 1) % len(nodes)
return nodes[s.rrIndex]
}

func (s *Scheduler) selectLeastLoaded(nodes []*Node) *Node {
if len(nodes) == 0 {
return nil
}

var selected *Node
lowestLoad := 2.0 // > 1.0

for _, node := range nodes {
load := node.LoadFactor()
if load < lowestLoad {
lowestLoad = load
selected = node
}
}

return selected
}

func (s *Scheduler) selectRandom(nodes []*Node) *Node {
if len(nodes) == 0 {
return nil
}

return nodes[rand.Intn(len(nodes))]
}

// selectResourceAware picks the node with the lowest composite resource
// pressure score. This avoids routing work to nodes that are experiencing
// high CPU, memory, or IO pressure (e.g. near-OOM or IO-blocked).
func (s *Scheduler) selectResourceAware(nodes []*Node) *Node {
if len(nodes) == 0 {
	return nil
}

var selected *Node
lowestScore := 2.0 // > 1.0

for _, node := range nodes {
	score := node.ResourcePressureScore()
	if score < lowestScore {
		lowestScore = score
		selected = node
	}
}

return selected
}

// SelectNodeForFunction selects a node for a specific function
// Can be extended to support function affinity, pinning, etc.
func (s *Scheduler) SelectNodeForFunction(functionID string) (*Node, error) {
// For now, use standard node selection
// Future: can add function->node affinity, locality, etc.
return s.SelectNode()
}
