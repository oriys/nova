package asyncqueue

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/queue"
	"github.com/oriys/nova/internal/store"
)

// Config configures async invocation workers.
type Config struct {
	Workers       int
	PollInterval  time.Duration
	LeaseDuration time.Duration
	InvokeTimeout time.Duration
	BatchSize     int
	Notifier      queue.Notifier // optional push-based notifier to reduce polling
	Adaptive      AdaptiveConfig // optional adaptive concurrency control
}

// WorkerPool polls queued async invocations and executes them.
type WorkerPool struct {
	store    *store.Store
	exec     executor.Invoker
	cfg      Config
	notifier queue.Notifier
	stopCh   chan struct{}
	taskCh   chan *store.AsyncInvocation
	started  bool
	mu       sync.Mutex
	wg       sync.WaitGroup

	// adaptive controller for dynamic concurrency tuning (nil when disabled)
	adaptive *AdaptiveController

	// cached global pause state to avoid querying DB on every poll
	pausedUntil atomic.Int64 // unix nano timestamp until which cached pause value is valid
	pausedVal   atomic.Int32 // 0 = not paused, 1 = paused
}

const (
	defaultWorkers      = 32
	defaultPollInterval = 100 * time.Millisecond
	defaultBatchSize    = 8
	pauseCacheDuration  = 2 * time.Second
)

// New creates a new async worker pool.
func New(s *store.Store, exec executor.Invoker, cfg Config) *WorkerPool {
	if cfg.Workers <= 0 {
		cfg.Workers = defaultWorkers
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = store.DefaultAsyncLeaseTimeout
	}
	if cfg.InvokeTimeout <= 0 {
		cfg.InvokeTimeout = 5 * time.Minute
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	notifier := cfg.Notifier
	if notifier == nil {
		notifier = queue.NewNoopNotifier()
	}
	wp := &WorkerPool{
		store:    s,
		exec:     exec,
		cfg:      cfg,
		notifier: notifier,
		stopCh:   make(chan struct{}),
		taskCh:   make(chan *store.AsyncInvocation, cfg.Workers*cfg.BatchSize),
	}
	if cfg.Adaptive.Enabled {
		wp.adaptive = newAdaptiveController(cfg.Adaptive, cfg.Workers, cfg.BatchSize, cfg.PollInterval)
	}
	return wp
}

// Start launches poller and worker goroutines.
func (w *WorkerPool) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	w.started = true

	if w.adaptive != nil {
		// Adaptive mode: start the controller and use elastic goroutines.
		w.adaptive.Start()
		w.wg.Add(1)
		go w.elasticWorkerManager()
		w.wg.Add(1)
		go w.elasticPollerManager()

		logging.Op().Info("async queue workers started (adaptive mode)",
			"initial_workers", w.adaptive.Workers(),
			"initial_pollers", w.adaptive.Pollers(),
			"initial_poll_interval", w.adaptive.PollInterval(),
			"initial_batch_size", w.adaptive.BatchSize(),
		)
		return
	}

	// Static mode: fixed number of workers and pollers.
	for i := 0; i < w.cfg.Workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}

	pollerCount := w.cfg.Workers / w.cfg.BatchSize
	if pollerCount < 2 {
		pollerCount = 2
	}
	if pollerCount > 8 {
		pollerCount = 8
	}
	for i := 0; i < pollerCount; i++ {
		w.wg.Add(1)
		go w.poller(i)
	}

	logging.Op().Info("async queue workers started",
		"workers", w.cfg.Workers,
		"pollers", pollerCount,
		"poll_interval", w.cfg.PollInterval,
		"batch_size", w.cfg.BatchSize,
	)
}

// Stop gracefully shuts down all workers.
func (w *WorkerPool) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	w.started = false
	close(w.stopCh)
	w.mu.Unlock()

	if w.adaptive != nil {
		w.adaptive.Stop()
	}
	w.wg.Wait()
	logging.Op().Info("async queue workers stopped")
}

// poller fetches batches of tasks from DB and dispatches them to workers via taskCh.
func (w *WorkerPool) poller(id int) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifyCh := w.notifier.Subscribe(ctx, queue.QueueAsync)

	pollerID := fmt.Sprintf("async-poller-%d", id)
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.pollBatch(pollerID)
		case <-notifyCh:
			w.pollBatch(pollerID)
		}
	}
}

func (w *WorkerPool) pollBatch(pollerID string) {
	// Check cached global pause state
	if w.isGloballyPaused() {
		return
	}

	batchSize := w.cfg.BatchSize
	if w.adaptive != nil {
		batchSize = w.adaptive.BatchSize()
	}

	jobs, err := w.store.AcquireDueAsyncInvocations(context.Background(), pollerID, w.cfg.LeaseDuration, batchSize)
	if err != nil {
		logging.Op().Error("acquire async invocations failed", "poller", pollerID, "error", err)
		return
	}

	// Feed queue depth signal to adaptive controller.
	if w.adaptive != nil {
		// When we get a full batch, the queue likely has more pending work.
		// Signal higher depth to indicate continued pressure.
		if len(jobs) >= batchSize {
			w.adaptive.SetQueueDepth(int64(batchSize) * 2)
		} else {
			w.adaptive.SetQueueDepth(int64(len(jobs)))
		}
	}

	if len(jobs) == 0 {
		return
	}

	for _, job := range jobs {
		select {
		case w.taskCh <- job:
		case <-w.stopCh:
			return
		}
	}

	// If we got a full batch, immediately try again (drain loop)
	if len(jobs) >= batchSize {
		select {
		case <-w.stopCh:
			return
		default:
			w.pollBatch(pollerID)
		}
	}
}

// isGloballyPaused returns the cached pause state, refreshing from DB when expired.
func (w *WorkerPool) isGloballyPaused() bool {
	now := time.Now().UnixNano()
	if now < w.pausedUntil.Load() {
		return w.pausedVal.Load() != 0
	}

	paused, err := w.store.GetGlobalAsyncPause(context.Background())
	if err != nil {
		logging.Op().Error("check global async pause failed", "error", err)
		return false
	}
	if paused {
		w.pausedVal.Store(1)
	} else {
		w.pausedVal.Store(0)
	}
	w.pausedUntil.Store(now + int64(pauseCacheDuration))
	return paused
}

// worker processes tasks from the channel.
func (w *WorkerPool) worker(id int) {
	defer w.wg.Done()
	workerID := fmt.Sprintf("async-worker-%d", id)

	for {
		select {
		case <-w.stopCh:
			return
		case job := <-w.taskCh:
			w.processJob(workerID, job)
		}
	}
}

func (w *WorkerPool) processJob(workerID string, job *store.AsyncInvocation) {
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.InvokeTimeout)
	ctx = store.WithTenantScope(ctx, job.TenantID, job.Namespace)
	defer cancel()

	resp, invokeErr := w.exec.Invoke(ctx, job.FunctionName, job.Payload)

	// Record completion for adaptive controller regardless of success/failure.
	if w.adaptive != nil {
		w.adaptive.RecordCompleted(1)
	}

	errMsg := ""
	if invokeErr != nil {
		errMsg = invokeErr.Error()
	} else if resp == nil {
		errMsg = "empty invocation response"
	} else if resp.Error != "" {
		errMsg = resp.Error
	}

	if errMsg == "" {
		if err := w.store.MarkAsyncInvocationSucceeded(context.Background(), job.ID, resp.RequestID, resp.Output, resp.DurationMs, resp.ColdStart); err != nil {
			logging.Op().Error("mark async invocation succeeded failed", "job", job.ID, "error", err)
			return
		}
		logging.Op().Debug("async invocation succeeded", "job", job.ID, "function", job.FunctionName, "attempt", job.Attempt)
		return
	}

	if job.Attempt >= job.MaxAttempts {
		if err := w.store.MarkAsyncInvocationDLQ(context.Background(), job.ID, errMsg); err != nil {
			logging.Op().Error("mark async invocation dlq failed", "job", job.ID, "error", err)
			return
		}
		logging.Op().Warn("async invocation moved to dlq", "job", job.ID, "function", job.FunctionName, "attempt", job.Attempt, "max_attempts", job.MaxAttempts, "error", errMsg)
		return
	}

	backoff := calcBackoff(job.Attempt, job.BackoffBaseMS, job.BackoffMaxMS)
	nextRun := time.Now().UTC().Add(backoff)
	if err := w.store.MarkAsyncInvocationForRetry(context.Background(), job.ID, errMsg, nextRun); err != nil {
		logging.Op().Error("mark async invocation retry failed", "job", job.ID, "error", err)
		return
	}
	logging.Op().Warn("async invocation retry scheduled", "job", job.ID, "function", job.FunctionName, "attempt", job.Attempt, "next_run_at", nextRun, "error", errMsg)
}

// elasticWorkerManager dynamically adjusts the number of active workers
// based on the adaptive controller's target. It runs a reconciliation loop
// that spawns or stops worker goroutines as needed.
func (w *WorkerPool) elasticWorkerManager() {
	defer w.wg.Done()

	type workerHandle struct {
		cancel context.CancelFunc
	}

	var handles []workerHandle
	reconcile := func() {
		target := w.adaptive.Workers()
		current := len(handles)

		if target > current {
			// Scale up: spawn new workers
			for i := current; i < target; i++ {
				ctx, cancel := context.WithCancel(context.Background())
				handles = append(handles, workerHandle{cancel: cancel})
				w.wg.Add(1)
				go w.elasticWorker(i, ctx)
			}
		} else if target < current {
			// Scale down: cancel excess workers (LIFO order)
			for i := current - 1; i >= target; i-- {
				handles[i].cancel()
			}
			handles = handles[:target]
		}
	}

	// Initial spawn
	reconcile()

	ticker := time.NewTicker(w.adaptive.cfg.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			// Cancel all remaining workers
			for _, h := range handles {
				h.cancel()
			}
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

// elasticWorker is a worker goroutine that processes tasks until its context
// is cancelled (scale-down) or the pool stops.
func (w *WorkerPool) elasticWorker(id int, ctx context.Context) {
	defer w.wg.Done()
	workerID := fmt.Sprintf("async-worker-%d", id)

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case job := <-w.taskCh:
			w.processJob(workerID, job)
		}
	}
}

// elasticPollerManager dynamically adjusts poller goroutines and their poll
// intervals based on the adaptive controller's target values.
func (w *WorkerPool) elasticPollerManager() {
	defer w.wg.Done()

	type pollerHandle struct {
		cancel context.CancelFunc
	}

	var handles []pollerHandle
	reconcile := func() {
		target := w.adaptive.Pollers()
		current := len(handles)

		if target > current {
			for i := current; i < target; i++ {
				ctx, cancel := context.WithCancel(context.Background())
				handles = append(handles, pollerHandle{cancel: cancel})
				w.wg.Add(1)
				go w.elasticPoller(i, ctx)
			}
		} else if target < current {
			for i := current - 1; i >= target; i-- {
				handles[i].cancel()
			}
			handles = handles[:target]
		}
	}

	reconcile()

	ticker := time.NewTicker(w.adaptive.cfg.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			for _, h := range handles {
				h.cancel()
			}
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

// elasticPoller is a poller goroutine whose poll interval is read dynamically
// from the adaptive controller.
func (w *WorkerPool) elasticPoller(id int, ctx context.Context) {
	defer w.wg.Done()

	pollInterval := w.adaptive.PollInterval()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	notifyCh := w.notifier.Subscribe(bgCtx, queue.QueueAsync)

	pollerID := fmt.Sprintf("async-poller-%d", id)
	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.pollBatch(pollerID)
			// Re-read poll interval from controller and reset ticker if changed.
			newInterval := w.adaptive.PollInterval()
			if newInterval != pollInterval {
				pollInterval = newInterval
				ticker.Reset(pollInterval)
			}
		case <-notifyCh:
			w.pollBatch(pollerID)
		}
	}
}

func calcBackoff(attempt, baseMS, maxMS int) time.Duration {
	if baseMS <= 0 {
		baseMS = store.DefaultAsyncBackoffBase
	}
	if maxMS <= 0 {
		maxMS = store.DefaultAsyncBackoffMax
	}
	if maxMS < baseMS {
		maxMS = baseMS
	}
	if attempt < 1 {
		attempt = 1
	}

	ms := float64(baseMS) * math.Pow(2, float64(attempt-1))
	if ms > float64(maxMS) {
		ms = float64(maxMS)
	}
	return time.Duration(ms) * time.Millisecond
}
