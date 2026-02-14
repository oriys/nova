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
	return &WorkerPool{
		store:    s,
		exec:     exec,
		cfg:      cfg,
		notifier: notifier,
		stopCh:   make(chan struct{}),
		taskCh:   make(chan *store.AsyncInvocation, cfg.Workers*cfg.BatchSize),
	}
}

// Start launches poller and worker goroutines.
func (w *WorkerPool) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	w.started = true

	// Start worker goroutines that process tasks from the channel
	for i := 0; i < w.cfg.Workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}

	// Start poller goroutines that fetch tasks from DB and feed the channel.
	// Use fewer pollers than workers since each poller fetches a batch.
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

	jobs, err := w.store.AcquireDueAsyncInvocations(context.Background(), pollerID, w.cfg.LeaseDuration, w.cfg.BatchSize)
	if err != nil {
		logging.Op().Error("acquire async invocations failed", "poller", pollerID, "error", err)
		return
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
	if len(jobs) >= w.cfg.BatchSize {
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
