// Package placement implements architecture-aware scheduling decisions
// for multi-arch Nova deployments.
package placement

import (
	"fmt"
	"sync"

	"github.com/oriys/nova/internal/domain"
)

// Node represents a compute node with its capabilities.
type Node struct {
	ID       string      `json:"id"`
	Addr     string      `json:"addr"`
	Arch     domain.Arch `json:"arch"`
	Backends []string    `json:"backends"` // Available backend types
	Capacity int         `json:"capacity"` // Max VMs
	Active   int         `json:"active"`   // Current active VMs
}

// Selector chooses the best node for a function based on architecture and capacity.
type Selector struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

// NewSelector creates a new placement selector.
func NewSelector() *Selector {
	return &Selector{
		nodes: make(map[string]*Node),
	}
}

// RegisterNode adds or updates a node in the selector.
func (s *Selector) RegisterNode(node *Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node
}

// RemoveNode removes a node from the selector.
func (s *Selector) RemoveNode(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, id)
}

// Select returns the best node for a function with the given architecture.
// It prefers nodes matching the requested arch, falls back to any node
// if emulation is allowed.
func (s *Selector) Select(arch domain.Arch, backend domain.BackendType, allowEmulation bool) (*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if arch == "" {
		arch = domain.ArchAMD64
	}

	var bestMatch *Node
	var bestFallback *Node

	for _, node := range s.nodes {
		remaining := node.Capacity - node.Active
		if remaining <= 0 {
			continue
		}

		// Check backend support
		if backend != "" && backend != domain.BackendAuto {
			found := false
			for _, b := range node.Backends {
				if b == string(backend) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if node.Arch == arch {
			if bestMatch == nil || (node.Capacity-node.Active) > (bestMatch.Capacity-bestMatch.Active) {
				bestMatch = node
			}
		} else if allowEmulation {
			if bestFallback == nil || (node.Capacity-node.Active) > (bestFallback.Capacity-bestFallback.Active) {
				bestFallback = node
			}
		}
	}

	if bestMatch != nil {
		return bestMatch, nil
	}
	if bestFallback != nil {
		return bestFallback, nil
	}
	return nil, fmt.Errorf("no available node for arch=%s backend=%s", arch, backend)
}

// ListNodes returns all registered nodes.
func (s *Selector) ListNodes() []*Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodes := make([]*Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

// FallbackStrategy defines how to handle missing architecture artifacts.
type FallbackStrategy int

const (
	StrategyReject    FallbackStrategy = iota // Reject if target arch unavailable
	StrategyEmulate                           // Use QEMU user-static emulation
	StrategyCrossNode                         // Route to a node with the right arch
)

// ResolveFallback determines the action when the requested arch artifact is missing.
func ResolveFallback(strategy FallbackStrategy, requestedArch, hostArch domain.Arch) (domain.Arch, bool, error) {
	if requestedArch == hostArch || requestedArch == "" {
		return hostArch, false, nil
	}

	switch strategy {
	case StrategyReject:
		return "", false, fmt.Errorf("artifact for arch %s not available on %s host", requestedArch, hostArch)
	case StrategyEmulate:
		return requestedArch, true, nil // emulated=true
	case StrategyCrossNode:
		return requestedArch, false, nil // Will be routed to correct node
	default:
		return "", false, fmt.Errorf("unknown fallback strategy: %d", strategy)
	}
}
