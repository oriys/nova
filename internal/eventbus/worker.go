package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// Config configures event delivery workers.
type Config struct {
	Workers       int
	PollInterval  time.Duration
	LeaseDuration time.Duration
	InvokeTimeout time.Duration
}

// WorkerPool polls queued deliveries and invokes subscribed functions.
type WorkerPool struct {
	store   *store.Store
	exec    *executor.Executor
	cfg     Config
	stopCh  chan struct{}
	started bool
	mu      sync.Mutex
	wg      sync.WaitGroup
}

// New creates a new event delivery worker pool.
func New(s *store.Store, exec *executor.Executor, cfg Config) *WorkerPool {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = store.DefaultEventLeaseTimeout
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
	logging.Op().Info("event bus workers started", "workers", w.cfg.Workers, "poll_interval", w.cfg.PollInterval)
}

// Stop gracefully shuts down workers.
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
	logging.Op().Info("event bus workers stopped")
}

func (w *WorkerPool) worker(id int) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	workerID := fmt.Sprintf("event-worker-%d", id)
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
	delivery, err := w.store.AcquireDueEventDelivery(context.Background(), workerID, w.cfg.LeaseDuration)
	if err != nil {
		logging.Op().Error("acquire event delivery failed", "worker", workerID, "error", err)
		return
	}
	if delivery == nil {
		return
	}

	inboxRec, deduplicated, err := w.store.PrepareEventInbox(context.Background(), delivery.SubscriptionID, delivery.MessageID, delivery.ID)
	if err != nil {
		logging.Op().Error("prepare event inbox failed", "delivery", delivery.ID, "error", err)
		w.retryOrDLQ(delivery, "prepare inbox: "+err.Error())
		return
	}
	if deduplicated {
		var output json.RawMessage
		var durationMS int64
		var coldStart bool
		var requestID string
		if inboxRec != nil {
			output = inboxRec.Output
			durationMS = inboxRec.DurationMS
			coldStart = inboxRec.ColdStart
			requestID = inboxRec.RequestID
		}
		if err := w.store.MarkEventDeliverySucceeded(context.Background(), delivery.ID, requestID, output, durationMS, coldStart); err != nil {
			logging.Op().Error("mark deduplicated event delivery succeeded failed", "delivery", delivery.ID, "error", err)
			w.retryOrDLQ(delivery, "mark deduplicated success: "+err.Error())
			return
		}
		logging.Op().Debug("event delivery deduplicated by inbox", "delivery", delivery.ID, "topic", delivery.TopicName, "subscription", delivery.SubscriptionName, "message_id", delivery.MessageID)
		return
	}

	payload, err := buildEventInvocationPayload(delivery)
	if err != nil {
		if markInboxErr := w.store.MarkEventInboxFailed(context.Background(), delivery.SubscriptionID, delivery.MessageID, delivery.ID, "serialize payload: "+err.Error()); markInboxErr != nil {
			logging.Op().Error("mark event inbox failed after serialization failure failed", "delivery", delivery.ID, "error", markInboxErr)
		}
		w.retryOrDLQ(delivery, "serialize event payload: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.InvokeTimeout)
	defer cancel()

	resp, invokeErr := w.exec.Invoke(ctx, delivery.FunctionName, payload)

	errMsg := ""
	if invokeErr != nil {
		errMsg = invokeErr.Error()
	} else if resp == nil {
		errMsg = "empty invocation response"
	} else if resp.Error != "" {
		errMsg = resp.Error
	}

	if errMsg == "" {
		if err := w.store.MarkEventInboxSucceeded(context.Background(), delivery.SubscriptionID, delivery.MessageID, delivery.ID, resp.RequestID, resp.Output, resp.DurationMs, resp.ColdStart); err != nil {
			logging.Op().Error("mark event inbox succeeded failed", "delivery", delivery.ID, "error", err)
			w.retryOrDLQ(delivery, "mark inbox success: "+err.Error())
			return
		}
		if err := w.store.MarkEventDeliverySucceeded(context.Background(), delivery.ID, resp.RequestID, resp.Output, resp.DurationMs, resp.ColdStart); err != nil {
			logging.Op().Error("mark event delivery succeeded failed", "delivery", delivery.ID, "error", err)
			w.retryOrDLQ(delivery, "mark delivery success: "+err.Error())
			return
		}
		logging.Op().Debug("event delivery succeeded", "delivery", delivery.ID, "topic", delivery.TopicName, "subscription", delivery.SubscriptionName, "attempt", delivery.Attempt)
		return
	}

	if err := w.store.MarkEventInboxFailed(context.Background(), delivery.SubscriptionID, delivery.MessageID, delivery.ID, errMsg); err != nil {
		logging.Op().Error("mark event inbox failed", "delivery", delivery.ID, "error", err)
	}
	w.retryOrDLQ(delivery, errMsg)
}

func (w *WorkerPool) retryOrDLQ(delivery *store.EventDelivery, errMsg string) {
	if delivery == nil {
		return
	}
	if delivery.Attempt >= delivery.MaxAttempts {
		if err := w.store.MarkEventDeliveryDLQ(context.Background(), delivery.ID, errMsg); err != nil {
			logging.Op().Error("mark event delivery dlq failed", "delivery", delivery.ID, "error", err)
			return
		}
		logging.Op().Warn("event delivery moved to dlq", "delivery", delivery.ID, "topic", delivery.TopicName, "subscription", delivery.SubscriptionName, "attempt", delivery.Attempt, "max_attempts", delivery.MaxAttempts, "error", errMsg)
		return
	}

	backoff := calcBackoff(delivery.Attempt, delivery.BackoffBaseMS, delivery.BackoffMaxMS)
	nextRun := time.Now().UTC().Add(backoff)
	if err := w.store.MarkEventDeliveryForRetry(context.Background(), delivery.ID, errMsg, nextRun); err != nil {
		logging.Op().Error("mark event delivery retry failed", "delivery", delivery.ID, "error", err)
		return
	}
	logging.Op().Warn("event delivery retry scheduled", "delivery", delivery.ID, "topic", delivery.TopicName, "subscription", delivery.SubscriptionName, "attempt", delivery.Attempt, "next_run_at", nextRun, "error", errMsg)
}

func buildEventInvocationPayload(delivery *store.EventDelivery) (json.RawMessage, error) {
	headers := delivery.Headers
	if len(headers) == 0 {
		headers = json.RawMessage(`{}`)
	}
	eventPayload := delivery.Payload
	if len(eventPayload) == 0 {
		eventPayload = json.RawMessage(`{}`)
	}

	envelope := map[string]any{
		"event": eventPayload,
		"_nova_event": map[string]any{
			"topic": map[string]any{
				"id":   delivery.TopicID,
				"name": delivery.TopicName,
			},
			"subscription": map[string]any{
				"id":             delivery.SubscriptionID,
				"name":           delivery.SubscriptionName,
				"consumer_group": delivery.ConsumerGroup,
			},
			"message": map[string]any{
				"id":           delivery.MessageID,
				"sequence":     delivery.MessageSequence,
				"ordering_key": delivery.OrderingKey,
				"headers":      headers,
			},
		},
	}
	b, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func calcBackoff(attempt, baseMS, maxMS int) time.Duration {
	if baseMS <= 0 {
		baseMS = store.DefaultEventBackoffBaseMS
	}
	if maxMS <= 0 {
		maxMS = store.DefaultEventBackoffMaxMS
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
