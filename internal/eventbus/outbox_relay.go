package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// OutboxRelayConfig configures outbox relay workers.
type OutboxRelayConfig struct {
	Workers       int
	PollInterval  time.Duration
	LeaseDuration time.Duration
}

// OutboxRelay relays pending outbox jobs into event messages.
type OutboxRelay struct {
	store   *store.Store
	cfg     OutboxRelayConfig
	stopCh  chan struct{}
	started bool
	mu      sync.Mutex
	wg      sync.WaitGroup
}

// NewOutboxRelay creates a new outbox relay worker pool.
func NewOutboxRelay(s *store.Store, cfg OutboxRelayConfig) *OutboxRelay {
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = store.DefaultOutboxLeaseTimeout
	}
	return &OutboxRelay{
		store:  s,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

// Start launches relay workers.
func (r *OutboxRelay) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return
	}
	r.started = true
	for i := 0; i < r.cfg.Workers; i++ {
		r.wg.Add(1)
		go r.worker(i)
	}
	logging.Op().Info("event outbox relay started", "workers", r.cfg.Workers, "poll_interval", r.cfg.PollInterval)
}

// Stop gracefully stops relay workers.
func (r *OutboxRelay) Stop() {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	r.started = false
	close(r.stopCh)
	r.mu.Unlock()

	r.wg.Wait()
	logging.Op().Info("event outbox relay stopped")
}

func (r *OutboxRelay) worker(id int) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	workerID := fmt.Sprintf("outbox-relay-%d", id)
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.poll(workerID)
		}
	}
}

func (r *OutboxRelay) poll(workerID string) {
	job, err := r.store.AcquireDueEventOutbox(context.Background(), workerID, r.cfg.LeaseDuration)
	if err != nil {
		logging.Op().Error("acquire event outbox failed", "worker", workerID, "error", err)
		return
	}
	if job == nil {
		return
	}

	publishCtx := store.WithTenantScope(context.Background(), job.TenantID, job.Namespace)
	msg, fanout, newlyPublished, err := r.store.PublishEventFromOutbox(publishCtx, job.ID, job.TopicID, job.OrderingKey, job.Payload, job.Headers)
	if err == nil {
		if err := r.store.MarkEventOutboxPublished(context.Background(), job.ID, msg.ID); err != nil {
			logging.Op().Error("mark event outbox published failed", "outbox", job.ID, "message_id", msg.ID, "error", err)
			return
		}
		logging.Op().Debug("event outbox relayed", "outbox", job.ID, "message_id", msg.ID, "topic", job.TopicName, "fanout", fanout, "newly_published", newlyPublished)
		return
	}

	errMsg := err.Error()
	if job.Attempt >= job.MaxAttempts {
		if markErr := r.store.MarkEventOutboxFailed(context.Background(), job.ID, errMsg); markErr != nil {
			logging.Op().Error("mark event outbox failed status failed", "outbox", job.ID, "error", markErr)
			return
		}
		logging.Op().Warn("event outbox moved to failed", "outbox", job.ID, "topic", job.TopicName, "attempt", job.Attempt, "max_attempts", job.MaxAttempts, "error", errMsg)
		return
	}

	backoff := calcBackoff(job.Attempt, job.BackoffBaseMS, job.BackoffMaxMS)
	nextRun := time.Now().UTC().Add(backoff)
	if markErr := r.store.MarkEventOutboxForRetry(context.Background(), job.ID, errMsg, nextRun); markErr != nil {
		logging.Op().Error("mark event outbox retry failed", "outbox", job.ID, "error", markErr)
		return
	}
	logging.Op().Warn("event outbox retry scheduled", "outbox", job.ID, "topic", job.TopicName, "attempt", job.Attempt, "next_run_at", nextRun, "error", errMsg)
}
