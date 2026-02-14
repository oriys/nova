package executor

import (
	"context"
	"log/slog"
	"time"

	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/logsink"
	"github.com/oriys/nova/internal/store"
)

const (
	defaultInvocationLogBatchSize     = 100
	defaultInvocationLogBufferSize    = 1000
	defaultInvocationLogFlushInterval = 500 * time.Millisecond
	defaultInvocationLogTimeout       = 5 * time.Second
	defaultInvocationLogMaxRetries    = 3
	defaultInvocationLogRetryInterval = 100 * time.Millisecond
)

// LogBatcherConfig holds configuration for the invocation log batcher
type LogBatcherConfig struct {
	BatchSize     int
	BufferSize    int
	FlushInterval time.Duration
	Timeout       time.Duration
	MaxRetries    int
	RetryInterval time.Duration
}

type invocationLogBatcher struct {
	sink          logsink.LogSink
	logger        *slog.Logger
	logs          chan *store.InvocationLog
	flushInterval time.Duration
	batchSize     int
	timeout       time.Duration
	maxRetries    int
	retryInterval time.Duration
	done          chan struct{}
}

func newInvocationLogBatcher(s *store.Store, sink logsink.LogSink, cfg LogBatcherConfig) *invocationLogBatcher {
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
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultInvocationLogMaxRetries
	}
	retryInterval := cfg.RetryInterval
	if retryInterval <= 0 {
		retryInterval = defaultInvocationLogRetryInterval
	}

	b := &invocationLogBatcher{
		sink:          sink,
		logger:        logging.Op(),
		logs:          make(chan *store.InvocationLog, bufferSize),
		flushInterval: flushInterval,
		batchSize:     batchSize,
		timeout:       timeout,
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
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
		var lastErr error
		for attempt := 0; attempt < b.maxRetries; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
			lastErr = b.sink.SaveBatch(ctx, batch)
			cancel()
			if lastErr == nil {
				break
			}
			b.logger.Warn("failed to persist invocation logs, retrying",
				"error", lastErr, "count", len(batch), "attempt", attempt+1)
			time.Sleep(time.Duration(1<<uint(attempt)) * b.retryInterval)
		}
		if lastErr != nil {
			b.logger.Error("permanently failed to persist invocation logs after retries",
				"error", lastErr, "count", len(batch))
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
