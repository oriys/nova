package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/layer"
	"github.com/oriys/nova/internal/store"
)

func setupLayerHandler(t *testing.T, ms *mockMetadataStore) (*Handler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	lm := layer.New(s, t.TempDir(), 6)
	h := &Handler{Store: s, LayerManager: lm}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestCreateLayer_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	body := `{"name":"test","runtime":"python","files":{"hello.py":"aGVsbG8="}}`
	req := httptest.NewRequest("POST", "/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestListLayers_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("GET", "/layers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestListLayers_Success(t *testing.T) {
	ms := &mockMetadataStore{
		listLayersFn: func(_ context.Context, limit, offset int) ([]*domain.Layer, error) {
			return []*domain.Layer{{ID: "l1", Name: "test"}}, nil
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("GET", "/layers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListLayers_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listLayersFn: func(_ context.Context, limit, offset int) ([]*domain.Layer, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("GET", "/layers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGetLayer_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("GET", "/layers/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestGetLayer_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getLayerByNameFn: func(_ context.Context, name string) (*domain.Layer, error) {
			return &domain.Layer{ID: "l1", Name: name}, nil
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("GET", "/layers/mylib", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetLayer_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getLayerByNameFn: func(_ context.Context, name string) (*domain.Layer, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("GET", "/layers/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestDeleteLayer_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("DELETE", "/layers/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestSetFunctionLayers_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("PUT", "/functions/hello/layers", strings.NewReader(`{"layer_ids":[]}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestGetFunctionLayers_NilManager(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("GET", "/functions/hello/layers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotImplemented)
}

func TestGetFunctionLayers_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		getFunctionLayersFn: func(_ context.Context, funcID string) ([]*domain.Layer, error) {
			return []*domain.Layer{}, nil
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("GET", "/functions/hello/layers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetFunctionLayers_FunctionNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("GET", "/functions/nope/layers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestSetFunctionLayers_FunctionNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupLayerHandler(t, ms)
	body := `{"layer_ids":["l1"]}`
	req := httptest.NewRequest("PUT", "/functions/nope/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestSetFunctionLayers_BadJSON(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("PUT", "/functions/hello/layers", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestDeleteLayer_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getLayerByNameFn: func(_ context.Context, name string) (*domain.Layer, error) {
			return &domain.Layer{ID: "l1", Name: name}, nil
		},
		getLayerFn: func(_ context.Context, id string) (*domain.Layer, error) {
			return &domain.Layer{ID: id, Name: "mylib"}, nil
		},
		deleteLayerFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/layers/mylib", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteLayer_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getLayerByNameFn: func(_ context.Context, name string) (*domain.Layer, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupLayerHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/layers/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestSetFunctionLayers_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		getLayerFn: func(_ context.Context, id string) (*domain.Layer, error) {
			return &domain.Layer{ID: id, Name: "test", Runtime: "python", SizeMB: 10}, nil
		},
		setFunctionLayersFn: func(_ context.Context, funcID string, layerIDs []string) error { return nil },
		getFunctionLayersFn: func(_ context.Context, funcID string) ([]*domain.Layer, error) {
			return []*domain.Layer{{ID: "l1", Name: "test", SizeMB: 10}}, nil
		},
	}
	_, mux := setupLayerHandler(t, ms)
	body := `{"layer_ids":["l1"]}`
	req := httptest.NewRequest("PUT", "/functions/hello/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestCreateLayer_BadJSON(t *testing.T) {
	_, mux := setupLayerHandler(t, nil)
	req := httptest.NewRequest("POST", "/layers", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateLayer_MissingName(t *testing.T) {
	_, mux := setupLayerHandler(t, nil)
	body := `{"runtime":"python","files":{"hello.py":"aGVsbG8="}}`
	req := httptest.NewRequest("POST", "/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateLayer_NoFiles(t *testing.T) {
	_, mux := setupLayerHandler(t, nil)
	body := `{"name":"test","runtime":"python","files":{}}`
	req := httptest.NewRequest("POST", "/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateLayer_InvalidBase64(t *testing.T) {
	_, mux := setupLayerHandler(t, nil)
	body := `{"name":"test","runtime":"python","files":{"x.py":"!!!notbase64"}}`
	req := httptest.NewRequest("POST", "/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}
