package circuitbreaker

import (
	"testing"
	"time"
)

func TestBreakerClosedAllowsRequests(t *testing.T) {
	b := New(Config{
		ErrorPct:       50,
		WindowDuration: 10 * time.Second,
		OpenDuration:   5 * time.Second,
		HalfOpenProbes: 2,
	})

	if !b.Allow() {
		t.Fatal("closed breaker should allow requests")
	}
	if b.State() != StateClosed {
		t.Fatalf("expected closed, got %v", b.State())
	}
}

func TestBreakerTripsOnHighErrorRate(t *testing.T) {
	b := New(Config{
		ErrorPct:       50,
		WindowDuration: 10 * time.Second,
		OpenDuration:   5 * time.Second,
		HalfOpenProbes: 1,
	})

	// Record enough failures to trip the breaker
	b.RecordSuccess()
	b.RecordFailure()
	b.RecordFailure()

	// Error rate is 66%, threshold is 50% -> should be open
	if b.State() != StateOpen {
		t.Fatalf("expected open after high error rate, got %v", b.State())
	}
	if b.Allow() {
		t.Fatal("open breaker should reject requests")
	}
}

func TestBreakerTransitionsToHalfOpen(t *testing.T) {
	b := New(Config{
		ErrorPct:       50,
		WindowDuration: 10 * time.Second,
		OpenDuration:   10 * time.Millisecond, // Very short for testing
		HalfOpenProbes: 1,
	})

	// Trip the breaker
	b.RecordFailure()
	b.RecordFailure()

	if b.State() != StateOpen {
		t.Fatalf("expected open, got %v", b.State())
	}

	// Wait for open duration to expire
	time.Sleep(20 * time.Millisecond)

	// Should transition to half-open and allow a probe
	if !b.Allow() {
		t.Fatal("should allow probe request in half-open state")
	}
}

func TestBreakerClosesAfterSuccessfulProbes(t *testing.T) {
	b := New(Config{
		ErrorPct:       50,
		WindowDuration: 10 * time.Second,
		OpenDuration:   10 * time.Millisecond,
		HalfOpenProbes: 1,
	})

	// Trip the breaker
	b.RecordFailure()
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	// Allow probe
	b.Allow()
	// Successful probe should close the breaker
	b.RecordSuccess()

	if b.State() != StateClosed {
		t.Fatalf("expected closed after successful probes, got %v", b.State())
	}
}

func TestBreakerReopensOnFailedProbe(t *testing.T) {
	b := New(Config{
		ErrorPct:       50,
		WindowDuration: 10 * time.Second,
		OpenDuration:   10 * time.Millisecond,
		HalfOpenProbes: 1,
	})

	// Trip the breaker
	b.RecordFailure()
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	// Allow probe
	b.Allow()
	// Failed probe should reopen
	b.RecordFailure()

	if b.State() != StateOpen {
		t.Fatalf("expected open after failed probe, got %v", b.State())
	}
}

func TestRegistryCreatesBreakerOnDemand(t *testing.T) {
	r := NewRegistry()

	cfg := Config{
		ErrorPct:       50,
		WindowDuration: 10 * time.Second,
		OpenDuration:   5 * time.Second,
		HalfOpenProbes: 1,
	}

	b1 := r.Get("func-1", cfg)
	if b1 == nil {
		t.Fatal("expected non-nil breaker")
	}

	b2 := r.Get("func-1", cfg)
	if b1 != b2 {
		t.Fatal("expected same breaker instance for same function")
	}
}

func TestRegistryReturnsNilForInvalidConfig(t *testing.T) {
	r := NewRegistry()

	b := r.Get("func-1", Config{})
	if b != nil {
		t.Fatal("expected nil breaker for zero config")
	}

	b = r.Get("func-1", Config{ErrorPct: 50})
	if b != nil {
		t.Fatal("expected nil breaker without window/open duration")
	}
}

func TestRegistrySnapshot(t *testing.T) {
	r := NewRegistry()

	cfg := Config{
		ErrorPct:       50,
		WindowDuration: 10 * time.Second,
		OpenDuration:   5 * time.Second,
		HalfOpenProbes: 1,
	}

	r.Get("func-1", cfg)
	r.Get("func-2", cfg)

	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(snap))
	}
	if snap["func-1"] != "closed" {
		t.Fatalf("expected closed, got %s", snap["func-1"])
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half_open"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
