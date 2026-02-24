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

func TestSetScalingPolicy(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, AutoScalePolicy: update.AutoScalePolicy}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"enabled":true,"min_replicas":1,"max_replicas":10,"target_utilization":0.7}`
		req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader(body))
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
		req := httptest.NewRequest("PUT", "/functions/nope/scaling", strings.NewReader(body))
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
		req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid_max_less_than_min", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"min_replicas":10,"max_replicas":1}`
		req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid_utilization", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"target_utilization":2.0}`
		req := httptest.NewRequest("PUT", "/functions/hello/scaling", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestGetScalingPolicy(t *testing.T) {
	t.Run("with_policy", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, AutoScalePolicy: &domain.AutoScalePolicy{Enabled: true}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/scaling", nil)
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
		req := httptest.NewRequest("GET", "/functions/hello/scaling", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})
}

func TestDeleteScalingPolicy(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/hello/scaling", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteScalingPolicy_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/nope/scaling", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestDeleteScalingPolicy_UpdateError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/hello/scaling", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}
