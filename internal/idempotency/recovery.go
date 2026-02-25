package idempotency

import (
	"context"
	"sync"
	"time"
)

// RecoveryConfig configures failure recovery behavior.
type RecoveryConfig struct {
	ScanInterval  time.Duration // How often to scan for expired leases
	MaxRecoveries int           // Max concurrent recovery attempts
	RetryDelay    time.Duration // Delay before retrying a recovered task
}

// DefaultRecoveryConfig returns sensible defaults.
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		ScanInterval:  30 * time.Second,
		MaxRecoveries: 10,
		RetryDelay:    5 * time.Second,
	}
}

// RecoverableTask represents a task that can be recovered after lease expiry.
type RecoverableTask struct {
	IdempotencyKey string    `json:"idempotency_key"`
	FunctionID     string    `json:"function_id"`
	Payload        []byte    `json:"payload"`
	OriginalOwner  string    `json:"original_owner"`
	FailedAt       time.Time `json:"failed_at"`
	RecoveryCount  int       `json:"recovery_count"`
}

// RecoveryHandler is called when a task is recovered for re-execution.
type RecoveryHandler func(ctx context.Context, task *RecoverableTask) error

// RecoveryManager monitors expired leases and re-dispatches failed tasks.
type RecoveryManager struct {
	mu           sync.Mutex
	leaseManager *LeaseManager
	inbox        *Inbox
	store        *Store
	cfg          RecoveryConfig
	handler      RecoveryHandler
	pending      map[string]*RecoverableTask
	ctx          context.Context
	cancel       context.CancelFunc

	// Stats
	recovered int64
	failed    int64
}

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(
	leaseManager *LeaseManager,
	inbox *Inbox,
	store *Store,
	cfg RecoveryConfig,
	handler RecoveryHandler,
) *RecoveryManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &RecoveryManager{
		leaseManager: leaseManager,
		inbox:        inbox,
		store:        store,
		cfg:          cfg,
		handler:      handler,
		pending:      make(map[string]*RecoverableTask),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins the recovery scan loop.
func (rm *RecoveryManager) Start() {
	go rm.scanLoop()
}

// Stop halts the recovery manager.
func (rm *RecoveryManager) Stop() {
	rm.cancel()
}

// SubmitForRecovery manually submits a task for recovery.
func (rm *RecoveryManager) SubmitForRecovery(task *RecoverableTask) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.pending[task.IdempotencyKey] = task
}

// Stats returns recovery statistics.
func (rm *RecoveryManager) Stats() RecoveryStats {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return RecoveryStats{
		Recovered:    rm.recovered,
		Failed:       rm.failed,
		PendingCount: len(rm.pending),
	}
}

// RecoveryStats contains recovery operational statistics.
type RecoveryStats struct {
	Recovered    int64 `json:"recovered"`
	Failed       int64 `json:"failed"`
	PendingCount int   `json:"pending_count"`
}

func (rm *RecoveryManager) scanLoop() {
	ticker := time.NewTicker(rm.cfg.ScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.recoverPending()
		}
	}
}

func (rm *RecoveryManager) recoverPending() {
	rm.mu.Lock()
	tasks := make([]*RecoverableTask, 0, len(rm.pending))
	for _, task := range rm.pending {
		tasks = append(tasks, task)
	}
	rm.mu.Unlock()

	recovered := 0
	for _, task := range tasks {
		if recovered >= rm.cfg.MaxRecoveries {
			break
		}

		// Check if the task was already completed via inbox
		if rm.inbox != nil {
			entry := rm.inbox.Lookup(task.IdempotencyKey)
			if entry != nil && entry.Status == "done" {
				// Already completed by another worker
				rm.mu.Lock()
				delete(rm.pending, task.IdempotencyKey)
				rm.mu.Unlock()
				continue
			}
		}

		// Check idempotency store
		if rm.store != nil {
			result := rm.store.Check(rm.ctx, task.IdempotencyKey, "recovery-manager")
			if result.Hit && result.Status == StatusCompleted {
				rm.mu.Lock()
				delete(rm.pending, task.IdempotencyKey)
				rm.mu.Unlock()
				continue
			}
		}

		// Re-execute via handler
		if rm.handler != nil {
			err := rm.handler(rm.ctx, task)
			rm.mu.Lock()
			if err != nil {
				task.RecoveryCount++
				rm.failed++
			} else {
				delete(rm.pending, task.IdempotencyKey)
				rm.recovered++
				recovered++
			}
			rm.mu.Unlock()
		}
	}
}
