package asyncqueue

import (
	"testing"
	"time"
)

func TestMergeAdaptiveConfig_Defaults(t *testing.T) {
	cfg := mergeAdaptiveConfig(AdaptiveConfig{})
	if cfg.ProbeInterval != 2*time.Second {
		t.Errorf("expected ProbeInterval=2s, got %v", cfg.ProbeInterval)
	}
	if cfg.MinWorkers != 4 {
		t.Errorf("expected MinWorkers=4, got %d", cfg.MinWorkers)
	}
	if cfg.MaxWorkers != 256 {
		t.Errorf("expected MaxWorkers=256, got %d", cfg.MaxWorkers)
	}
	if cfg.MinBatchSize != 4 {
		t.Errorf("expected MinBatchSize=4, got %d", cfg.MinBatchSize)
	}
	if cfg.MaxBatchSize != 32 {
		t.Errorf("expected MaxBatchSize=32, got %d", cfg.MaxBatchSize)
	}
	if cfg.MinPollInterval != 20*time.Millisecond {
		t.Errorf("expected MinPollInterval=20ms, got %v", cfg.MinPollInterval)
	}
	if cfg.MaxPollInterval != 500*time.Millisecond {
		t.Errorf("expected MaxPollInterval=500ms, got %v", cfg.MaxPollInterval)
	}
	if cfg.ScaleUpStep != 4 {
		t.Errorf("expected ScaleUpStep=4, got %d", cfg.ScaleUpStep)
	}
	if cfg.ScaleDownRate != 0.75 {
		t.Errorf("expected ScaleDownRate=0.75, got %f", cfg.ScaleDownRate)
	}
	if cfg.StableRoundsBeforeScaleDown != 3 {
		t.Errorf("expected StableRoundsBeforeScaleDown=3, got %d", cfg.StableRoundsBeforeScaleDown)
	}
}

func TestMergeAdaptiveConfig_ClampMaxLessThanMin(t *testing.T) {
	cfg := mergeAdaptiveConfig(AdaptiveConfig{
		MinWorkers: 10,
		MaxWorkers: 5, // less than min
	})
	if cfg.MaxWorkers < cfg.MinWorkers {
		t.Errorf("MaxWorkers (%d) should be >= MinWorkers (%d)", cfg.MaxWorkers, cfg.MinWorkers)
	}
}

func TestNewAdaptiveController_InitialValues(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		MinWorkers: 4,
		MaxWorkers: 100,
	}, 16, 8, 100*time.Millisecond)

	if ac.Workers() != 16 {
		t.Errorf("expected initial workers=16, got %d", ac.Workers())
	}
	if ac.BatchSize() != 8 {
		t.Errorf("expected initial batch=8, got %d", ac.BatchSize())
	}
	if ac.PollInterval() != 100*time.Millisecond {
		t.Errorf("expected initial poll=100ms, got %v", ac.PollInterval())
	}
}

func TestNewAdaptiveController_ClampsInitialValues(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		MinWorkers: 10,
		MaxWorkers: 50,
	}, 2, 2, 1*time.Millisecond) // below minimums

	if ac.Workers() < 10 {
		t.Errorf("expected workers clamped to min 10, got %d", ac.Workers())
	}
}

func TestAdaptiveController_ScaleUpOnGrowingQueue(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:       true,
		ProbeInterval: 100 * time.Millisecond,
		MinWorkers:    4,
		MaxWorkers:    100,
		ScaleUpStep:   4,
	}, 8, 8, 100*time.Millisecond)

	initialWorkers := ac.Workers()

	// Simulate growing queue: depth increases between probes
	ac.SetQueueDepth(10)
	ac.probe()

	// Second probe with higher depth -> should scale up
	ac.SetQueueDepth(20)
	ac.probe()

	if ac.Workers() <= initialWorkers {
		t.Errorf("expected workers to increase from %d, got %d", initialWorkers, ac.Workers())
	}
}

func TestAdaptiveController_ScaleDownOnIdle(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:                     true,
		ProbeInterval:               100 * time.Millisecond,
		MinWorkers:                  4,
		MaxWorkers:                  100,
		ScaleDownRate:               0.5,
		StableRoundsBeforeScaleDown: 2,
	}, 32, 8, 100*time.Millisecond)

	initialWorkers := ac.Workers()

	// Simulate idle queue: depth=0, completed=0
	ac.SetQueueDepth(0)

	// Run enough probe rounds to trigger scale-down
	for i := 0; i < 5; i++ {
		ac.probe()
	}

	if ac.Workers() >= initialWorkers {
		t.Errorf("expected workers to decrease from %d, got %d", initialWorkers, ac.Workers())
	}
	if ac.Workers() < 4 {
		t.Errorf("workers should not go below MinWorkers=4, got %d", ac.Workers())
	}
}

func TestAdaptiveController_ScaleDownOnDraining(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:                     true,
		ProbeInterval:               100 * time.Millisecond,
		MinWorkers:                  4,
		MaxWorkers:                  100,
		ScaleDownRate:               0.5,
		StableRoundsBeforeScaleDown: 2,
	}, 32, 8, 100*time.Millisecond)

	initialWorkers := ac.Workers()

	// Simulate draining: depth=0 but tasks being completed
	ac.SetQueueDepth(0)
	ac.RecordCompleted(5)
	ac.probe()

	ac.RecordCompleted(3)
	ac.probe()

	ac.RecordCompleted(1)
	ac.probe()

	if ac.Workers() >= initialWorkers {
		t.Errorf("expected workers to decrease from %d after draining, got %d", initialWorkers, ac.Workers())
	}
}

func TestAdaptiveController_NeverExceedsBounds(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:       true,
		ProbeInterval: 100 * time.Millisecond,
		MinWorkers:    4,
		MaxWorkers:    20,
		ScaleUpStep:   10,
	}, 18, 8, 100*time.Millisecond)

	// Force many scale-ups
	for i := 0; i < 20; i++ {
		ac.SetQueueDepth(int64((i + 1) * 100))
		ac.probe()
	}

	if ac.Workers() > 20 {
		t.Errorf("workers should not exceed MaxWorkers=20, got %d", ac.Workers())
	}
}

func TestAdaptiveController_PollIntervalAdjusts(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:         true,
		ProbeInterval:   100 * time.Millisecond,
		MinWorkers:      4,
		MaxWorkers:      100,
		MinPollInterval: 10 * time.Millisecond,
		MaxPollInterval: 1 * time.Second,
	}, 8, 8, 200*time.Millisecond)

	initialPoll := ac.PollInterval()

	// Growing queue -> poll should decrease
	ac.SetQueueDepth(10)
	ac.probe()
	ac.SetQueueDepth(50)
	ac.probe()

	if ac.PollInterval() >= initialPoll {
		t.Errorf("expected poll interval to decrease under load, initial=%v, got=%v", initialPoll, ac.PollInterval())
	}
}

func TestAdaptiveController_BatchSizeScalesUp(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:      true,
		ProbeInterval: 100 * time.Millisecond,
		MinWorkers:   4,
		MaxWorkers:   100,
		MinBatchSize: 4,
		MaxBatchSize: 32,
	}, 8, 4, 100*time.Millisecond) // start with small batch

	initialBatch := ac.BatchSize()

	// Simulate very deep queue (much larger than batch*workers)
	ac.SetQueueDepth(10)
	ac.probe()
	ac.SetQueueDepth(1000)
	ac.probe()

	if ac.BatchSize() <= initialBatch {
		t.Errorf("expected batch size to increase under heavy load, initial=%d, got=%d", initialBatch, ac.BatchSize())
	}
}

func TestAdaptiveController_StartStop(t *testing.T) {
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:       true,
		ProbeInterval: 50 * time.Millisecond,
		MinWorkers:    2,
		MaxWorkers:    10,
	}, 4, 4, 50*time.Millisecond)

	ac.Start()
	time.Sleep(100 * time.Millisecond)
	ac.Stop()
	// Should not hang or panic
}

func TestCalcPollerCount(t *testing.T) {
	tests := []struct {
		workers, batch, want int
	}{
		{32, 8, 4},
		{4, 8, 2},  // min 2
		{100, 1, 8}, // max 8
		{0, 0, 2},   // edge case
	}
	for _, tt := range tests {
		got := calcPollerCount(tt.workers, tt.batch)
		if got != tt.want {
			t.Errorf("calcPollerCount(%d, %d) = %d, want %d", tt.workers, tt.batch, got, tt.want)
		}
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, lo, hi, want int
	}{
		{5, 1, 10, 5},
		{0, 1, 10, 1},
		{15, 1, 10, 10},
	}
	for _, tt := range tests {
		got := clampInt(tt.v, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestClampDuration(t *testing.T) {
	tests := []struct {
		v, lo, hi, want time.Duration
	}{
		{100 * time.Millisecond, 50 * time.Millisecond, 500 * time.Millisecond, 100 * time.Millisecond},
		{10 * time.Millisecond, 50 * time.Millisecond, 500 * time.Millisecond, 50 * time.Millisecond},
		{1 * time.Second, 50 * time.Millisecond, 500 * time.Millisecond, 500 * time.Millisecond},
	}
	for _, tt := range tests {
		got := clampDuration(tt.v, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("clampDuration(%v, %v, %v) = %v, want %v", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestAdaptiveController_SteadyState(t *testing.T) {
	// When queue has items but isn't growing, workers should remain roughly stable
	ac := newAdaptiveController(AdaptiveConfig{
		Enabled:       true,
		ProbeInterval: 100 * time.Millisecond,
		MinWorkers:    4,
		MaxWorkers:    100,
		ScaleUpStep:   4,
	}, 16, 8, 100*time.Millisecond)

	// Set a steady depth (not growing)
	ac.SetQueueDepth(20)
	ac.probe() // First probe establishes prevDepth

	w1 := ac.Workers()

	// Same depth, some completions
	ac.RecordCompleted(10)
	ac.SetQueueDepth(20)
	ac.probe()

	w2 := ac.Workers()

	// Workers should increase slightly (depth > workers) or stay same
	if w2 < w1 {
		t.Errorf("workers should not decrease in steady state with backlog, before=%d, after=%d", w1, w2)
	}
}

func TestNewWorkerPool_AdaptiveInitialized(t *testing.T) {
	// Verify that the adaptive controller is properly initialized when enabled.
	wp := New(nil, nil, Config{
		Workers:      16,
		PollInterval: 100 * time.Millisecond,
		BatchSize:    8,
		Adaptive: AdaptiveConfig{
			Enabled:    true,
			MinWorkers: 4,
			MaxWorkers: 64,
		},
	})
	if wp.adaptive == nil {
		t.Fatal("expected adaptive controller to be initialized when Enabled=true")
	}
	if wp.adaptive.Workers() != 16 {
		t.Errorf("expected initial workers=16, got %d", wp.adaptive.Workers())
	}
}

func TestNewWorkerPool_AdaptiveDisabled(t *testing.T) {
	// Verify that the adaptive controller is nil when disabled.
	wp := New(nil, nil, Config{
		Workers:      16,
		PollInterval: 100 * time.Millisecond,
		BatchSize:    8,
		Adaptive: AdaptiveConfig{
			Enabled: false,
		},
	})
	if wp.adaptive != nil {
		t.Fatal("expected adaptive controller to be nil when Enabled=false")
	}
}
