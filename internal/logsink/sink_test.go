package logsink

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/oriys/nova/internal/store"
)

func TestNoopSink(t *testing.T) {
	sink := NewNoopSink()

	log := &store.InvocationLog{ID: "test-1"}
	if err := sink.Save(context.Background(), log); err != nil {
		t.Fatalf("NoopSink.Save should not return error: %v", err)
	}

	logs := []*store.InvocationLog{log}
	if err := sink.SaveBatch(context.Background(), logs); err != nil {
		t.Fatalf("NoopSink.SaveBatch should not return error: %v", err)
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("NoopSink.Close should not return error: %v", err)
	}
}

// mockSink records calls for testing
type mockSink struct {
	mu        sync.Mutex
	saved     []*store.InvocationLog
	batches   int
	saveErr   error
	batchErr  error
	closeErr  error
}

func (m *mockSink) Save(_ context.Context, log *store.InvocationLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saved = append(m.saved, log)
	return m.saveErr
}

func (m *mockSink) SaveBatch(_ context.Context, logs []*store.InvocationLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saved = append(m.saved, logs...)
	m.batches++
	return m.batchErr
}

func (m *mockSink) Close() error { return m.closeErr }

func TestMultiSink_FanOut(t *testing.T) {
	primary := &mockSink{}
	secondary := &mockSink{}
	multi := NewMultiSink(primary, secondary)

	log := &store.InvocationLog{ID: "multi-1"}
	if err := multi.Save(context.Background(), log); err != nil {
		t.Fatalf("MultiSink.Save failed: %v", err)
	}

	if len(primary.saved) != 1 {
		t.Fatalf("expected primary to have 1 log, got %d", len(primary.saved))
	}
	if len(secondary.saved) != 1 {
		t.Fatalf("expected secondary to have 1 log, got %d", len(secondary.saved))
	}
}

func TestMultiSink_BatchFanOut(t *testing.T) {
	primary := &mockSink{}
	secondary := &mockSink{}
	multi := NewMultiSink(primary, secondary)

	logs := []*store.InvocationLog{
		{ID: "batch-1"},
		{ID: "batch-2"},
	}
	if err := multi.SaveBatch(context.Background(), logs); err != nil {
		t.Fatalf("MultiSink.SaveBatch failed: %v", err)
	}

	if len(primary.saved) != 2 {
		t.Fatalf("expected primary to have 2 logs, got %d", len(primary.saved))
	}
	if len(secondary.saved) != 2 {
		t.Fatalf("expected secondary to have 2 logs, got %d", len(secondary.saved))
	}
	if primary.batches != 1 || secondary.batches != 1 {
		t.Fatal("expected one batch call per sink")
	}
}

func TestMultiSink_PrimaryError(t *testing.T) {
	errPrimary := errors.New("primary failed")
	primary := &mockSink{saveErr: errPrimary}
	secondary := &mockSink{}
	multi := NewMultiSink(primary, secondary)

	log := &store.InvocationLog{ID: "err-1"}
	err := multi.Save(context.Background(), log)
	if err == nil {
		t.Fatal("expected error from primary sink")
	}
	if err != errPrimary {
		t.Fatalf("expected primary error, got: %v", err)
	}

	// Secondary should still have received the log
	if len(secondary.saved) != 1 {
		t.Fatalf("expected secondary to have 1 log despite primary error, got %d", len(secondary.saved))
	}
}

func TestMultiSink_Close(t *testing.T) {
	errClose := errors.New("close failed")
	primary := &mockSink{closeErr: errClose}
	secondary := &mockSink{}
	multi := NewMultiSink(primary, secondary)

	err := multi.Close()
	if err == nil {
		t.Fatal("expected error from primary close")
	}
	if err != errClose {
		t.Fatalf("expected primary close error, got: %v", err)
	}
}
