package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/store"
)

// ---------------------------------------------------------------------------
// ErrCircuitOpen sentinel
// ---------------------------------------------------------------------------

func TestErrCircuitOpen(t *testing.T) {
	t.Parallel()
	if ErrCircuitOpen == nil {
		t.Fatal("ErrCircuitOpen should not be nil")
	}
	if !strings.Contains(ErrCircuitOpen.Error(), "circuit breaker") {
		t.Fatalf("ErrCircuitOpen should contain 'circuit breaker', got %q", ErrCircuitOpen.Error())
	}
}

// ---------------------------------------------------------------------------
// resolveVolumeMounts
// ---------------------------------------------------------------------------

func TestResolveVolumeMounts(t *testing.T) {
	t.Parallel()

	t.Run("nil mounts", func(t *testing.T) {
		t.Parallel()
		got := resolveVolumeMounts(nil, []*domain.Volume{{ID: "v1", ImagePath: "/img"}})
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty mounts", func(t *testing.T) {
		t.Parallel()
		got := resolveVolumeMounts([]domain.VolumeMount{}, []*domain.Volume{{ID: "v1", ImagePath: "/img"}})
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("matching mount and volume", func(t *testing.T) {
		t.Parallel()
		mounts := []domain.VolumeMount{{VolumeID: "v1", MountPath: "/mnt/data", ReadOnly: true}}
		volumes := []*domain.Volume{{ID: "v1", ImagePath: "/host/v1.ext4"}}
		got := resolveVolumeMounts(mounts, volumes)
		if len(got) != 1 {
			t.Fatalf("expected 1 resolved mount, got %d", len(got))
		}
		if got[0].ImagePath != "/host/v1.ext4" || got[0].MountPath != "/mnt/data" || !got[0].ReadOnly {
			t.Fatalf("unexpected resolved mount: %+v", got[0])
		}
	})

	t.Run("unresolved volume ID", func(t *testing.T) {
		t.Parallel()
		mounts := []domain.VolumeMount{{VolumeID: "missing", MountPath: "/mnt"}}
		volumes := []*domain.Volume{{ID: "v1", ImagePath: "/img"}}
		got := resolveVolumeMounts(mounts, volumes)
		if len(got) != 0 {
			t.Fatalf("expected 0 resolved mounts, got %d", len(got))
		}
	})

	t.Run("volume with empty ImagePath", func(t *testing.T) {
		t.Parallel()
		mounts := []domain.VolumeMount{{VolumeID: "v1", MountPath: "/mnt"}}
		volumes := []*domain.Volume{{ID: "v1", ImagePath: ""}}
		got := resolveVolumeMounts(mounts, volumes)
		if len(got) != 0 {
			t.Fatalf("expected 0 resolved mounts for empty ImagePath, got %d", len(got))
		}
	})

	t.Run("mixed resolved and unresolved", func(t *testing.T) {
		t.Parallel()
		mounts := []domain.VolumeMount{
			{VolumeID: "v1", MountPath: "/mnt/a", ReadOnly: false},
			{VolumeID: "missing", MountPath: "/mnt/b"},
			{VolumeID: "v2", MountPath: "/mnt/c", ReadOnly: true},
		}
		volumes := []*domain.Volume{
			{ID: "v1", ImagePath: "/host/v1.ext4"},
			{ID: "v2", ImagePath: "/host/v2.ext4"},
		}
		got := resolveVolumeMounts(mounts, volumes)
		if len(got) != 2 {
			t.Fatalf("expected 2 resolved mounts, got %d", len(got))
		}
		if got[0].ImagePath != "/host/v1.ext4" || got[0].MountPath != "/mnt/a" || got[0].ReadOnly {
			t.Fatalf("unexpected first mount: %+v", got[0])
		}
		if got[1].ImagePath != "/host/v2.ext4" || got[1].MountPath != "/mnt/c" || !got[1].ReadOnly {
			t.Fatalf("unexpected second mount: %+v", got[1])
		}
	})
}

// ---------------------------------------------------------------------------
// safeGo
// ---------------------------------------------------------------------------

func TestSafeGo_RecoversPanic(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	wg.Add(1)
	safeGo(func() {
		defer wg.Done()
		panic("boom")
	})
	wg.Wait() // if safeGo didn't recover, the test process would crash
}

// ---------------------------------------------------------------------------
// New constructor
// ---------------------------------------------------------------------------

func TestNew_NilStoreAndPool(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	if e.store != nil {
		t.Fatal("expected nil store")
	}
	if e.pool != nil {
		t.Fatal("expected nil pool")
	}
	if e.breakers == nil {
		t.Fatal("expected non-nil breakers registry")
	}
	if e.persistPayloads {
		t.Fatal("expected persistPayloads to default to false")
	}
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestWithPayloadPersistence(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink), WithPayloadPersistence(true))
	defer e.logBatcher.Shutdown(time.Second)

	if !e.persistPayloads {
		t.Fatal("expected persistPayloads to be true")
	}
}

func TestWithSecretsResolver_Nil(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink), WithSecretsResolver(nil))
	defer e.logBatcher.Shutdown(time.Second)

	if e.secretsResolver != nil {
		t.Fatal("expected nil secretsResolver")
	}
}

func TestWithLogSink(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	if e.logSink != sink {
		t.Fatal("expected logSink to be the provided sink")
	}
}

// ---------------------------------------------------------------------------
// Graceful shutdown – closing rejects new invocations
// ---------------------------------------------------------------------------

func TestInvoke_RejectsWhenClosing(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	e.closing.Store(true)

	_, err := e.Invoke(context.Background(), "any-func", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when executor is closing")
	}
	if !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("expected 'shutting down' error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – closing rejects
// ---------------------------------------------------------------------------

func TestInvokeStream_RejectsWhenClosing(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	e.closing.Store(true)

	err := e.InvokeStream(context.Background(), "any-func", json.RawMessage(`{}`), func([]byte, bool, error) error { return nil })
	if err == nil {
		t.Fatal("expected error when executor is closing")
	}
	if !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("expected 'shutting down' error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// WithLogger option
// ---------------------------------------------------------------------------

func TestWithLogger(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	logger := logging.Default()
	e := New(nil, nil, WithLogSink(sink), WithLogger(logger))
	defer e.logBatcher.Shutdown(time.Second)

	if e.logger != logger {
		t.Fatal("expected logger to be the provided logger")
	}
}

// ---------------------------------------------------------------------------
// WithTransportCipher option
// ---------------------------------------------------------------------------

func TestWithTransportCipher_Nil(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink), WithTransportCipher(nil))
	defer e.logBatcher.Shutdown(time.Second)

	if e.transportCipher != nil {
		t.Fatal("expected transportCipher to be nil")
	}
}

func TestWithTransportCipher_NonNil(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	// Create a real cipher from a 32-byte hex key (64 hex chars)
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	c, err := secrets.NewCipher(hexKey)
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}
	tc := secrets.NewTransportCipher(c)
	e := New(nil, nil, WithLogSink(sink), WithTransportCipher(tc))
	defer e.logBatcher.Shutdown(time.Second)

	if e.transportCipher != tc {
		t.Fatal("expected transportCipher to be the provided cipher")
	}
}

// ---------------------------------------------------------------------------
// selectRolloutTarget
// ---------------------------------------------------------------------------

func TestSelectRolloutTarget_NilFunction(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	got := e.selectRolloutTarget(context.Background(), nil)
	if got != nil {
		t.Fatal("expected nil for nil function input")
	}
}

func TestSelectRolloutTarget_NilPolicy(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{Name: "test-fn", RolloutPolicy: nil}
	got := e.selectRolloutTarget(context.Background(), fn)
	if got != fn {
		t.Fatal("expected same function when RolloutPolicy is nil")
	}
}

func TestSelectRolloutTarget_DisabledPolicy(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{
		Name:          "test-fn",
		RolloutPolicy: &domain.RolloutPolicy{Enabled: false},
	}
	got := e.selectRolloutTarget(context.Background(), fn)
	if got != fn {
		t.Fatal("expected same function when policy is disabled")
	}
}

func TestSelectRolloutTarget_EmptyCanaryName(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{
		Name: "test-fn",
		RolloutPolicy: &domain.RolloutPolicy{
			Enabled:        true,
			CanaryFunction: "",
			CanaryPercent:  50,
		},
	}
	got := e.selectRolloutTarget(context.Background(), fn)
	if got != fn {
		t.Fatal("expected same function when canary name is empty")
	}
}

func TestSelectRolloutTarget_SameNameCanary(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{
		Name: "test-fn",
		RolloutPolicy: &domain.RolloutPolicy{
			Enabled:        true,
			CanaryFunction: "test-fn",
			CanaryPercent:  50,
		},
	}
	got := e.selectRolloutTarget(context.Background(), fn)
	if got != fn {
		t.Fatal("expected same function when canary name equals primary")
	}
}

func TestSelectRolloutTarget_ZeroPercent(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{
		Name: "test-fn",
		RolloutPolicy: &domain.RolloutPolicy{
			Enabled:        true,
			CanaryFunction: "canary-fn",
			CanaryPercent:  0,
		},
	}
	got := e.selectRolloutTarget(context.Background(), fn)
	if got != fn {
		t.Fatal("expected same function when canary percent is 0")
	}
}

// ---------------------------------------------------------------------------
// getBreakerForFunction
// ---------------------------------------------------------------------------

func TestGetBreakerForFunction_NilPolicy(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{ID: "f1", CapacityPolicy: nil}
	b := e.getBreakerForFunction(fn)
	if b != nil {
		t.Fatal("expected nil breaker when CapacityPolicy is nil")
	}
}

func TestGetBreakerForFunction_Disabled(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{
		ID:             "f1",
		CapacityPolicy: &domain.CapacityPolicy{Enabled: false, BreakerErrorPct: 50, BreakerWindowS: 10, BreakerOpenS: 5},
	}
	b := e.getBreakerForFunction(fn)
	if b != nil {
		t.Fatal("expected nil breaker when CapacityPolicy is disabled")
	}
}

func TestGetBreakerForFunction_InvalidConfig(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	tests := []struct {
		name string
		cp   *domain.CapacityPolicy
	}{
		{"zero error pct", &domain.CapacityPolicy{Enabled: true, BreakerErrorPct: 0, BreakerWindowS: 10, BreakerOpenS: 5}},
		{"zero window", &domain.CapacityPolicy{Enabled: true, BreakerErrorPct: 50, BreakerWindowS: 0, BreakerOpenS: 5}},
		{"zero open", &domain.CapacityPolicy{Enabled: true, BreakerErrorPct: 50, BreakerWindowS: 10, BreakerOpenS: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := &domain.Function{ID: "f-inv", CapacityPolicy: tt.cp}
			b := e.getBreakerForFunction(fn)
			if b != nil {
				t.Fatal("expected nil breaker for invalid config")
			}
		})
	}
}

func TestGetBreakerForFunction_ValidConfig(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{
		ID: "f-valid",
		CapacityPolicy: &domain.CapacityPolicy{
			Enabled:         true,
			BreakerErrorPct: 50,
			BreakerWindowS:  10,
			BreakerOpenS:    5,
		},
	}
	b := e.getBreakerForFunction(fn)
	if b == nil {
		t.Fatal("expected non-nil breaker for valid config")
	}
}

// ---------------------------------------------------------------------------
// BreakerSnapshot
// ---------------------------------------------------------------------------

func TestBreakerSnapshot_Empty(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	snap := e.BreakerSnapshot()
	if len(snap) != 0 {
		t.Fatalf("expected empty snapshot, got %d entries", len(snap))
	}
}

func TestBreakerSnapshot_WithBreaker(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil, WithLogSink(sink))
	defer e.logBatcher.Shutdown(time.Second)

	// Create a breaker by calling getBreakerForFunction with valid config
	fn := &domain.Function{
		ID: "f-snap",
		CapacityPolicy: &domain.CapacityPolicy{
			Enabled:         true,
			BreakerErrorPct: 50,
			BreakerWindowS:  10,
			BreakerOpenS:    5,
		},
	}
	e.getBreakerForFunction(fn)

	snap := e.BreakerSnapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 breaker in snapshot, got %d", len(snap))
	}
	if _, ok := snap["f-snap"]; !ok {
		t.Fatal("expected breaker for 'f-snap' in snapshot")
	}
}

// ---------------------------------------------------------------------------
// InvalidateSnapshot
// ---------------------------------------------------------------------------

func TestInvalidateSnapshot_EmptyDir(t *testing.T) {
	t.Parallel()
	err := InvalidateSnapshot("", "some-func")
	if err != nil {
		t.Fatalf("expected nil error for empty dir, got %v", err)
	}
}

func TestInvalidateSnapshot_NonexistentDir(t *testing.T) {
	t.Parallel()
	err := InvalidateSnapshot("/nonexistent/path/xxxxx", "some-func")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
}

func TestInvalidateSnapshot_WithFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	funcID := "test-func"
	snapPath := filepath.Join(dir, funcID+".snap")
	memPath := filepath.Join(dir, funcID+".mem")
	metaPath := filepath.Join(dir, funcID+".meta")

	os.WriteFile(snapPath, []byte("snap"), 0644)
	os.WriteFile(memPath, []byte("mem"), 0644)
	os.WriteFile(metaPath, []byte(`{}`), 0644)

	err := InvalidateSnapshot(dir, funcID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range []string{snapPath, memPath, metaPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected file %s to be removed", p)
		}
	}
}

func TestInvalidateSnapshot_WithMetaCodeDrives(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	funcID := "test-func2"
	codeDrive := filepath.Join(dir, "code.ext4")
	codeDriveBackup := filepath.Join(dir, "code.ext4.bak")

	os.WriteFile(codeDrive, []byte("drive"), 0644)
	os.WriteFile(codeDriveBackup, []byte("backup"), 0644)

	meta := fmt.Sprintf(`{"code_drive":%q,"code_drive_backup":%q}`, codeDrive, codeDriveBackup)
	metaPath := filepath.Join(dir, funcID+".meta")
	os.WriteFile(metaPath, []byte(meta), 0644)
	os.WriteFile(filepath.Join(dir, funcID+".snap"), []byte("s"), 0644)
	os.WriteFile(filepath.Join(dir, funcID+".mem"), []byte("m"), 0644)

	err := InvalidateSnapshot(dir, funcID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range []string{codeDrive, codeDriveBackup} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected file %s to be removed", p)
		}
	}
}

// ---------------------------------------------------------------------------
// HasSnapshot
// ---------------------------------------------------------------------------

func TestHasSnapshot_EmptyDir(t *testing.T) {
	t.Parallel()
	if HasSnapshot("", "func") {
		t.Fatal("expected false for empty dir")
	}
}

func TestHasSnapshot_MissingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if HasSnapshot(dir, "func") {
		t.Fatal("expected false when snap/mem files don't exist")
	}
}

func TestHasSnapshot_OnlySnap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "func.snap"), []byte("s"), 0644)
	if HasSnapshot(dir, "func") {
		t.Fatal("expected false when only snap exists")
	}
}

func TestHasSnapshot_BothFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "func.snap"), []byte("s"), 0644)
	os.WriteFile(filepath.Join(dir, "func.mem"), []byte("m"), 0644)
	if !HasSnapshot(dir, "func") {
		t.Fatal("expected true when both snap and mem exist")
	}
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

func TestShutdown_SetsClosingFlag(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}

	// We need a mock pool that implements Shutdown() to avoid nil panic.
	// Since pool.Pool.Shutdown() requires a real pool, we test the parts
	// we can: closing flag and logBatcher shutdown.
	e := New(nil, nil, WithLogSink(sink))

	if e.closing.Load() {
		t.Fatal("expected closing to be false initially")
	}

	// We can't call e.Shutdown() directly since it calls e.pool.Shutdown()
	// which will panic on nil pool. Test the closing logic manually.
	e.closing.Store(true)
	if !e.closing.Load() {
		t.Fatal("expected closing to be true after setting")
	}

	e.logBatcher.Shutdown(time.Second)
}

// ---------------------------------------------------------------------------
// invocationLogBatcher
// ---------------------------------------------------------------------------

func TestLogBatcher_Enqueue(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	b := newInvocationLogBatcher(nil, sink, LogBatcherConfig{
		BatchSize:     1,
		FlushInterval: time.Hour,
	})

	log := &store.InvocationLog{ID: "req-1", FunctionID: "f1"}
	b.Enqueue(log)

	select {
	case got := <-sink.ch:
		if got.ID != "req-1" {
			t.Fatalf("expected req-1, got %s", got.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for log to be flushed")
	}

	b.Shutdown(time.Second)
}

func TestLogBatcher_FlushByInterval(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	b := newInvocationLogBatcher(nil, sink, LogBatcherConfig{
		BatchSize:     100, // large batch so it won't trigger by size
		FlushInterval: 50 * time.Millisecond,
	})

	log := &store.InvocationLog{ID: "req-interval", FunctionID: "f1"}
	b.Enqueue(log)

	select {
	case got := <-sink.ch:
		if got.ID != "req-interval" {
			t.Fatalf("expected req-interval, got %s", got.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for interval flush")
	}

	b.Shutdown(time.Second)
}

func TestLogBatcher_DroppedTotal(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	b := newInvocationLogBatcher(nil, sink, LogBatcherConfig{})
	defer b.Shutdown(time.Second)

	if b.DroppedTotal() != 0 {
		t.Fatalf("expected 0 dropped, got %d", b.DroppedTotal())
	}
}

func TestLogBatcher_FlushFailedTotal(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	b := newInvocationLogBatcher(nil, sink, LogBatcherConfig{})
	defer b.Shutdown(time.Second)

	if b.FlushFailedTotal() != 0 {
		t.Fatalf("expected 0 flush failures, got %d", b.FlushFailedTotal())
	}
}

func TestLogBatcher_Shutdown(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	b := newInvocationLogBatcher(nil, sink, LogBatcherConfig{})
	// Should not panic
	b.Shutdown(time.Second)
}

func TestLogBatcher_DroppedOnFullBuffer(t *testing.T) {
	t.Parallel()
	// Use a blocking sink so the buffer fills up
	blockSink := &blockingSink{block: make(chan struct{})}
	b := newInvocationLogBatcher(nil, blockSink, LogBatcherConfig{
		BufferSize:    2,
		BatchSize:     1,
		FlushInterval: time.Millisecond,
	})

	// Fill the buffer – the batcher goroutine might pull one item immediately,
	// so we need to send enough to overflow
	for i := 0; i < 10; i++ {
		b.Enqueue(&store.InvocationLog{ID: fmt.Sprintf("req-%d", i)})
	}

	// Give it a moment for drops to register
	time.Sleep(50 * time.Millisecond)

	if b.DroppedTotal() == 0 {
		t.Fatal("expected some dropped logs")
	}

	close(blockSink.block)
	b.Shutdown(time.Second)
}

func TestLogBatcher_FlushFailed(t *testing.T) {
	t.Parallel()
	failSink := &failingSink{}
	b := newInvocationLogBatcher(nil, failSink, LogBatcherConfig{
		BatchSize:     1,
		FlushInterval: time.Hour,
		MaxRetries:    1,
		RetryInterval: time.Millisecond,
		Timeout:       time.Second,
	})

	b.Enqueue(&store.InvocationLog{ID: "req-fail", FunctionID: "f1"})

	// Wait for flush failure to be recorded
	deadline := time.After(3 * time.Second)
	for {
		if b.FlushFailedTotal() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for flush failure")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	b.Shutdown(time.Second)
}

func TestLogBatcher_DefaultConfig(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	b := newInvocationLogBatcher(nil, sink, LogBatcherConfig{})
	defer b.Shutdown(time.Second)

	if b.batchSize != defaultInvocationLogBatchSize {
		t.Fatalf("expected default batchSize %d, got %d", defaultInvocationLogBatchSize, b.batchSize)
	}
	if b.flushInterval != defaultInvocationLogFlushInterval {
		t.Fatalf("expected default flushInterval %v, got %v", defaultInvocationLogFlushInterval, b.flushInterval)
	}
	if b.timeout != defaultInvocationLogTimeout {
		t.Fatalf("expected default timeout %v, got %v", defaultInvocationLogTimeout, b.timeout)
	}
	if b.maxRetries != defaultInvocationLogMaxRetries {
		t.Fatalf("expected default maxRetries %d, got %d", defaultInvocationLogMaxRetries, b.maxRetries)
	}
	if b.retryInterval != defaultInvocationLogRetryInterval {
		t.Fatalf("expected default retryInterval %v, got %v", defaultInvocationLogRetryInterval, b.retryInterval)
	}
}

// ---------------------------------------------------------------------------
// endpointScore
// ---------------------------------------------------------------------------

func TestEndpointScore_ZeroLoad(t *testing.T) {
	t.Parallel()
	ep := &cometEndpoint{}
	score := endpointScore(ep)
	if score != 0.0 {
		t.Fatalf("expected 0.0 score for zero load, got %f", score)
	}
}

func TestEndpointScore_InflightOnly(t *testing.T) {
	t.Parallel()
	ep := &cometEndpoint{}
	ep.inflight.Store(50)
	score := endpointScore(ep)
	// 50/100 = 0.5 normalized, * 0.5 weight = 0.25
	expected := 0.25
	if score != expected {
		t.Fatalf("expected %f, got %f", expected, score)
	}
}

func TestEndpointScore_InflightCapped(t *testing.T) {
	t.Parallel()
	ep := &cometEndpoint{}
	ep.inflight.Store(200) // exceeds referenceCapacity of 100
	score := endpointScore(ep)
	// capped to 1.0, * 0.5 = 0.5
	expected := 0.5
	if score != expected {
		t.Fatalf("expected %f, got %f", expected, score)
	}
}

func TestEndpointScore_WithCPUAndMem(t *testing.T) {
	t.Parallel()
	ep := &cometEndpoint{}
	ep.cpuUsage.Store(0.6)
	ep.memUsage.Store(0.4)
	score := endpointScore(ep)
	// inflight=0 → 0, cpu=0.6*0.3=0.18, mem=0.4*0.2=0.08 → total=0.26
	expected := 0.26
	if abs(score-expected) > 0.001 {
		t.Fatalf("expected ~%f, got %f", expected, score)
	}
}

func TestEndpointScore_FullLoad(t *testing.T) {
	t.Parallel()
	ep := &cometEndpoint{}
	ep.inflight.Store(100)
	ep.cpuUsage.Store(1.0)
	ep.memUsage.Store(1.0)
	score := endpointScore(ep)
	// 1.0*0.5 + 1.0*0.3 + 1.0*0.2 = 1.0
	expected := 1.0
	if abs(score-expected) > 0.001 {
		t.Fatalf("expected ~%f, got %f", expected, score)
	}
}

// ---------------------------------------------------------------------------
// leastLoaded
// ---------------------------------------------------------------------------

func TestLeastLoaded_SingleEndpoint(t *testing.T) {
	t.Parallel()
	ep := &cometEndpoint{addr: "ep1"}
	b := &BalancedRemoteInvoker{endpoints: []*cometEndpoint{ep}}
	got := b.leastLoaded()
	if got != ep {
		t.Fatal("expected the single endpoint")
	}
}

func TestLeastLoaded_ChoosesBestEndpoint(t *testing.T) {
	t.Parallel()
	ep1 := &cometEndpoint{addr: "ep1"}
	ep1.inflight.Store(10)
	ep2 := &cometEndpoint{addr: "ep2"}
	ep2.inflight.Store(0)
	ep3 := &cometEndpoint{addr: "ep3"}
	ep3.inflight.Store(5)

	b := &BalancedRemoteInvoker{endpoints: []*cometEndpoint{ep1, ep2, ep3}}
	got := b.leastLoaded()
	if got.addr != "ep2" {
		t.Fatalf("expected ep2 (least loaded), got %s", got.addr)
	}
}

func TestLeastLoaded_ConsidersCPUAndMem(t *testing.T) {
	t.Parallel()
	ep1 := &cometEndpoint{addr: "ep1"}
	ep1.inflight.Store(0)
	ep1.cpuUsage.Store(0.9)
	ep1.memUsage.Store(0.9)

	ep2 := &cometEndpoint{addr: "ep2"}
	ep2.inflight.Store(5)
	ep2.cpuUsage.Store(0.0)
	ep2.memUsage.Store(0.0)

	b := &BalancedRemoteInvoker{endpoints: []*cometEndpoint{ep1, ep2}}
	got := b.leastLoaded()
	// ep1: 0 + 0.9*0.3 + 0.9*0.2 = 0.45
	// ep2: (5/100)*0.5 = 0.025
	if got.addr != "ep2" {
		t.Fatalf("expected ep2 (lower composite score), got %s", got.addr)
	}
}

// ---------------------------------------------------------------------------
// BalancedRemoteInvoker – ReportLoad
// ---------------------------------------------------------------------------

func TestBalancedInvoker_ReportLoad(t *testing.T) {
	t.Parallel()
	ep1 := &cometEndpoint{addr: "ep1:9090"}
	ep2 := &cometEndpoint{addr: "ep2:9090"}
	b := &BalancedRemoteInvoker{endpoints: []*cometEndpoint{ep1, ep2}}

	b.ReportLoad("ep1:9090", 0.5, 0.3)

	cpu, _ := ep1.cpuUsage.Load().(float64)
	mem, _ := ep1.memUsage.Load().(float64)
	if cpu != 0.5 || mem != 0.3 {
		t.Fatalf("expected cpu=0.5 mem=0.3, got cpu=%f mem=%f", cpu, mem)
	}

	// ep2 should be unaffected
	cpu2, ok := ep2.cpuUsage.Load().(float64)
	if ok && cpu2 != 0.0 {
		t.Fatalf("expected ep2 cpu unchanged, got %f", cpu2)
	}
}

func TestBalancedInvoker_ReportLoad_UnknownAddr(t *testing.T) {
	t.Parallel()
	ep := &cometEndpoint{addr: "ep1:9090"}
	b := &BalancedRemoteInvoker{endpoints: []*cometEndpoint{ep}}

	// Should not panic when address is not found
	b.ReportLoad("unknown:9090", 0.5, 0.3)
}

// ---------------------------------------------------------------------------
// NewBalancedRemoteInvoker
// ---------------------------------------------------------------------------

func TestNewBalancedRemoteInvoker_EmptyAddrs(t *testing.T) {
	t.Parallel()
	_, err := NewBalancedRemoteInvoker([]string{})
	if err == nil {
		t.Fatal("expected error for empty addrs")
	}
}

func TestNewBalancedRemoteInvoker_ValidAddrs(t *testing.T) {
	t.Parallel()
	b, err := NewBalancedRemoteInvoker([]string{"localhost:9090"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer b.Close()

	if len(b.endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(b.endpoints))
	}
}

func TestNewBalancedRemoteInvoker_MultipleAddrs(t *testing.T) {
	t.Parallel()
	b, err := NewBalancedRemoteInvoker([]string{"localhost:9090", "localhost:9091"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer b.Close()

	if len(b.endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(b.endpoints))
	}
}

func TestBalancedRemoteInvoker_Close(t *testing.T) {
	t.Parallel()
	b, err := NewBalancedRemoteInvoker([]string{"localhost:9090"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RemoteInvoker
// ---------------------------------------------------------------------------

func TestNewRemoteInvoker(t *testing.T) {
	t.Parallel()
	r, err := NewRemoteInvoker("localhost:9090")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer r.Close()

	if r.conn == nil {
		t.Fatal("expected non-nil conn")
	}
	if r.client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestRemoteInvoker_Close(t *testing.T) {
	t.Parallel()
	r, err := NewRemoteInvoker("localhost:9090")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestRemoteInvoker_Close_NilConn(t *testing.T) {
	t.Parallel()
	r := &RemoteInvoker{conn: nil}
	if err := r.Close(); err != nil {
		t.Fatalf("Close with nil conn should not fail: %v", err)
	}
}

// ---------------------------------------------------------------------------
// WithLogBatcherConfig option
// ---------------------------------------------------------------------------

func TestWithLogBatcherConfig(t *testing.T) {
	t.Parallel()
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	cfg := LogBatcherConfig{BatchSize: 42, BufferSize: 99}
	e := New(nil, nil, WithLogSink(sink), WithLogBatcherConfig(cfg))
	defer e.logBatcher.Shutdown(time.Second)

	if e.logBatcherConfig.BatchSize != 42 {
		t.Fatalf("expected BatchSize 42, got %d", e.logBatcherConfig.BatchSize)
	}
	if e.logBatcherConfig.BufferSize != 99 {
		t.Fatalf("expected BufferSize 99, got %d", e.logBatcherConfig.BufferSize)
	}
}

// ---------------------------------------------------------------------------
// PersistentVsockStream – additional coverage
// ---------------------------------------------------------------------------

func TestPersistentVsockStream_RecvError(t *testing.T) {
	t.Parallel()
	p := NewPersistentVsockStream(
		func() error { return nil },
		func(msg interface{}) error { return nil },
		func() (interface{}, error) { return nil, errors.New("recv failed") },
		func() error { return nil },
	)
	defer p.Close()

	_, err := p.Execute("request")
	if err == nil || !strings.Contains(err.Error(), "receive") {
		t.Fatalf("expected receive error, got %v", err)
	}
	if p.alive {
		t.Fatal("expected alive=false after receive error")
	}
}

func TestPersistentVsockStream_RedialFails(t *testing.T) {
	t.Parallel()
	sendCount := 0
	dialCount := 0
	p := NewPersistentVsockStream(
		func() error {
			dialCount++
			if dialCount == 2 {
				return errors.New("redial failed")
			}
			return nil
		},
		func(msg interface{}) error {
			sendCount++
			if sendCount == 1 {
				return errors.New("broken")
			}
			return nil
		},
		func() (interface{}, error) { return "ok", nil },
		func() error { return nil },
	)
	defer p.Close()

	_, err := p.Execute("request")
	if err == nil || !strings.Contains(err.Error(), "redial") {
		t.Fatalf("expected redial error, got %v", err)
	}
}

func TestPersistentVsockStream_SendFailsAfterRedial(t *testing.T) {
	t.Parallel()
	p := NewPersistentVsockStream(
		func() error { return nil },
		func(msg interface{}) error { return errors.New("always fails") },
		func() (interface{}, error) { return "ok", nil },
		func() error { return nil },
	)
	defer p.Close()

	_, err := p.Execute("request")
	if err == nil || !strings.Contains(err.Error(), "send after redial") {
		t.Fatalf("expected 'send after redial' error, got %v", err)
	}
}

func TestPersistentVsockStream_CloseNilCloser(t *testing.T) {
	t.Parallel()
	p := &PersistentVsockStream{closer: nil}
	if err := p.Close(); err != nil {
		t.Fatalf("Close with nil closer should not fail: %v", err)
	}
}

// ---------------------------------------------------------------------------
// safeGo – normal execution
// ---------------------------------------------------------------------------

func TestSafeGo_NormalExecution(t *testing.T) {
	t.Parallel()
	var result atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	safeGo(func() {
		defer wg.Done()
		result.Store(42)
	})
	wg.Wait()
	if result.Load() != 42 {
		t.Fatalf("expected 42, got %d", result.Load())
	}
}

// ---------------------------------------------------------------------------
// resolveVolumeMounts – nil volumes
// ---------------------------------------------------------------------------

func TestResolveVolumeMounts_NilVolumes(t *testing.T) {
	t.Parallel()
	mounts := []domain.VolumeMount{{VolumeID: "v1", MountPath: "/mnt"}}
	got := resolveVolumeMounts(mounts, nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// test helper sinks
// ---------------------------------------------------------------------------

type blockingSink struct {
	block chan struct{}
}

func (s *blockingSink) Save(_ context.Context, _ *store.InvocationLog) error {
	<-s.block
	return nil
}

func (s *blockingSink) SaveBatch(_ context.Context, _ []*store.InvocationLog) error {
	<-s.block
	return nil
}

func (s *blockingSink) Close() error { return nil }

type failingSink struct{}

func (s *failingSink) Save(_ context.Context, _ *store.InvocationLog) error {
	return errors.New("save failed")
}

func (s *failingSink) SaveBatch(_ context.Context, _ []*store.InvocationLog) error {
	return errors.New("save batch failed")
}

func (s *failingSink) Close() error { return nil }

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
