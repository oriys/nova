package cluster

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// SchedulingStrategy defines how to select nodes for workload placement
type SchedulingStrategy string

const (
	StrategyRoundRobin    SchedulingStrategy = "round-robin"
	StrategyLeastLoaded   SchedulingStrategy = "least-loaded"
	StrategyRandom        SchedulingStrategy = "random"
	StrategyResourceAware SchedulingStrategy = "resource-aware"
	StrategyLocalityAware SchedulingStrategy = "locality-aware"
)

// Scheduler selects nodes for workload placement
type Scheduler struct {
	registry *Registry
	strategy SchedulingStrategy

	mu          sync.Mutex // protects rrIndex
	rrIndex     int
	affinityTTL time.Duration
	affinity    sync.Map // functionID -> affinityEntry
}

// NewScheduler creates a new cluster scheduler
func NewScheduler(registry *Registry, strategy SchedulingStrategy) *Scheduler {
	if strategy == "" {
		strategy = StrategyLeastLoaded
	}

	return &Scheduler{
		registry:    registry,
		strategy:    strategy,
		affinityTTL: 5 * time.Minute,
	}
}

// SelectNode selects the best node for a new invocation
func (s *Scheduler) SelectNode() (*Node, error) {
	return s.selectNode("", false)
}

func (s *Scheduler) selectRoundRobin(nodes []*Node) *Node {
	if len(nodes) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	index := s.rrIndex % len(nodes)
	s.rrIndex++
	return nodes[index]
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
func (s *Scheduler) SelectNodeForFunction(functionID string) (*Node, error) {
	return s.selectNode(functionID, true)
}

// RecordFunctionPlacement updates short-term function-to-node affinity after a
// successful invocation routing decision.
func (s *Scheduler) RecordFunctionPlacement(functionID, nodeID string) {
	if functionID == "" || nodeID == "" {
		return
	}
	s.affinity.Store(functionID, affinityEntry{
		nodeID:    nodeID,
		expiresAt: time.Now().Add(s.affinityTTL),
	})
}

func (s *Scheduler) selectNode(functionID string, preferLocality bool) (*Node, error) {
	nodes := s.registry.ListHealthyNodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no healthy nodes available")
	}

	if preferLocality {
		if preferred := s.selectAffinity(nodes, functionID); preferred != nil {
			return preferred, nil
		}
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
	case StrategyLocalityAware:
		return s.selectLocalityAware(nodes, functionID), nil
	default:
		return s.selectLeastLoaded(nodes), nil
	}
}

func (s *Scheduler) selectAffinity(nodes []*Node, functionID string) *Node {
	if functionID == "" {
		return nil
	}

	if raw, ok := s.affinity.Load(functionID); ok {
		entry := raw.(affinityEntry)
		if time.Now().After(entry.expiresAt) {
			s.affinity.Delete(functionID)
		} else if node := findNodeByID(nodes, entry.nodeID); node != nil && node.AvailableCapacity() > 0 {
			return node
		}
	}

	bestWarm := s.selectWarmNode(nodes, functionID)
	if bestWarm != nil {
		return bestWarm
	}

	return nil
}

func (s *Scheduler) selectWarmNode(nodes []*Node, functionID string) *Node {
	if functionID == "" {
		return nil
	}
	var selected *Node
	bestScore := -1.0
	for _, node := range nodes {
		if !node.HasWarmFunction(functionID) {
			continue
		}
		score := localityScore(node, true)
		if score > bestScore {
			bestScore = score
			selected = node
		}
	}
	return selected
}

func (s *Scheduler) selectLocalityAware(nodes []*Node, functionID string) *Node {
	var selected *Node
	bestScore := -1.0

	for _, node := range nodes {
		score := localityScore(node, node.HasWarmFunction(functionID))
		if score > bestScore {
			bestScore = score
			selected = node
		}
	}

	return selected
}

type affinityEntry struct {
	nodeID    string
	expiresAt time.Time
}

func findNodeByID(nodes []*Node, nodeID string) *Node {
	for _, node := range nodes {
		if node != nil && node.ID == nodeID {
			return node
		}
	}
	return nil
}

func localityScore(node *Node, warmBoost bool) float64 {
	if node == nil {
		return -1
	}

	maxVMs := node.MaxVMs
	if maxVMs <= 0 {
		maxVMs = 1
	}

	capacityScore := float64(maxInt(node.AvailableCapacity(), 0)) / float64(maxVMs)
	loadScore := 1 - clamp01(node.LoadFactor())
	pressureScore := 1 - clamp01(node.ResourcePressureScore())
	queuePenalty := clamp01(float64(maxInt(node.QueueDepth, 0)) / float64(maxVMs))
	queueScore := 1 - queuePenalty

	score := capacityScore*0.4 + loadScore*0.25 + pressureScore*0.2 + queueScore*0.15
	if warmBoost {
		score += 0.1
	}
	return score
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
