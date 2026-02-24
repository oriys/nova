package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/firecracker"
)

// testBackend is a minimal Backend implementation for snapshot tests.
type testBackend struct {
	snapshotDir string
}

func (b *testBackend) CreateVM(_ context.Context, _ *domain.Function, _ []byte) (*backend.VM, error) {
	return nil, nil
}
func (b *testBackend) CreateVMWithFiles(_ context.Context, _ *domain.Function, _ map[string][]byte) (*backend.VM, error) {
	return nil, nil
}
func (b *testBackend) StopVM(_ string) error                           { return nil }
func (b *testBackend) NewClient(_ *backend.VM) (backend.Client, error) { return nil, nil }
func (b *testBackend) Shutdown()                                       {}
func (b *testBackend) SnapshotDir() string                             { return b.snapshotDir }

func setupSnapshotHandler(t *testing.T, ms *mockMetadataStore, snapshotDir string) *http.ServeMux {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	h := &Handler{
		Store:   newTestStore(ms),
		Backend: &testBackend{snapshotDir: snapshotDir},
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestListSnapshots_NoSnapshots(t *testing.T) {
	ms := &mockMetadataStore{
		listFunctionsFn: func(_ context.Context, limit, offset int) ([]*domain.Function, error) {
			return []*domain.Function{}, nil
		},
	}
	mux := setupSnapshotHandler(t, ms, t.TempDir())
	req := httptest.NewRequest("GET", "/snapshots", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListSnapshots_WithSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "fn-1.snap"), []byte("snap"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "fn-1.mem"), []byte("mem"), 0644)

	ms := &mockMetadataStore{
		listFunctionsFn: func(_ context.Context, limit, offset int) ([]*domain.Function, error) {
			if offset > 0 {
				return []*domain.Function{}, nil
			}
			return []*domain.Function{{ID: "fn-1", Name: "hello"}}, nil
		},
	}
	mux := setupSnapshotHandler(t, ms, tmpDir)
	req := httptest.NewRequest("GET", "/snapshots", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp struct {
		Items      []json.RawMessage `json:"items"`
		Pagination struct {
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Pagination.Total != 1 {
		t.Fatalf("expected 1 snapshot, got %d", resp.Pagination.Total)
	}
}

func TestCreateSnapshot_NilFCAdapter(t *testing.T) {
	ms := &mockMetadataStore{}
	h := &Handler{Store: newTestStore(ms), FCAdapter: nil}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/functions/hello/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestDeleteSnapshot_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	mux := setupSnapshotHandler(t, ms, tmpDir)
	req := httptest.NewRequest("DELETE", "/functions/hello/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestDeleteSnapshot_FunctionNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	mux := setupSnapshotHandler(t, ms, t.TempDir())
	req := httptest.NewRequest("DELETE", "/functions/nope/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestDeleteSnapshot_Success(t *testing.T) {
	tmpDir := t.TempDir()
	fnID := "fn-1"
	// Create fake snapshot files
	os.WriteFile(filepath.Join(tmpDir, fnID+".snap"), []byte("snap"), 0644)
	os.WriteFile(filepath.Join(tmpDir, fnID+".mem"), []byte("mem"), 0644)

	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: fnID, Name: name}, nil
		},
	}
	mux := setupSnapshotHandler(t, ms, tmpDir)
	req := httptest.NewRequest("DELETE", "/functions/hello/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Fatalf("expected deleted status, got %v", resp)
	}
}

func TestCreateSnapshot_FunctionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	s := newTestStore(ms)
	b := &testBackend{snapshotDir: tmpDir}
	h := &Handler{Store: s, Backend: b, FCAdapter: &firecracker.Adapter{}}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/functions/nope/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestCreateSnapshot_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	fnID := "fn-1"
	os.WriteFile(filepath.Join(tmpDir, fnID+".snap"), []byte("snap"), 0644)
	os.WriteFile(filepath.Join(tmpDir, fnID+".mem"), []byte("mem"), 0644)

	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: fnID, Name: name}, nil
		},
	}
	s := newTestStore(ms)
	b := &testBackend{snapshotDir: tmpDir}
	h := &Handler{Store: s, Backend: b, FCAdapter: &firecracker.Adapter{}}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/functions/hello/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "exists" {
		t.Fatalf("expected exists status, got %v", resp)
	}
}

func TestCreateSnapshot_CodeNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		getFunctionCodeFn: func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
			return nil, fmt.Errorf("code not found")
		},
	}
	s := newTestStore(ms)
	b := &testBackend{snapshotDir: tmpDir}
	h := &Handler{Store: s, Backend: b, FCAdapter: &firecracker.Adapter{}}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/functions/hello/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestCreateSnapshot_NilCode(t *testing.T) {
	tmpDir := t.TempDir()
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		getFunctionCodeFn: func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
			return nil, nil
		},
	}
	s := newTestStore(ms)
	b := &testBackend{snapshotDir: tmpDir}
	h := &Handler{Store: s, Backend: b, FCAdapter: &firecracker.Adapter{}}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/functions/hello/snapshot", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}
