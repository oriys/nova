package cluster

import (
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
			name:     "idle node",
			cpu:      0, memory: 0, io: 0,
			wantLow:  0.0,
			wantHigh: 0.01,
		},
		{
			name:     "moderate load",
			cpu:      50, memory: 40, io: 20,
			wantLow:  0.3,
			wantHigh: 0.4,
		},
		{
			name:     "high load",
			cpu:      90, memory: 85, io: 70,
			wantLow:  0.7,
			wantHigh: 0.9,
		},
		{
			name:     "fully saturated",
			cpu:      100, memory: 100, io: 100,
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
		reg.RegisterNode(nil, n)
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
