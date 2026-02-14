package asyncqueue

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oriys/nova/internal/logging"
)

// AdaptiveController dynamically adjusts worker count, poll interval, and
// batch size based on observed queue depth and processing throughput.
//
// Algorithm:
//   - Every probe interval, the controller reads the current queue depth
//     (pending tasks) and the number of tasks completed since the last probe.
//   - When the queue is growing (depth > previous depth), the controller
//     increases concurrency (additive increase) and shortens the poll interval.
//   - When the queue is shrinking or empty, the controller decreases
//     concurrency (multiplicative decrease) and lengthens the poll interval.
//   - All values are clamped to configured min/max bounds.
//
// This is inspired by the AIMD (Additive Increase / Multiplicative Decrease)
// pattern used in TCP congestion control, adapted for task queue throughput.
type AdaptiveController struct {
	cfg AdaptiveConfig

	// Current dynamic parameters (read by pollers/workers via atomic loads).
	currentWorkers  atomic.Int32
	currentBatch    atomic.Int32
	currentPollNs   atomic.Int64 // poll interval in nanoseconds
	currentPollers  atomic.Int32

	// Metrics fed by the worker pool.
	completedCount atomic.Int64 // tasks completed since last probe
	queueDepth     atomic.Int64 // latest known queue depth

	// Internal state for the control loop.
	prevDepth    int64
	prevRate     float64 // tasks/sec observed in the previous window
	stableRounds int     // consecutive rounds with low/empty queue

	mu     sync.Mutex
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// AdaptiveConfig configures the adaptive concurrency controller.
type AdaptiveConfig struct {
	Enabled bool // Enable adaptive scaling (default: false)

	// Probe interval: how often the controller re-evaluates parameters.
	ProbeInterval time.Duration // default: 2s

	// Worker bounds
	MinWorkers int // default: 4
	MaxWorkers int // default: 256

	// Batch size bounds
	MinBatchSize int // default: 4
	MaxBatchSize int // default: 32

	// Poll interval bounds
	MinPollInterval time.Duration // default: 20ms  (aggressive polling under load)
	MaxPollInterval time.Duration // default: 500ms (relaxed polling when idle)

	// Scale factors
	ScaleUpStep   int     // workers to add per scale-up   (default: 4)
	ScaleDownRate float64 // multiplicative factor for scale-down (default: 0.75)

	// StableRoundsBeforeScaleDown: require N consecutive low-depth rounds
	// before reducing concurrency, to avoid premature scale-down.
	StableRoundsBeforeScaleDown int // default: 3
}

func defaultAdaptiveConfig() AdaptiveConfig {
	return AdaptiveConfig{
		Enabled:                     false,
		ProbeInterval:               2 * time.Second,
		MinWorkers:                  4,
		MaxWorkers:                  256,
		MinBatchSize:                4,
		MaxBatchSize:                32,
		MinPollInterval:             20 * time.Millisecond,
		MaxPollInterval:             500 * time.Millisecond,
		ScaleUpStep:                 4,
		ScaleDownRate:               0.75,
		StableRoundsBeforeScaleDown: 3,
	}
}

func mergeAdaptiveConfig(cfg AdaptiveConfig) AdaptiveConfig {
	d := defaultAdaptiveConfig()
	cfg.Enabled = cfg.Enabled // keep caller value

	if cfg.ProbeInterval <= 0 {
		cfg.ProbeInterval = d.ProbeInterval
	}
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = d.MinWorkers
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = d.MaxWorkers
	}
	if cfg.MaxWorkers < cfg.MinWorkers {
		cfg.MaxWorkers = cfg.MinWorkers
	}
	if cfg.MinBatchSize <= 0 {
		cfg.MinBatchSize = d.MinBatchSize
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = d.MaxBatchSize
	}
	if cfg.MaxBatchSize < cfg.MinBatchSize {
		cfg.MaxBatchSize = cfg.MinBatchSize
	}
	if cfg.MinPollInterval <= 0 {
		cfg.MinPollInterval = d.MinPollInterval
	}
	if cfg.MaxPollInterval <= 0 {
		cfg.MaxPollInterval = d.MaxPollInterval
	}
	if cfg.MaxPollInterval < cfg.MinPollInterval {
		cfg.MaxPollInterval = cfg.MinPollInterval
	}
	if cfg.ScaleUpStep <= 0 {
		cfg.ScaleUpStep = d.ScaleUpStep
	}
	if cfg.ScaleDownRate <= 0 || cfg.ScaleDownRate >= 1 {
		cfg.ScaleDownRate = d.ScaleDownRate
	}
	if cfg.StableRoundsBeforeScaleDown <= 0 {
		cfg.StableRoundsBeforeScaleDown = d.StableRoundsBeforeScaleDown
	}
	return cfg
}

// newAdaptiveController creates a new adaptive controller.
// initialWorkers / initialBatch / initialPoll are the starting values
// (typically from the static Config).
func newAdaptiveController(cfg AdaptiveConfig, initialWorkers, initialBatch int, initialPoll time.Duration) *AdaptiveController {
	cfg = mergeAdaptiveConfig(cfg)

	workers := clampInt(initialWorkers, cfg.MinWorkers, cfg.MaxWorkers)
	batch := clampInt(initialBatch, cfg.MinBatchSize, cfg.MaxBatchSize)
	poll := clampDuration(initialPoll, cfg.MinPollInterval, cfg.MaxPollInterval)

	ac := &AdaptiveController{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
	ac.currentWorkers.Store(int32(workers))
	ac.currentBatch.Store(int32(batch))
	ac.currentPollNs.Store(int64(poll))
	ac.currentPollers.Store(int32(calcPollerCount(workers, batch)))
	return ac
}

// Start begins the background control loop.
func (ac *AdaptiveController) Start() {
	ac.wg.Add(1)
	go ac.loop()
}

// Stop signals the control loop to exit and waits for it.
func (ac *AdaptiveController) Stop() {
	close(ac.stopCh)
	ac.wg.Wait()
}

// RecordCompleted increments the completed task counter.
// Called by workers after each successful or failed task processing.
func (ac *AdaptiveController) RecordCompleted(n int64) {
	ac.completedCount.Add(n)
}

// SetQueueDepth updates the latest known queue depth.
// Called by pollers after acquiring tasks (with the remaining depth).
func (ac *AdaptiveController) SetQueueDepth(depth int64) {
	ac.queueDepth.Store(depth)
}

// Workers returns the current target worker count.
func (ac *AdaptiveController) Workers() int {
	return int(ac.currentWorkers.Load())
}

// BatchSize returns the current target batch size.
func (ac *AdaptiveController) BatchSize() int {
	return int(ac.currentBatch.Load())
}

// PollInterval returns the current target poll interval.
func (ac *AdaptiveController) PollInterval() time.Duration {
	return time.Duration(ac.currentPollNs.Load())
}

// Pollers returns the current target poller count.
func (ac *AdaptiveController) Pollers() int {
	return int(ac.currentPollers.Load())
}

func (ac *AdaptiveController) loop() {
	defer ac.wg.Done()
	ticker := time.NewTicker(ac.cfg.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ac.stopCh:
			return
		case <-ticker.C:
			ac.probe()
		}
	}
}

func (ac *AdaptiveController) probe() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Snapshot and reset counters
	completed := ac.completedCount.Swap(0)
	depth := ac.queueDepth.Load()

	// Compute observed throughput (tasks/sec)
	windowSecs := ac.cfg.ProbeInterval.Seconds()
	if windowSecs <= 0 {
		windowSecs = 1
	}
	rate := float64(completed) / windowSecs

	workers := int(ac.currentWorkers.Load())
	batch := int(ac.currentBatch.Load())
	pollNs := ac.currentPollNs.Load()

	// Determine pressure: is the queue growing, stable, or draining?
	growing := depth > 0 && depth > ac.prevDepth
	idle := depth == 0 && completed == 0
	draining := depth == 0 && completed > 0

	switch {
	case growing:
		// Queue is building up -> scale up (additive increase)
		ac.stableRounds = 0
		workers = minInt(workers+ac.cfg.ScaleUpStep, ac.cfg.MaxWorkers)

		// Also increase batch size proportionally to depth pressure
		if depth > int64(batch*workers) {
			batch = minInt(batch+2, ac.cfg.MaxBatchSize)
		}

		// Shorten poll interval
		newPoll := time.Duration(float64(pollNs) * 0.75)
		pollNs = int64(clampDuration(newPoll, ac.cfg.MinPollInterval, ac.cfg.MaxPollInterval))

	case idle:
		// No work at all -> aggressively scale down
		ac.stableRounds++
		if ac.stableRounds >= ac.cfg.StableRoundsBeforeScaleDown {
			workers = maxInt(int(math.Ceil(float64(workers)*ac.cfg.ScaleDownRate)), ac.cfg.MinWorkers)
			batch = maxInt(batch-1, ac.cfg.MinBatchSize)

			// Lengthen poll interval
			newPoll := time.Duration(float64(pollNs) * 1.5)
			pollNs = int64(clampDuration(newPoll, ac.cfg.MinPollInterval, ac.cfg.MaxPollInterval))
		}

	case draining:
		// Completing tasks but no backlog -> gentle reduce
		ac.stableRounds++
		if ac.stableRounds >= ac.cfg.StableRoundsBeforeScaleDown {
			workers = maxInt(int(math.Ceil(float64(workers)*ac.cfg.ScaleDownRate)), ac.cfg.MinWorkers)

			newPoll := time.Duration(float64(pollNs) * 1.25)
			pollNs = int64(clampDuration(newPoll, ac.cfg.MinPollInterval, ac.cfg.MaxPollInterval))
		}

	default:
		// depth > 0 but not growing (steady or shrinking) -> hold or slight scale-up
		ac.stableRounds = 0
		if rate > 0 && depth > int64(workers) {
			// Still some backlog but keeping up -> small bump
			workers = minInt(workers+1, ac.cfg.MaxWorkers)
		}
	}

	// Apply new values
	pollers := calcPollerCount(workers, batch)

	ac.currentWorkers.Store(int32(workers))
	ac.currentBatch.Store(int32(batch))
	ac.currentPollNs.Store(pollNs)
	ac.currentPollers.Store(int32(pollers))

	ac.prevDepth = depth
	ac.prevRate = rate

	logging.Op().Debug("adaptive controller probe",
		"depth", depth,
		"completed", completed,
		"rate_per_sec", rate,
		"workers", workers,
		"pollers", pollers,
		"batch_size", batch,
		"poll_interval", time.Duration(pollNs),
	)
}

func calcPollerCount(workers, batchSize int) int {
	if batchSize <= 0 {
		batchSize = 1
	}
	count := workers / batchSize
	if count < 2 {
		count = 2
	}
	if count > 8 {
		count = 8
	}
	return count
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampDuration(v, lo, hi time.Duration) time.Duration {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
