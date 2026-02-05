package executor

import (
	"context"
	"log/slog"
	"time"

	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

const (
	defaultInvocationLogBatchSize     = 100
	defaultInvocationLogBufferSize    = 1000
	defaultInvocationLogFlushInterval = 500 * time.Millisecond
	defaultInvocationLogTimeout       = 5 * time.Second
)

// LogBatcherConfig holds configuration for the invocation log batcher
type LogBatcherConfig struct {
	BatchSize     int
	BufferSize    int
	FlushInterval time.Duration
	Timeout       time.Duration
}

type invocationLogBatcher struct {
	store         *store.Store
	logger        *slog.Logger
	logs          chan *store.InvocationLog
	flushInterval time.Duration
	batchSize     int
	timeout       time.Duration
	done          chan struct{}
}

func newInvocationLogBatcher(s *store.Store, cfg LogBatcherConfig) *invocationLogBatcher {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = defaultInvocationLogBatchSize
	}
	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = defaultInvocationLogBufferSize
	}
	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultInvocationLogFlushInterval
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultInvocationLogTimeout
	}

	b := &invocationLogBatcher{
		store:         s,
		logger:        logging.Op(),
		logs:          make(chan *store.InvocationLog, bufferSize),
		flushInterval: flushInterval,
		batchSize:     batchSize,
		timeout:       timeout,
		done:          make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *invocationLogBatcher) Enqueue(log *store.InvocationLog) {
	select {
	case b.logs <- log:
	default:
		b.logger.Warn("dropping invocation log due to full buffer", "request_id", log.ID, "function_id", log.FunctionID)
	}
}

func (b *invocationLogBatcher) Shutdown(timeout time.Duration) {
	close(b.logs)
	select {
	case <-b.done:
		return
	case <-time.After(timeout):
		b.logger.Warn("timeout waiting for invocation log batcher shutdown", "timeout", timeout)
	}
}

func (b *invocationLogBatcher) run() {
	defer close(b.done)

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	batch := make([]*store.InvocationLog, 0, b.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		if err := b.store.SaveInvocationLogs(ctx, batch); err != nil {
			b.logger.Warn("failed to persist invocation logs", "error", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case log, ok := <-b.logs:
			if !ok {
				flush()
				return
			}
			batch = append(batch, log)
			if len(batch) >= b.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
