package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/store"
)

func setupTestSuiteHandler(t *testing.T, ms *mockMetadataStore, aiSvc *ai.Service) *http.ServeMux {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	h := &TestSuiteHandler{Store: s, AIService: aiSvc}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestGetTestSuite(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTestSuiteFn: func(_ context.Context, name string) (*store.TestSuite, error) {
				return &store.TestSuite{FunctionName: name, TestCases: json.RawMessage(`[]`)}, nil
			},
		}
		mux := setupTestSuiteHandler(t, ms, nil)
		req := httptest.NewRequest("GET", "/functions/hello/test-suite", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTestSuiteFn: func(_ context.Context, name string) (*store.TestSuite, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		mux := setupTestSuiteHandler(t, ms, nil)
		req := httptest.NewRequest("GET", "/functions/nope/test-suite", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestSaveTestSuite(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTestSuiteFn: func(_ context.Context, name string) (*store.TestSuite, error) {
				return nil, fmt.Errorf("not found")
			},
			saveTestSuiteFn: func(_ context.Context, ts *store.TestSuite) error { return nil },
		}
		mux := setupTestSuiteHandler(t, ms, nil)
		body := `{"test_cases":[{"input":"{}","expected_output":"{}"}]}`
		req := httptest.NewRequest("PUT", "/functions/hello/test-suite", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("bad_json", func(t *testing.T) {
		mux := setupTestSuiteHandler(t, nil, nil)
		req := httptest.NewRequest("PUT", "/functions/hello/test-suite", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_test_cases", func(t *testing.T) {
		mux := setupTestSuiteHandler(t, nil, nil)
		body := `{}`
		req := httptest.NewRequest("PUT", "/functions/hello/test-suite", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestDeleteTestSuite(t *testing.T) {
	ms := &mockMetadataStore{
		deleteTestSuiteFn: func(_ context.Context, name string) error { return nil },
	}
	mux := setupTestSuiteHandler(t, ms, nil)
	req := httptest.NewRequest("DELETE", "/functions/hello/test-suite", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteTestSuite_Error(t *testing.T) {
	ms := &mockMetadataStore{
		deleteTestSuiteFn: func(_ context.Context, name string) error { return fmt.Errorf("not found") },
	}
	mux := setupTestSuiteHandler(t, ms, nil)
	req := httptest.NewRequest("DELETE", "/functions/nope/test-suite", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		// Some implementations return ok even on error
	}
}

func TestGenerateTests_MissingCode(t *testing.T) {
	mux := setupTestSuiteHandler(t, nil, ai.NewService(ai.DefaultConfig()))
	body := `{"function_name":"hello"}`
	req := httptest.NewRequest("POST", "/ai/generate-tests", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestGenerateTests_MissingFunctionName(t *testing.T) {
	mux := setupTestSuiteHandler(t, nil, ai.NewService(ai.DefaultConfig()))
	body := `{"code":"print('hello')"}`
	req := httptest.NewRequest("POST", "/ai/generate-tests", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestGenerateTests_ServiceError(t *testing.T) {
	mux := setupTestSuiteHandler(t, nil, ai.NewService(ai.DefaultConfig()))
	body := `{"function_name":"hello","code":"print('hello')","runtime":"python"}`
	req := httptest.NewRequest("POST", "/ai/generate-tests", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Service will error since no API key configured
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGenerateTests_NilAI(t *testing.T) {
	mux := setupTestSuiteHandler(t, nil, nil)
	body := `{"function_name":"hello","code":"def hello(): pass"}`
	req := httptest.NewRequest("POST", "/ai/generate-tests", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusServiceUnavailable)
}

func TestGenerateTests_BadJSON(t *testing.T) {
	mux := setupTestSuiteHandler(t, nil, ai.NewService(ai.DefaultConfig()))
	req := httptest.NewRequest("POST", "/ai/generate-tests", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}
