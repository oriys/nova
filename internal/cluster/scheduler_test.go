package cluster

import (
	"context"
	"testing"
	"time"
)

func TestResourcePressureScore(t *testing.T) {
	tests := []struct {
		name     string
		cpu      float64
		memory   float64
		io       float64
		wantLow  float64
		wantHigh float64
	}{
		{
			name: "idle node",
			cpu:  0, memory: 0, io: 0,
			wantLow:  0.0,
			wantHigh: 0.01,
		},
		{
			name: "moderate load",
			cpu:  50, memory: 40, io: 20,
			wantLow:  0.3,
			wantHigh: 0.4,
		},
		{
			name: "high load",
			cpu:  90, memory: 85, io: 70,
			wantLow:  0.7,
			wantHigh: 0.9,
		},
		{
			name: "fully saturated",
			cpu:  100, memory: 100, io: 100,
			wantLow:  0.99,
			wantHigh: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{
				CPUUsage:    tt.cpu,
				MemoryUsage: tt.memory,
				IOPressure:  tt.io,
			}
			score := n.ResourcePressureScore()
			if score < tt.wantLow || score > tt.wantHigh {
				t.Errorf("ResourcePressureScore() = %f, want [%f, %f]", score, tt.wantLow, tt.wantHigh)
			}
		})
	}
}

func TestSelectResourceAware(t *testing.T) {
	reg := NewRegistry(nil, DefaultConfig("test"))
	s := NewScheduler(reg, StrategyResourceAware)

	// Register nodes with different pressure levels
	nodes := []*Node{
		{
			ID: "high-load", Name: "high-load", Address: "h:9090",
			State: NodeStateActive, MaxVMs: 10, ActiveVMs: 8,
			CPUUsage: 90, MemoryUsage: 85, IOPressure: 70,
			LastHeartbeat: time.Now(),
		},
		{
			ID: "low-load", Name: "low-load", Address: "l:9090",
			State: NodeStateActive, MaxVMs: 10, ActiveVMs: 2,
			CPUUsage: 10, MemoryUsage: 15, IOPressure: 5,
			LastHeartbeat: time.Now(),
		},
		{
			ID: "mid-load", Name: "mid-load", Address: "m:9090",
			State: NodeStateActive, MaxVMs: 10, ActiveVMs: 5,
			CPUUsage: 50, MemoryUsage: 40, IOPressure: 20,
			LastHeartbeat: time.Now(),
		},
	}

	for _, n := range nodes {
		reg.RegisterNode(context.Background(), n)
	}

	// Resource-aware should select the low-load node
	selected, err := s.SelectNode()
	if err != nil {
		t.Fatalf("SelectNode failed: %v", err)
	}
	if selected.ID != "low-load" {
		t.Errorf("expected 'low-load' node, got '%s'", selected.ID)
	}
}

func TestSelectResourceAware_NoNodes(t *testing.T) {
	reg := NewRegistry(nil, DefaultConfig("test"))
	s := NewScheduler(reg, StrategyResourceAware)

	_, err := s.SelectNode()
	if err == nil {
		t.Fatal("expected error when no nodes available")
	}
}

func TestSelectNodeForFunction_UsesAffinity(t *testing.T) {
	reg := NewRegistry(nil, DefaultConfig("node-a"))
	s := NewScheduler(reg, StrategyLocalityAware)

	nodes := []*Node{
		{
			ID: "node-a", Name: "node-a", Address: "a:9090",
			State: NodeStateActive, MaxVMs: 10, ActiveVMs: 4,
			LastHeartbeat: time.Now(),
		},
		{
			ID: "node-b", Name: "node-b", Address: "b:9090",
			State: NodeStateActive, MaxVMs: 10, ActiveVMs: 6,
			LastHeartbeat: time.Now(),
		},
	}
	for _, n := range nodes {
		if err := reg.RegisterNode(context.Background(), n); err != nil {
			t.Fatalf("register node: %v", err)
		}
	}

	s.RecordFunctionPlacement("fn-1", "node-b")

	selected, err := s.SelectNodeForFunction("fn-1")
	if err != nil {
		t.Fatalf("SelectNodeForFunction failed: %v", err)
	}
	if selected.ID != "node-b" {
		t.Fatalf("expected affinity node-b, got %s", selected.ID)
	}
}

func TestLocalityAwarePrefersWarmNode(t *testing.T) {
	reg := NewRegistry(nil, DefaultConfig("node-a"))
	s := NewScheduler(reg, StrategyLocalityAware)

	coldNode := &Node{
		ID: "cold", Name: "cold", Address: "cold:9090",
		State: NodeStateActive, MaxVMs: 10, ActiveVMs: 1,
		LastHeartbeat: time.Now(),
	}
	warmNode := &Node{
		ID: "warm", Name: "warm", Address: "warm:9090",
		State: NodeStateActive, MaxVMs: 10, ActiveVMs: 2,
		Labels: map[string]string{
			"warm/fn-2": "true",
		},
		LastHeartbeat: time.Now(),
	}
	if err := reg.RegisterNode(context.Background(), coldNode); err != nil {
		t.Fatalf("register cold node: %v", err)
	}
	if err := reg.RegisterNode(context.Background(), warmNode); err != nil {
		t.Fatalf("register warm node: %v", err)
	}

	selected, err := s.SelectNodeForFunction("fn-2")
	if err != nil {
		t.Fatalf("SelectNodeForFunction failed: %v", err)
	}
	if selected.ID != "warm" {
		t.Fatalf("expected warm node, got %s", selected.ID)
	}
}
