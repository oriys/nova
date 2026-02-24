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
)

func TestSetCapacityPolicy(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, CapacityPolicy: update.CapacityPolicy}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"enabled":true,"max_inflight":10}`
		req := httptest.NewRequest("PUT", "/functions/hello/capacity", strings.NewReader(body))
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
		body := `{"enabled":true}`
		req := httptest.NewRequest("PUT", "/functions/nope/capacity", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})

	t.Run("bad_json", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("PUT", "/functions/hello/capacity", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid_policy", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"max_inflight":-1}`
		req := httptest.NewRequest("PUT", "/functions/hello/capacity", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestGetCapacityPolicy(t *testing.T) {
	t.Run("with_policy", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, CapacityPolicy: &domain.CapacityPolicy{Enabled: true}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/capacity", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("nil_policy", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/capacity", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/nope/capacity", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteCapacityPolicy(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/hello/capacity", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteCapacityPolicy_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/nope/capacity", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestSetCapacityPolicy_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"enabled":true,"max_inflight":10}`
	req := httptest.NewRequest("PUT", "/functions/hello/capacity", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestSetCapacityPolicy_Validation(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)

	tests := []struct {
		name string
		body string
	}{
		{"negative_max_inflight", `{"enabled":true,"max_inflight":-1}`},
		{"negative_max_queue_depth", `{"enabled":true,"max_queue_depth":-1}`},
		{"negative_max_queue_wait_ms", `{"enabled":true,"max_queue_wait_ms":-1}`},
		{"negative_retry_after_s", `{"enabled":true,"retry_after_s":-1}`},
		{"invalid_shed_status_code", `{"enabled":true,"shed_status_code":418}`},
		{"breaker_error_pct_over_100", `{"enabled":true,"breaker_error_pct":101}`},
		{"breaker_error_pct_negative", `{"enabled":true,"breaker_error_pct":-1}`},
		{"negative_breaker_window_s", `{"enabled":true,"breaker_window_s":-1}`},
		{"negative_breaker_open_s", `{"enabled":true,"breaker_open_s":-1}`},
		{"negative_half_open_probes", `{"enabled":true,"half_open_probes":-1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/functions/hello/capacity", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			expectStatus(t, w, http.StatusBadRequest)
		})
	}
}
