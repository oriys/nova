package ebpf

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FlameGraphRequest parameters for generating a flame graph.
type FlameGraphRequest struct {
	FunctionID string        `json:"function_id"`
	Window     time.Duration `json:"window"`
	MinSamples int           `json:"min_samples"`
}

// FlameGraphNode represents a node in a flame graph.
type FlameGraphNode struct {
	Name     string            `json:"name"`
	Value    int64             `json:"value"`
	Children []*FlameGraphNode `json:"children,omitempty"`
}

// FlameGraphResult contains a generated flame graph.
type FlameGraphResult struct {
	FunctionID   string          `json:"function_id"`
	Root         *FlameGraphNode `json:"root"`
	TotalSamples int64          `json:"total_samples"`
	Window       time.Duration  `json:"window"`
	GeneratedAt  time.Time      `json:"generated_at"`
}

// FoldedStack is a semicolon-separated stack trace with count (Brendan Gregg format).
type FoldedStack struct {
	Stack string
	Count int64
}

// FlameGraphGenerator generates flame graphs from eBPF profile data.
type FlameGraphGenerator struct {
	aurora *AuroraIntegration
}

// NewFlameGraphGenerator creates a flame graph generator.
func NewFlameGraphGenerator(aurora *AuroraIntegration) *FlameGraphGenerator {
	return &FlameGraphGenerator{aurora: aurora}
}

// Generate creates a flame graph for the given function within the time window.
func (fg *FlameGraphGenerator) Generate(req *FlameGraphRequest) *FlameGraphResult {
	profiles := fg.collectProfiles(req)

	root := &FlameGraphNode{Name: "root"}
	var totalSamples int64

	for _, profile := range profiles {
		for _, sc := range profile.SyscallStats {
			child := fg.findOrCreateChild(root, profile.FunctionID)
			syscallNode := fg.findOrCreateChild(child, sc.Name)
			syscallNode.Value += sc.Count
			totalSamples += sc.Count
		}
	}

	fg.sortChildren(root)

	return &FlameGraphResult{
		FunctionID:   req.FunctionID,
		Root:         root,
		TotalSamples: totalSamples,
		Window:       req.Window,
		GeneratedAt:  time.Now(),
	}
}

// GenerateFolded produces folded-stack output compatible with flamegraph.pl.
func (fg *FlameGraphGenerator) GenerateFolded(req *FlameGraphRequest) string {
	profiles := fg.collectProfiles(req)

	stacks := make(map[string]int64)
	for _, profile := range profiles {
		for _, sc := range profile.SyscallStats {
			key := fmt.Sprintf("%s;%s", profile.FunctionID, sc.Name)
			stacks[key] += sc.Count
		}
	}

	var lines []FoldedStack
	for stack, count := range stacks {
		lines = append(lines, FoldedStack{Stack: stack, Count: count})
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Stack < lines[j].Stack
	})

	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b, "%s %d\n", line.Stack, line.Count)
	}
	return b.String()
}

func (fg *FlameGraphGenerator) collectProfiles(req *FlameGraphRequest) []*ProfileResult {
	cutoff := time.Now().Add(-req.Window)
	fg.aurora.mu.RLock()
	defer fg.aurora.mu.RUnlock()

	var profiles []*ProfileResult
	for _, p := range fg.aurora.profiles {
		if req.FunctionID != "" && p.FunctionID != req.FunctionID {
			continue
		}
		if p.CollectedAt.Before(cutoff) {
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles
}

func (fg *FlameGraphGenerator) findOrCreateChild(parent *FlameGraphNode, name string) *FlameGraphNode {
	for _, child := range parent.Children {
		if child.Name == name {
			return child
		}
	}
	child := &FlameGraphNode{Name: name}
	parent.Children = append(parent.Children, child)
	return child
}

func (fg *FlameGraphGenerator) sortChildren(node *FlameGraphNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Value > node.Children[j].Value
	})
	for _, child := range node.Children {
		fg.sortChildren(child)
	}
}
