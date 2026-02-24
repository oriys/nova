package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/store"
)

func setupAIHandler(t *testing.T, ms *mockMetadataStore) (*AIHandler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	svc := ai.NewService(ai.DefaultConfig())
	h := &AIHandler{Service: svc, Store: s}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestAIStatus(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("GET", "/ai/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestAIGetConfig(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("GET", "/ai/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestAIUpdateConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			setConfigFn: func(_ context.Context, key, value string) error { return nil },
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{}, nil
			},
		}
		_, mux := setupAIHandler(t, ms)
		body := `{"enabled":true,"model":"gpt-4"}`
		req := httptest.NewRequest("PUT", "/ai/config", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupAIHandler(t, nil)
		req := httptest.NewRequest("PUT", "/ai/config", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("masked_key_ignored", func(t *testing.T) {
		ms := &mockMetadataStore{
			setConfigFn: func(_ context.Context, key, value string) error { return nil },
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{}, nil
			},
		}
		_, mux := setupAIHandler(t, ms)
		body := `{"api_key":"sk-****abcd"}`
		req := httptest.NewRequest("PUT", "/ai/config", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})
}

func TestIsMaskedAPIKey(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"****", true},
		{"sk-****abcd", true},
		{"sk-real-key-here", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isMaskedAPIKey(tt.input); got != tt.expected {
				t.Fatalf("isMaskedAPIKey(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLoadAIConfigFromStore(t *testing.T) {
	t.Run("nil_store", func(t *testing.T) {
		h := &AIHandler{Service: ai.NewService(ai.DefaultConfig()), Store: nil}
		cfg := h.loadAIConfigFromStore(httptest.NewRequest("GET", "/", nil))
		if cfg.Model != "gpt-4o-mini" {
			t.Fatalf("unexpected model: %s", cfg.Model)
		}
	})

	t.Run("with_values", func(t *testing.T) {
		ms := &mockMetadataStore{
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{
					"ai_enabled": "true",
					"ai_model":   "gpt-4",
					"ai_api_key": "sk-test",
				}, nil
			},
		}
		s := store.NewStore(ms)
		h := &AIHandler{Service: ai.NewService(ai.DefaultConfig()), Store: s}
		cfg := h.loadAIConfigFromStore(httptest.NewRequest("GET", "/", nil))
		if !cfg.Enabled || cfg.Model != "gpt-4" || cfg.APIKey != "sk-test" {
			t.Fatalf("unexpected config: %+v", cfg)
		}
	})
}

func TestAIGenerate_BadJSON(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("POST", "/ai/generate", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIGenerate_MissingDescription(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"runtime":"python"}`
	req := httptest.NewRequest("POST", "/ai/generate", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIGenerate_MissingRuntime(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"description":"hello world function"}`
	req := httptest.NewRequest("POST", "/ai/generate", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIReview_BadJSON(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("POST", "/ai/review", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIReview_MissingCode(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"runtime":"python"}`
	req := httptest.NewRequest("POST", "/ai/review", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIRewrite_BadJSON(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("POST", "/ai/rewrite", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIRewrite_MissingCode(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"runtime":"python"}`
	req := httptest.NewRequest("POST", "/ai/rewrite", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIAnalyzeDiagnostics_BadJSON(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("POST", "/ai/analyze-diagnostics", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIAnalyzeDiagnostics_MissingFunctionName(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{}`
	req := httptest.NewRequest("POST", "/ai/analyze-diagnostics", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIListPrompts(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("GET", "/ai/prompts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestAIGetPrompt(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("GET", "/ai/prompts/generate", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// This will succeed or return 404 depending on embedded templates
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Fatalf("expected 200 or 404, got %d", w.Code)
	}
}

func TestAIUpdatePrompt_BadJSON(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("PUT", "/ai/prompts/generate", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIListModels_ServiceDisabled(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("GET", "/ai/models", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Service is disabled so this will likely return an error
	if w.Code == http.StatusOK || w.Code == http.StatusBadGateway || w.Code == http.StatusInternalServerError {
		// any of these is acceptable
	} else {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestAIGetPrompt_NotFound(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	req := httptest.NewRequest("GET", "/ai/prompts/nonexistent-template", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Expect not found since this template doesn't exist
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 404 or 500, got %d", w.Code)
	}
}

func TestAIUpdatePrompt_Empty(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"content":""}`
	req := httptest.NewRequest("PUT", "/ai/prompts/generate", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Template not found or invalid
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Fatalf("expected 400 or 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLoadAIConfigFromStore_Error(t *testing.T) {
	ms := &mockMetadataStore{
		getConfigFn: func(_ context.Context) (map[string]string, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	s := store.NewStore(ms)
	h := &AIHandler{Service: ai.NewService(ai.DefaultConfig()), Store: s}
	cfg := h.loadAIConfigFromStore(httptest.NewRequest("GET", "/", nil))
	// Should return defaults when store errors
	if cfg.Model != "gpt-4o-mini" {
		t.Fatalf("expected default model, got: %s", cfg.Model)
	}
}

func TestAIGenerate_ServiceError(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"description":"hello world","runtime":"python"}`
	req := httptest.NewRequest("POST", "/ai/generate", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestAIReview_ServiceError(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"code":"def hello(): pass","runtime":"python"}`
	req := httptest.NewRequest("POST", "/ai/review", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestAIReview_MissingRuntime(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"code":"def hello(): pass"}`
	req := httptest.NewRequest("POST", "/ai/review", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIRewrite_ServiceError(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"code":"def hello(): pass","runtime":"python"}`
	req := httptest.NewRequest("POST", "/ai/rewrite", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestAIRewrite_MissingRuntime(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"code":"def hello(): pass"}`
	req := httptest.NewRequest("POST", "/ai/rewrite", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestAIAnalyzeDiagnostics_ServiceError(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"function_name":"hello"}`
	req := httptest.NewRequest("POST", "/ai/analyze-diagnostics", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Service will error since no API key configured
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestAIAnalyzeDiagnostics_FunctionNotFound(t *testing.T) {
	_, mux := setupAIHandler(t, nil)
	body := `{"function_name":"nope"}`
	req := httptest.NewRequest("POST", "/ai/analyze-diagnostics", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Will error - AI service not configured
	expectStatus(t, w, http.StatusInternalServerError)
}
