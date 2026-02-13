package workflow

import (
	"fmt"

	"github.com/oriys/nova/internal/domain"
)

// ValidateDAG checks that the workflow definition forms a valid DAG:
// - At least one node
// - All edge references point to valid node keys
// - No cycles (Kahn's algorithm)
// Returns topological order of node keys.
func ValidateDAG(def *domain.WorkflowDefinition) ([]string, error) {
	if len(def.Nodes) == 0 {
		return nil, fmt.Errorf("workflow must have at least one node")
	}

	// Build node key set
	nodeSet := make(map[string]bool, len(def.Nodes))
	for _, n := range def.Nodes {
		if n.NodeKey == "" {
			return nil, fmt.Errorf("node_key cannot be empty")
		}

		// Determine effective node type (default to "function")
		nodeType := n.NodeType
		if nodeType == "" {
			nodeType = domain.NodeTypeFunction
		}

		switch nodeType {
		case domain.NodeTypeFunction:
			if n.FunctionName == "" {
				return nil, fmt.Errorf("node %q: function_name is required for function nodes", n.NodeKey)
			}
		case domain.NodeTypeSubWorkflow:
			if n.WorkflowName == "" {
				return nil, fmt.Errorf("node %q: workflow_name is required for sub_workflow nodes", n.NodeKey)
			}
		default:
			return nil, fmt.Errorf("node %q: unknown node_type %q", n.NodeKey, nodeType)
		}

		if nodeSet[n.NodeKey] {
			return nil, fmt.Errorf("duplicate node_key: %q", n.NodeKey)
		}
		nodeSet[n.NodeKey] = true
	}

	// Validate edges reference existing nodes
	for _, e := range def.Edges {
		if !nodeSet[e.From] {
			return nil, fmt.Errorf("edge from unknown node: %q", e.From)
		}
		if !nodeSet[e.To] {
			return nil, fmt.Errorf("edge to unknown node: %q", e.To)
		}
		if e.From == e.To {
			return nil, fmt.Errorf("self-loop on node %q", e.From)
		}
	}

	// Kahn's algorithm for topological sort + cycle detection
	inDegree := make(map[string]int, len(def.Nodes))
	successors := make(map[string][]string)
	for _, n := range def.Nodes {
		inDegree[n.NodeKey] = 0
	}
	for _, e := range def.Edges {
		inDegree[e.To]++
		successors[e.From] = append(successors[e.From], e.To)
	}

	// Seed queue with zero-in-degree nodes
	var queue []string
	for _, n := range def.Nodes {
		if inDegree[n.NodeKey] == 0 {
			queue = append(queue, n.NodeKey)
		}
	}

	var order []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, succ := range successors[curr] {
			inDegree[succ]--
			if inDegree[succ] == 0 {
				queue = append(queue, succ)
			}
		}
	}

	if len(order) != len(def.Nodes) {
		return nil, fmt.Errorf("workflow contains a cycle")
	}

	return order, nil
}

// BuildDependencyMap returns a map of node_key -> list of predecessor node_keys.
func BuildDependencyMap(def *domain.WorkflowDefinition) map[string][]string {
	deps := make(map[string][]string)
	for _, e := range def.Edges {
		deps[e.To] = append(deps[e.To], e.From)
	}
	return deps
}

// BuildSuccessorMap returns a map of node_key -> list of successor node_keys.
func BuildSuccessorMap(def *domain.WorkflowDefinition) map[string][]string {
	succs := make(map[string][]string)
	for _, e := range def.Edges {
		succs[e.From] = append(succs[e.From], e.To)
	}
	return succs
}
