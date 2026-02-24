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

func TestSetSLOPolicy(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, SLOPolicy: update.SLOPolicy}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"enabled":true,"objectives":{"success_rate_pct":99.9}}`
		req := httptest.NewRequest("PUT", "/functions/hello/slo", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PUT", "/functions/hello/slo", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid_policy", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"window_s":-1}`
		req := httptest.NewRequest("PUT", "/functions/hello/slo", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestGetSLOPolicy(t *testing.T) {
	t.Run("with_policy", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, SLOPolicy: &domain.SLOPolicy{Enabled: true, WindowS: 900}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/slo", nil)
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
		req := httptest.NewRequest("GET", "/functions/hello/slo", nil)
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
		req := httptest.NewRequest("GET", "/functions/nope/slo", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteSLOPolicy(t *testing.T) {
	ms := &mockMetadataStore{
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/hello/slo", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestValidateSLOPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  *domain.SLOPolicy
		wantErr bool
	}{
		{"nil", nil, true},
		{"valid", &domain.SLOPolicy{WindowS: 900, MinSamples: 20}, false},
		{"negative_window", &domain.SLOPolicy{WindowS: -1}, true},
		{"negative_samples", &domain.SLOPolicy{MinSamples: -1}, true},
		{"bad_success_rate", &domain.SLOPolicy{Objectives: domain.SLOObjectives{SuccessRatePct: 101}}, true},
		{"bad_p95", &domain.SLOPolicy{Objectives: domain.SLOObjectives{P95DurationMs: -1}}, true},
		{"bad_cold_start", &domain.SLOPolicy{Objectives: domain.SLOObjectives{ColdStartRatePct: -1}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSLOPolicy(tt.policy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSLOPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeSLOPolicy(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := normalizeSLOPolicy(nil); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("defaults", func(t *testing.T) {
		p := normalizeSLOPolicy(&domain.SLOPolicy{})
		if p.WindowS != defaultSLOWindowSeconds || p.MinSamples != defaultSLOMinSamples {
			t.Fatalf("unexpected defaults: %+v", p)
		}
		if p.Notifications == nil {
			t.Fatal("expected non-nil Notifications")
		}
	})
}

func TestDeleteSLOPolicy_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return nil, fmt.Errorf("function not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/nope/slo", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestDeleteSLOPolicy_UpdateError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateFunctionFn: func(_ context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/hello/slo", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}
