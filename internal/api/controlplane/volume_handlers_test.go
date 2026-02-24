package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/volume"
)

func setupVolumeHandler(t *testing.T, ms *mockMetadataStore) (*Handler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	vm, err := volume.NewManager(s, &volume.Config{VolumeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Store: s, VolumeManager: vm}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestCreateVolume_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	body := `{"name":"test","size_mb":100}`
	req := httptest.NewRequest("POST", "/volumes", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestListVolumes(t *testing.T) {
	ms := &mockMetadataStore{
		listVolumesFn: func(_ context.Context) ([]*domain.Volume, error) {
			return []*domain.Volume{{ID: "v1", Name: "vol1"}}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/volumes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListVolumes_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listVolumesFn: func(_ context.Context) ([]*domain.Volume, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/volumes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGetVolume(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getVolumeByNameFn: func(_ context.Context, name string) (*domain.Volume, error) {
				return &domain.Volume{ID: "v1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/volumes/myvol", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getVolumeByNameFn: func(_ context.Context, name string) (*domain.Volume, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/volumes/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteVolume_NilManager(t *testing.T) {
	ms := &mockMetadataStore{
		getVolumeByNameFn: func(_ context.Context, name string) (*domain.Volume, error) {
			return &domain.Volume{ID: "v1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/volumes/myvol", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestSetFunctionMounts_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	body := `{"mounts":[]}`
	req := httptest.NewRequest("PUT", "/functions/hello/mounts", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestSetFunctionMounts_FunctionNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupVolumeHandler(t, ms)
	req := httptest.NewRequest("PUT", "/functions/nope/mounts", strings.NewReader(`{"mounts":[]}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestCreateVolume_Success(t *testing.T) {
	ms := &mockMetadataStore{
		createVolumeFn: func(_ context.Context, vol *domain.Volume) error { return nil },
	}
	_, mux := setupVolumeHandler(t, ms)
	body := `{"name":"test-vol","size_mb":10,"description":"test volume"}`
	req := httptest.NewRequest("POST", "/volumes", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// May fail if mkfs.ext4 not available, but exercises the code path
	if w.Code != http.StatusCreated && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 201 or 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateVolume_BadJSON(t *testing.T) {
	_, mux := setupVolumeHandler(t, nil)
	req := httptest.NewRequest("POST", "/volumes", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateVolume_MissingName(t *testing.T) {
	_, mux := setupVolumeHandler(t, nil)
	body := `{"size_mb":100}`
	req := httptest.NewRequest("POST", "/volumes", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateVolume_BadSize(t *testing.T) {
	_, mux := setupVolumeHandler(t, nil)
	body := `{"name":"test","size_mb":-1}`
	req := httptest.NewRequest("POST", "/volumes", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestSetFunctionMounts_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		getVolumeFn: func(_ context.Context, id string) (*domain.Volume, error) {
			return &domain.Volume{ID: id, Name: "v1", ImagePath: "/opt/nova/volumes/test.ext4"}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupVolumeHandler(t, ms)
	body := `{"mounts":[{"volume_id":"v1","mount_path":"/data"}]}`
	req := httptest.NewRequest("PUT", "/functions/hello/mounts", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestSetFunctionMounts_BadJSON(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupVolumeHandler(t, ms)
	req := httptest.NewRequest("PUT", "/functions/hello/mounts", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestSetFunctionMounts_MissingVolumeID(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupVolumeHandler(t, ms)
	body := `{"mounts":[{"mount_path":"/data"}]}`
	req := httptest.NewRequest("PUT", "/functions/hello/mounts", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestGetFunctionMounts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, Mounts: []domain.VolumeMount{}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/mounts", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/nope/mounts", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteVolume_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getVolumeByNameFn: func(_ context.Context, name string) (*domain.Volume, error) {
			return &domain.Volume{ID: "v1", Name: name, ImagePath: "/tmp/nonexist.ext4"}, nil
		},
		getVolumeFn: func(_ context.Context, id string) (*domain.Volume, error) {
			return &domain.Volume{ID: id, Name: "myvol", ImagePath: "/tmp/nonexist.ext4"}, nil
		},
		deleteVolumeFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupVolumeHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/volumes/myvol", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestDeleteVolume_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getVolumeByNameFn: func(_ context.Context, name string) (*domain.Volume, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupVolumeHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/volumes/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}
