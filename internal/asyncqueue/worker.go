package asyncqueue

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// Config configures async invocation workers.
type Config struct {
	Workers       int
	PollInterval  time.Duration
	LeaseDuration time.Duration
	InvokeTimeout time.Duration
}

// WorkerPool polls queued async invocations and executes them.
type WorkerPool struct {
	store   *store.Store
	exec    executor.Invoker
	cfg     Config
	stopCh  chan struct{}
	started bool
	mu      sync.Mutex
	wg      sync.WaitGroup
}

// New creates a new async worker pool.
func New(s *store.Store, exec executor.Invoker, cfg Config) *WorkerPool {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = store.DefaultAsyncLeaseTimeout
	}
	if cfg.InvokeTimeout <= 0 {
		cfg.InvokeTimeout = 5 * time.Minute
	}
	return &WorkerPool{
		store:  s,
		exec:   exec,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

// Start launches worker goroutines.
func (w *WorkerPool) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	w.started = true

	for i := 0; i < w.cfg.Workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}
	logging.Op().Info("async queue workers started", "workers", w.cfg.Workers, "poll_interval", w.cfg.PollInterval)
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

func (w *WorkerPool) worker(id int) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	workerID := fmt.Sprintf("async-worker-%d", id)
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.poll(workerID)
		}
	}
}

func (w *WorkerPool) poll(workerID string) {
	// Check if global pause is enabled
	paused, err := w.store.GetGlobalAsyncPause(context.Background())
	if err != nil {
		logging.Op().Error("check global async pause failed", "worker", workerID, "error", err)
		return
	}
	if paused {
		// Global pause is enabled, skip processing
		return
	}

	job, err := w.store.AcquireDueAsyncInvocation(context.Background(), workerID, w.cfg.LeaseDuration)
	if err != nil {
		logging.Op().Error("acquire async invocation failed", "worker", workerID, "error", err)
		return
	}
	if job == nil {
		return
	}

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
