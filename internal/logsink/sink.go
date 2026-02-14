// Package logsink defines an abstraction for invocation log persistence.
// By default, invocation logs are written to PostgreSQL. The LogSink interface
// allows routing logs to external systems (ClickHouse, Elasticsearch,
// OpenTelemetry collectors, etc.) to reduce write pressure on the primary
// database.
//
// The executor's log batcher writes through the LogSink interface rather
// than directly to the MetadataStore, enabling pluggable log backends.
package logsink

import (
	"context"

	"github.com/oriys/nova/internal/store"
)

// LogSink abstracts the destination for invocation logs.
// Implementations must be safe for concurrent use.
type LogSink interface {
	// Save persists a single invocation log entry.
	Save(ctx context.Context, log *store.InvocationLog) error

	// SaveBatch persists a batch of invocation log entries.
	// Implementations should use bulk insert for efficiency.
	SaveBatch(ctx context.Context, logs []*store.InvocationLog) error

	// Close releases any resources held by the sink.
	Close() error
}

// PostgresSink writes invocation logs to PostgreSQL via the MetadataStore.
// This is the default sink that preserves the existing behavior.
type PostgresSink struct {
	store *store.Store
}

// NewPostgresSink creates a LogSink backed by PostgreSQL.
func NewPostgresSink(s *store.Store) *PostgresSink {
	return &PostgresSink{store: s}
}

func (s *PostgresSink) Save(ctx context.Context, log *store.InvocationLog) error {
	return s.store.SaveInvocationLog(ctx, log)
}

func (s *PostgresSink) SaveBatch(ctx context.Context, logs []*store.InvocationLog) error {
	return s.store.SaveInvocationLogs(ctx, logs)
}

func (s *PostgresSink) Close() error { return nil }

// MultiSink fans out log writes to multiple sinks. This allows writing
// to PostgreSQL (for query) and an external system (for analytics)
// simultaneously during a migration period.
type MultiSink struct {
	sinks []LogSink
}

// NewMultiSink creates a LogSink that writes to all provided sinks.
// The first error encountered from any sink is returned.
func NewMultiSink(primary LogSink, secondary ...LogSink) *MultiSink {
	sinks := make([]LogSink, 0, 1+len(secondary))
	sinks = append(sinks, primary)
	sinks = append(sinks, secondary...)
	return &MultiSink{sinks: sinks}
}

func (m *MultiSink) Save(ctx context.Context, log *store.InvocationLog) error {
	var firstErr error
	for _, sink := range m.sinks {
		if err := sink.Save(ctx, log); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *MultiSink) SaveBatch(ctx context.Context, logs []*store.InvocationLog) error {
	var firstErr error
	for _, sink := range m.sinks {
		if err := sink.SaveBatch(ctx, logs); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *MultiSink) Close() error {
	var firstErr error
	for _, sink := range m.sinks {
		if err := sink.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NoopSink discards all logs. Useful for testing or when log persistence
// is handled entirely by external observability infrastructure.
type NoopSink struct{}

func NewNoopSink() *NoopSink { return &NoopSink{} }

func (n *NoopSink) Save(_ context.Context, _ *store.InvocationLog) error   { return nil }
func (n *NoopSink) SaveBatch(_ context.Context, _ []*store.InvocationLog) error { return nil }
func (n *NoopSink) Close() error                                                 { return nil }
