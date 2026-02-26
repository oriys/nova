package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// --- mock invoker ---

type mockInvocation struct {
	funcName string
	payload  json.RawMessage
}

type mockInvoker struct {
	invocations []mockInvocation
	mu          sync.Mutex
	err         error
}

func (m *mockInvoker) Invoke(_ context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invocations = append(m.invocations, mockInvocation{funcName: funcName, payload: payload})
	if m.err != nil {
		return nil, m.err
	}
	return &domain.InvokeResponse{RequestID: "test-req"}, nil
}

// --- helpers ---

func newTestScheduler(t *testing.T) (*Scheduler, *mockInvoker) {
	t.Helper()
	inv := &mockInvoker{}
	s := New(nil, inv)
	return s, inv
}

func makeSchedule(id, fnName, cronExpr string) *store.Schedule {
	return &store.Schedule{
		ID:           id,
		FunctionName: fnName,
		CronExpr:     cronExpr,
		Enabled:      true,
	}
}

// --- tests ---

func TestNew(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.cron == nil {
		t.Fatal("expected non-nil cron instance")
	}
	if s.entries == nil {
		t.Fatal("expected non-nil entries map")
	}
}

func TestAdd_ValidCron(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t)
	defer s.Stop()

	err := s.Add(makeSchedule("s1", "myFunc", "@every 1s"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	s.mu.Lock()
	_, ok := s.entries["s1"]
	s.mu.Unlock()
	if !ok {
		t.Fatal("expected entry to be registered")
	}
}

func TestAdd_InvalidCron(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t)
	defer s.Stop()

	err := s.Add(makeSchedule("s1", "myFunc", "not-a-cron"))
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

func TestAdd_ReplaceExisting(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t)
	defer s.Stop()

	if err := s.Add(makeSchedule("s1", "funcA", "@every 2s")); err != nil {
		t.Fatalf("first add failed: %v", err)
	}

	s.mu.Lock()
	firstEntry := s.entries["s1"]
	s.mu.Unlock()

	if err := s.Add(makeSchedule("s1", "funcB", "@every 3s")); err != nil {
		t.Fatalf("second add failed: %v", err)
	}

	s.mu.Lock()
	secondEntry := s.entries["s1"]
	s.mu.Unlock()

	if firstEntry == secondEntry {
		t.Fatal("expected entry ID to change after replacement")
	}
}

func TestRemove_Existing(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t)
	defer s.Stop()

	if err := s.Add(makeSchedule("s1", "myFunc", "@every 1s")); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	s.Remove("s1")

	s.mu.Lock()
	_, ok := s.entries["s1"]
	s.mu.Unlock()
	if ok {
		t.Fatal("expected entry to be removed")
	}
}

func TestRemove_NonExistent(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t)
	defer s.Stop()

	// Should not panic
	s.Remove("does-not-exist")
}

func TestStop(t *testing.T) {
	t.Parallel()
	s, _ := newTestScheduler(t)
	// Should not panic on a fresh (not started) scheduler
	s.Stop()
}

func TestStart_NilStore(t *testing.T) {
	t.Parallel()
	inv := &mockInvoker{}
	s := New(nil, inv)

	err := s.Start()
	if err == nil {
		t.Fatal("expected error when store is nil")
	}
	want := "schedule store not configured"
	if err.Error() != want {
		t.Fatalf("got error %q, want %q", err.Error(), want)
	}
}

func TestStart_NilScheduleStore(t *testing.T) {
	t.Parallel()
	inv := &mockInvoker{}
	st := &store.Store{} // ScheduleStore is nil
	s := New(st, inv)

	err := s.Start()
	if err == nil {
		t.Fatal("expected error when ScheduleStore is nil")
	}
	want := "schedule store not configured"
	if err.Error() != want {
		t.Fatalf("got error %q, want %q", err.Error(), want)
	}
}

// --- mock schedule store ---

type mockScheduleStore struct {
	schedules       []*store.Schedule
	listErr         error
	updateErr       error
	lastRunID       string
	lastRunMu       sync.Mutex
	tryLockErr      error
	tryLockReturnFn func() bool // If set, use this to control lock success
}

func (m *mockScheduleStore) SaveSchedule(ctx context.Context, s *store.Schedule) error {
	return nil
}
func (m *mockScheduleStore) GetSchedule(ctx context.Context, id string) (*store.Schedule, error) {
	return nil, nil
}
func (m *mockScheduleStore) ListSchedulesByFunction(ctx context.Context, fn string, limit, offset int) ([]*store.Schedule, error) {
	return nil, nil
}
func (m *mockScheduleStore) ListAllSchedules(ctx context.Context, limit, offset int) ([]*store.Schedule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.schedules, nil
}
func (m *mockScheduleStore) DeleteSchedule(ctx context.Context, id string) error { return nil }
func (m *mockScheduleStore) UpdateScheduleLastRun(ctx context.Context, id string, lastRun time.Time) error {
	m.lastRunMu.Lock()
	defer m.lastRunMu.Unlock()
	m.lastRunID = id
	return m.updateErr
}
func (m *mockScheduleStore) UpdateScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	return nil
}
func (m *mockScheduleStore) UpdateScheduleCron(ctx context.Context, id string, cronExpr string) error {
	return nil
}
func (m *mockScheduleStore) TryLockSchedule(ctx context.Context, id string) (bool, error) {
	if m.tryLockErr != nil {
		return false, m.tryLockErr
	}
	// If a custom return function is set, use it to determine lock success
	if m.tryLockReturnFn != nil {
		return m.tryLockReturnFn(), nil
	}
	// Default: lock succeeds
	return true, nil
}

// --- Start() tests ---

func TestStart_WithMockStore(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{
		schedules: []*store.Schedule{
			{ID: "s1", FunctionName: "fn1", CronExpr: "@every 1m", Enabled: true},
			{ID: "s2", FunctionName: "fn2", CronExpr: "@every 2m", Enabled: false},
			{ID: "s3", FunctionName: "fn3", CronExpr: "@every 3m", Enabled: true},
		},
	}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)
	defer s.Stop()

	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries["s1"]; !ok {
		t.Error("expected enabled schedule s1 to be registered")
	}
	if _, ok := s.entries["s2"]; ok {
		t.Error("expected disabled schedule s2 to NOT be registered")
	}
	if _, ok := s.entries["s3"]; !ok {
		t.Error("expected enabled schedule s3 to be registered")
	}
}

func TestStart_StoreError(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{
		listErr: fmt.Errorf("db connection failed"),
	}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)

	err := s.Start()
	if err == nil {
		t.Fatal("expected error from Start()")
	}
	if err.Error() != "db connection failed" {
		t.Fatalf("got error %q, want %q", err.Error(), "db connection failed")
	}
}

func TestStart_EmptySchedules(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{schedules: []*store.Schedule{}}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)
	defer s.Stop()

	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	s.mu.Lock()
	count := len(s.entries)
	s.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 entries, got %d", count)
	}
}

// --- invoke() tests ---

func TestInvoke_Success(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)

	s.invoke("sched-1", "tenant-1", "ns-1", "myFunc", json.RawMessage(`{"key":"val"}`))

	inv.mu.Lock()
	defer inv.mu.Unlock()
	if len(inv.invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(inv.invocations))
	}
	if inv.invocations[0].funcName != "myFunc" {
		t.Errorf("expected funcName %q, got %q", "myFunc", inv.invocations[0].funcName)
	}
	if string(inv.invocations[0].payload) != `{"key":"val"}` {
		t.Errorf("expected payload %q, got %q", `{"key":"val"}`, string(inv.invocations[0].payload))
	}
}

func TestInvoke_EmptyPayload(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)

	s.invoke("sched-2", "t", "ns", "fn2", nil)

	inv.mu.Lock()
	defer inv.mu.Unlock()
	if len(inv.invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(inv.invocations))
	}
	if string(inv.invocations[0].payload) != `{}` {
		t.Errorf("expected default payload %q, got %q", `{}`, string(inv.invocations[0].payload))
	}
}

func TestInvoke_EmptyPayloadBytes(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)

	s.invoke("sched-3", "t", "ns", "fn3", json.RawMessage{})

	inv.mu.Lock()
	defer inv.mu.Unlock()
	if len(inv.invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(inv.invocations))
	}
	if string(inv.invocations[0].payload) != `{}` {
		t.Errorf("expected default payload %q, got %q", `{}`, string(inv.invocations[0].payload))
	}
}

func TestInvoke_Error(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{err: fmt.Errorf("invoke failed")}
	s := New(st, inv)

	// Should not panic even when invocation fails
	s.invoke("sched-4", "t", "ns", "fn4", json.RawMessage(`{}`))

	inv.mu.Lock()
	defer inv.mu.Unlock()
	if len(inv.invocations) != 1 {
		t.Fatalf("expected 1 invocation attempt, got %d", len(inv.invocations))
	}
}

func TestInvoke_UpdateLastRunError(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{tryLockErr: fmt.Errorf("lock failed")}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)

	// Should not panic even when TryLockSchedule fails
	s.invoke("sched-5", "t", "ns", "fn5", json.RawMessage(`{}`))

	inv.mu.Lock()
	defer inv.mu.Unlock()
	if len(inv.invocations) != 0 {
		t.Fatalf("expected 0 invocations when lock fails, got %d", len(inv.invocations))
	}
}

func TestInvoke_LockAlreadyHeld(t *testing.T) {
	t.Parallel()
	mock := &mockScheduleStore{
		tryLockReturnFn: func() bool {
			return false // Lock is already held
		},
	}
	st := &store.Store{ScheduleStore: mock}
	inv := &mockInvoker{}
	s := New(st, inv)

	s.invoke("sched-6", "t", "ns", "fn6", json.RawMessage(`{}`))

	inv.mu.Lock()
	defer inv.mu.Unlock()
	if len(inv.invocations) != 0 {
		t.Fatalf("expected 0 invocations when lock is held by another instance, got %d", len(inv.invocations))
	}
}
