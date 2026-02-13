package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// stubMetadataStore is a minimal stub implementing the methods under test.
// It delegates most methods to an embedded nil MetadataStore (they will panic
// if called unexpectedly, which is exactly what we want in tests).
type stubMetadataStore struct {
	MetadataStore // embed – uncalled methods will panic if exercised

	fnByNameCalls  atomic.Int64
	fnByIDCalls    atomic.Int64
	runtimeCalls   atomic.Int64
	codeCalls      atomic.Int64
	hasFilesCalls  atomic.Int64
	filesCalls     atomic.Int64
	layersCalls    atomic.Int64

	// configurable return values
	fn      *domain.Function
	rt      *RuntimeRecord
	code    *domain.FunctionCode
	hasF    bool
	files   map[string][]byte
	layers  []*domain.Layer
}

func (s *stubMetadataStore) Close() error                      { return nil }
func (s *stubMetadataStore) Ping(_ context.Context) error      { return nil }

func (s *stubMetadataStore) GetFunctionByName(_ context.Context, _ string) (*domain.Function, error) {
	s.fnByNameCalls.Add(1)
	if s.fn == nil {
		return nil, fmt.Errorf("function not found")
	}
	return s.fn, nil
}

func (s *stubMetadataStore) GetFunction(_ context.Context, _ string) (*domain.Function, error) {
	s.fnByIDCalls.Add(1)
	if s.fn == nil {
		return nil, fmt.Errorf("function not found")
	}
	return s.fn, nil
}

func (s *stubMetadataStore) GetRuntime(_ context.Context, _ string) (*RuntimeRecord, error) {
	s.runtimeCalls.Add(1)
	if s.rt == nil {
		return nil, fmt.Errorf("runtime not found")
	}
	return s.rt, nil
}

func (s *stubMetadataStore) GetFunctionCode(_ context.Context, _ string) (*domain.FunctionCode, error) {
	s.codeCalls.Add(1)
	return s.code, nil
}

func (s *stubMetadataStore) HasFunctionFiles(_ context.Context, _ string) (bool, error) {
	s.hasFilesCalls.Add(1)
	return s.hasF, nil
}

func (s *stubMetadataStore) GetFunctionFiles(_ context.Context, _ string) (map[string][]byte, error) {
	s.filesCalls.Add(1)
	return s.files, nil
}

func (s *stubMetadataStore) GetFunctionLayers(_ context.Context, _ string) ([]*domain.Layer, error) {
	s.layersCalls.Add(1)
	return s.layers, nil
}

// ─── write stubs (no-ops that allow invalidation to succeed) ────────────────

func (s *stubMetadataStore) SaveFunction(_ context.Context, _ *domain.Function) error          { return nil }
func (s *stubMetadataStore) UpdateFunction(_ context.Context, _ string, _ *FunctionUpdate) (*domain.Function, error) {
	return s.fn, nil
}
func (s *stubMetadataStore) DeleteFunction(_ context.Context, _ string) error                  { return nil }
func (s *stubMetadataStore) SaveFunctionCode(_ context.Context, _, _, _ string) error          { return nil }
func (s *stubMetadataStore) UpdateFunctionCode(_ context.Context, _, _, _ string) error        { return nil }
func (s *stubMetadataStore) UpdateCompileResult(_ context.Context, _ string, _ []byte, _ string, _ domain.CompileStatus, _ string) error {
	return nil
}
func (s *stubMetadataStore) DeleteFunctionCode(_ context.Context, _ string) error              { return nil }
func (s *stubMetadataStore) SaveFunctionFiles(_ context.Context, _ string, _ map[string][]byte) error { return nil }
func (s *stubMetadataStore) DeleteFunctionFiles(_ context.Context, _ string) error             { return nil }
func (s *stubMetadataStore) SetFunctionLayers(_ context.Context, _ string, _ []string) error   { return nil }
func (s *stubMetadataStore) SaveRuntime(_ context.Context, _ *RuntimeRecord) error             { return nil }
func (s *stubMetadataStore) DeleteRuntime(_ context.Context, _ string) error                   { return nil }

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestCachedStore_GetFunctionByName_CacheHit(t *testing.T) {
	stub := &stubMetadataStore{
		fn: &domain.Function{ID: "f1", Name: "hello", TenantID: "default", Namespace: "default"},
	}
	cached := NewCachedMetadataStore(stub, 1*time.Second)
	ctx := WithTenantScope(context.Background(), "default", "default")

	// First call – miss
	fn, err := cached.GetFunctionByName(ctx, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fn.Name != "hello" {
		t.Fatalf("expected hello, got %s", fn.Name)
	}
	if stub.fnByNameCalls.Load() != 1 {
		t.Fatalf("expected 1 underlying call, got %d", stub.fnByNameCalls.Load())
	}

	// Second call – should be cache hit
	fn2, err := cached.GetFunctionByName(ctx, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fn2.ID != "f1" {
		t.Fatalf("expected f1, got %s", fn2.ID)
	}
	if stub.fnByNameCalls.Load() != 1 {
		t.Fatalf("expected still 1 underlying call (cache hit), got %d", stub.fnByNameCalls.Load())
	}
}

func TestCachedStore_GetFunctionByName_Expiry(t *testing.T) {
	stub := &stubMetadataStore{
		fn: &domain.Function{ID: "f1", Name: "hello", TenantID: "default", Namespace: "default"},
	}
	cached := NewCachedMetadataStore(stub, 50*time.Millisecond)
	ctx := WithTenantScope(context.Background(), "default", "default")

	_, _ = cached.GetFunctionByName(ctx, "hello")
	if stub.fnByNameCalls.Load() != 1 {
		t.Fatal("expected 1 call")
	}

	// Wait for expiry
	time.Sleep(80 * time.Millisecond)

	_, _ = cached.GetFunctionByName(ctx, "hello")
	if stub.fnByNameCalls.Load() != 2 {
		t.Fatalf("expected 2 calls after expiry, got %d", stub.fnByNameCalls.Load())
	}
}

func TestCachedStore_SaveFunction_Invalidates(t *testing.T) {
	fn := &domain.Function{ID: "f1", Name: "hello", TenantID: "default", Namespace: "default"}
	stub := &stubMetadataStore{fn: fn}
	cached := NewCachedMetadataStore(stub, 10*time.Second)
	ctx := WithTenantScope(context.Background(), "default", "default")

	// Populate cache
	_, _ = cached.GetFunctionByName(ctx, "hello")
	if stub.fnByNameCalls.Load() != 1 {
		t.Fatal("expected 1 call")
	}

	// Save should invalidate
	_ = cached.SaveFunction(ctx, fn)

	// Next read should miss cache
	_, _ = cached.GetFunctionByName(ctx, "hello")
	if stub.fnByNameCalls.Load() != 2 {
		t.Fatalf("expected 2 calls after invalidation, got %d", stub.fnByNameCalls.Load())
	}
}

func TestCachedStore_DeleteFunction_Invalidates(t *testing.T) {
	fn := &domain.Function{ID: "f1", Name: "hello", TenantID: "default", Namespace: "default"}
	stub := &stubMetadataStore{fn: fn}
	cached := NewCachedMetadataStore(stub, 10*time.Second)
	ctx := WithTenantScope(context.Background(), "default", "default")

	// Populate cache
	_, _ = cached.GetFunctionByName(ctx, "hello")
	_, _ = cached.GetFunction(ctx, "f1")

	// Delete should invalidate both caches
	_ = cached.DeleteFunction(ctx, "f1")

	// Next read should go to underlying store
	_, _ = cached.GetFunctionByName(ctx, "hello")
	if stub.fnByNameCalls.Load() != 2 {
		t.Fatalf("expected 2 fnByName calls after delete, got %d", stub.fnByNameCalls.Load())
	}
}

func TestCachedStore_GetRuntime_CacheHit(t *testing.T) {
	stub := &stubMetadataStore{
		rt: &RuntimeRecord{ID: "python", Name: "Python"},
	}
	cached := NewCachedMetadataStore(stub, 1*time.Second)
	ctx := context.Background()

	_, _ = cached.GetRuntime(ctx, "python")
	_, _ = cached.GetRuntime(ctx, "python")

	if stub.runtimeCalls.Load() != 1 {
		t.Fatalf("expected 1 runtime call (cache hit), got %d", stub.runtimeCalls.Load())
	}
}

func TestCachedStore_SaveRuntime_Invalidates(t *testing.T) {
	rt := &RuntimeRecord{ID: "python", Name: "Python"}
	stub := &stubMetadataStore{rt: rt}
	cached := NewCachedMetadataStore(stub, 10*time.Second)
	ctx := context.Background()

	_, _ = cached.GetRuntime(ctx, "python")
	_ = cached.SaveRuntime(ctx, rt)
	_, _ = cached.GetRuntime(ctx, "python")

	if stub.runtimeCalls.Load() != 2 {
		t.Fatalf("expected 2 runtime calls after invalidation, got %d", stub.runtimeCalls.Load())
	}
}

func TestCachedStore_GetFunctionCode_CacheHit(t *testing.T) {
	stub := &stubMetadataStore{
		code: &domain.FunctionCode{FunctionID: "f1", SourceCode: "print('hi')"},
	}
	cached := NewCachedMetadataStore(stub, 1*time.Second)
	ctx := context.Background()

	_, _ = cached.GetFunctionCode(ctx, "f1")
	_, _ = cached.GetFunctionCode(ctx, "f1")

	if stub.codeCalls.Load() != 1 {
		t.Fatalf("expected 1 code call (cache hit), got %d", stub.codeCalls.Load())
	}
}

func TestCachedStore_SaveFunctionCode_Invalidates(t *testing.T) {
	stub := &stubMetadataStore{
		code: &domain.FunctionCode{FunctionID: "f1", SourceCode: "print('hi')"},
	}
	cached := NewCachedMetadataStore(stub, 10*time.Second)
	ctx := context.Background()

	_, _ = cached.GetFunctionCode(ctx, "f1")
	_ = cached.SaveFunctionCode(ctx, "f1", "new code", "newhash")
	_, _ = cached.GetFunctionCode(ctx, "f1")

	if stub.codeCalls.Load() != 2 {
		t.Fatalf("expected 2 code calls after invalidation, got %d", stub.codeCalls.Load())
	}
}

func TestCachedStore_UpdateCompileResult_Invalidates(t *testing.T) {
	stub := &stubMetadataStore{
		code: &domain.FunctionCode{FunctionID: "f1"},
	}
	cached := NewCachedMetadataStore(stub, 10*time.Second)
	ctx := context.Background()

	_, _ = cached.GetFunctionCode(ctx, "f1")
	_ = cached.UpdateCompileResult(ctx, "f1", []byte("binary"), "hash", domain.CompileStatusSuccess, "")
	_, _ = cached.GetFunctionCode(ctx, "f1")

	if stub.codeCalls.Load() != 2 {
		t.Fatalf("expected 2 code calls after compile result update, got %d", stub.codeCalls.Load())
	}
}

func TestCachedStore_HasFunctionFiles_CacheHit(t *testing.T) {
	stub := &stubMetadataStore{hasF: true}
	cached := NewCachedMetadataStore(stub, 1*time.Second)
	ctx := context.Background()

	_, _ = cached.HasFunctionFiles(ctx, "f1")
	_, _ = cached.HasFunctionFiles(ctx, "f1")

	if stub.hasFilesCalls.Load() != 1 {
		t.Fatalf("expected 1 hasFiles call (cache hit), got %d", stub.hasFilesCalls.Load())
	}
}

func TestCachedStore_SaveFunctionFiles_Invalidates(t *testing.T) {
	stub := &stubMetadataStore{hasF: true, files: map[string][]byte{"main.py": []byte("code")}}
	cached := NewCachedMetadataStore(stub, 10*time.Second)
	ctx := context.Background()

	_, _ = cached.HasFunctionFiles(ctx, "f1")
	_, _ = cached.GetFunctionFiles(ctx, "f1")

	_ = cached.SaveFunctionFiles(ctx, "f1", map[string][]byte{"new.py": []byte("new")})

	_, _ = cached.HasFunctionFiles(ctx, "f1")
	_, _ = cached.GetFunctionFiles(ctx, "f1")

	if stub.hasFilesCalls.Load() != 2 {
		t.Fatalf("expected 2 hasFiles calls after invalidation, got %d", stub.hasFilesCalls.Load())
	}
	if stub.filesCalls.Load() != 2 {
		t.Fatalf("expected 2 files calls after invalidation, got %d", stub.filesCalls.Load())
	}
}

func TestCachedStore_GetFunctionLayers_CacheHit(t *testing.T) {
	stub := &stubMetadataStore{
		layers: []*domain.Layer{{ID: "l1", Name: "numpy"}},
	}
	cached := NewCachedMetadataStore(stub, 1*time.Second)
	ctx := context.Background()

	_, _ = cached.GetFunctionLayers(ctx, "f1")
	_, _ = cached.GetFunctionLayers(ctx, "f1")

	if stub.layersCalls.Load() != 1 {
		t.Fatalf("expected 1 layers call (cache hit), got %d", stub.layersCalls.Load())
	}
}

func TestCachedStore_SetFunctionLayers_Invalidates(t *testing.T) {
	stub := &stubMetadataStore{
		layers: []*domain.Layer{{ID: "l1", Name: "numpy"}},
	}
	cached := NewCachedMetadataStore(stub, 10*time.Second)
	ctx := context.Background()

	_, _ = cached.GetFunctionLayers(ctx, "f1")
	_ = cached.SetFunctionLayers(ctx, "f1", []string{"l2"})
	_, _ = cached.GetFunctionLayers(ctx, "f1")

	if stub.layersCalls.Load() != 2 {
		t.Fatalf("expected 2 layers calls after invalidation, got %d", stub.layersCalls.Load())
	}
}

func TestCachedStore_TenantIsolation(t *testing.T) {
	fn1 := &domain.Function{ID: "f1", Name: "hello", TenantID: "t1", Namespace: "ns1"}
	fn2 := &domain.Function{ID: "f2", Name: "hello", TenantID: "t2", Namespace: "ns1"}

	stub := &stubMetadataStore{}

	cached := NewCachedMetadataStore(stub, 10*time.Second)

	// Manually populate cache for two different tenants
	ctx1 := WithTenantScope(context.Background(), "t1", "ns1")
	ctx2 := WithTenantScope(context.Background(), "t2", "ns1")

	stub.fn = fn1
	_, _ = cached.GetFunctionByName(ctx1, "hello")

	stub.fn = fn2
	_, _ = cached.GetFunctionByName(ctx2, "hello")

	// Should have called underlying store twice (different tenant)
	if stub.fnByNameCalls.Load() != 2 {
		t.Fatalf("expected 2 calls for different tenants, got %d", stub.fnByNameCalls.Load())
	}
}

func TestCachedStore_DefaultTTL(t *testing.T) {
	cached := NewCachedMetadataStore(nil, 0)
	if cached.ttl != DefaultCacheTTL {
		t.Fatalf("expected default TTL %v, got %v", DefaultCacheTTL, cached.ttl)
	}
}
