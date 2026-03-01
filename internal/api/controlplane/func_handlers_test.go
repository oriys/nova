package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/service"
	"github.com/oriys/nova/internal/store"
)

// setupFuncTestHandler builds a Handler with FunctionService wired up for CreateFunction tests.
func setupFuncTestHandler(t *testing.T, ms *mockMetadataStore) (*Handler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	h := &Handler{
		Store:           s,
		FunctionService: service.NewFunctionService(s, nil),
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestCreateFunction(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		publishedVersion := 0
		publishedFunctionID := ""
		publishedCode := ""
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
			saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
				fn.ID = "fn-1"
				fn.CreatedAt = time.Now()
				return nil
			},
			saveFunctionCodeFn: func(ctx context.Context, funcID, sourceCode, sourceHash string) error {
				return nil
			},
			publishVersionFn: func(ctx context.Context, funcID string, version *domain.FunctionVersion) error {
				publishedFunctionID = funcID
				publishedVersion = version.Version
				publishedCode = version.Code
				return nil
			},
		}
		_, mux := setupFuncTestHandler(t, ms)
		body := `{"name":"hello","runtime":"python","code":"print(1)"}`
		req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["name"] != "hello" {
			t.Fatalf("unexpected name: %v", resp["name"])
		}
		if publishedVersion != 1 {
			t.Fatalf("expected initial published version 1, got %d", publishedVersion)
		}
		if publishedFunctionID == "" {
			t.Fatalf("expected publishVersion to receive function id")
		}
		if publishedCode != "print(1)" {
			t.Fatalf("expected published code to match create payload")
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupFuncTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/functions", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_name", func(t *testing.T) {
		_, mux := setupFuncTestHandler(t, nil)
		body := `{"runtime":"python","code":"print(1)"}`
		req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_runtime", func(t *testing.T) {
		_, mux := setupFuncTestHandler(t, nil)
		body := `{"name":"hello","code":"print(1)"}`
		req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_code", func(t *testing.T) {
		_, mux := setupFuncTestHandler(t, nil)
		body := `{"name":"hello","runtime":"python"}`
		req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "existing", Name: name}, nil
			},
		}
		_, mux := setupFuncTestHandler(t, ms)
		body := `{"name":"hello","runtime":"python","code":"print(1)"}`
		req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_runtime", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRuntimeFn: func(ctx context.Context, id string) (*store.RuntimeRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupFuncTestHandler(t, ms)
		body := `{"name":"hello","runtime":"invalid-runtime","code":"print(1)"}`
		req := httptest.NewRequest("POST", "/functions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
		}
	})
}

func TestListFunctions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp paginatedListResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
	})

	t.Run("with_data", func(t *testing.T) {
		ms := &mockMetadataStore{
			listFunctionsFn: func(ctx context.Context, limit, offset int) ([]*domain.Function, error) {
				return []*domain.Function{
					{ID: "1", Name: "fn1", Runtime: "python"},
					{ID: "2", Name: "fn2", Runtime: "node"},
				}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
		var resp paginatedListResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		items := resp.Items.([]interface{})
		if len(items) != 2 {
			t.Fatalf("expected 2 functions, got %d", len(items))
		}
	})

	t.Run("with_search", func(t *testing.T) {
		var searchedQuery string
		ms := &mockMetadataStore{
			searchFunctionsFn: func(ctx context.Context, query string, limit, offset int) ([]*domain.Function, error) {
				searchedQuery = query
				return []*domain.Function{{ID: "1", Name: "hello"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions?search=hello", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
		if searchedQuery != "hello" {
			t.Fatalf("expected search query 'hello', got %q", searchedQuery)
		}
	})

	t.Run("with_q_param", func(t *testing.T) {
		var searchedQuery string
		ms := &mockMetadataStore{
			searchFunctionsFn: func(ctx context.Context, query string, limit, offset int) ([]*domain.Function, error) {
				searchedQuery = query
				return []*domain.Function{}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions?q=world", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
		if searchedQuery != "world" {
			t.Fatalf("expected q param 'world', got %q", searchedQuery)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		ms := &mockMetadataStore{
			listFunctionsFn: func(ctx context.Context, limit, offset int) ([]*domain.Function, error) {
				if limit != 10 || offset != 5 {
					t.Fatalf("expected limit=10, offset=5, got %d, %d", limit, offset)
				}
				return []*domain.Function{{ID: "1", Name: "fn1"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions?limit=10&offset=5", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("invalid_limit", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("GET", "/functions?limit=abc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid_offset", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("GET", "/functions?offset=abc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			listFunctionsFn: func(ctx context.Context, limit, offset int) ([]*domain.Function, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestGetFunction(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["name"] != "hello" {
			t.Fatalf("unexpected name: %v", resp["name"])
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("function not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestUpdateFunction(t *testing.T) {
	t.Run("success_no_code_change", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateFunctionFn: func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, Runtime: "python", MemoryMB: 256}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"memory_mb":256}`
		req := httptest.NewRequest("PATCH", "/functions/hello", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("publish_version_conflict_retries", func(t *testing.T) {
		publishAttempts := make([]int, 0, 2)
		persistedVersion := 0
		ms := &mockMetadataStore{
			updateFunctionFn: func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
				return &domain.Function{
					ID:       "fn-1",
					Name:     name,
					Runtime:  "python",
					Handler:  "main.handler",
					CodeHash: "hash-v1",
					Version:  1,
				}, nil
			},
			publishVersionFn: func(ctx context.Context, funcID string, version *domain.FunctionVersion) error {
				publishAttempts = append(publishAttempts, version.Version)
				if version.Version == 2 {
					return fmt.Errorf("publish version: version already exists: %s v%d", funcID, version.Version)
				}
				return nil
			},
			saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
				persistedVersion = fn.Version
				return nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"memory_mb":256}`
		req := httptest.NewRequest("PATCH", "/functions/hello", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		if len(publishAttempts) != 2 || publishAttempts[0] != 2 || publishAttempts[1] != 3 {
			t.Fatalf("expected publish attempts [2 3], got %v", publishAttempts)
		}
		if persistedVersion != 3 {
			t.Fatalf("expected persisted version 3, got %d", persistedVersion)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got, ok := resp["version"].(float64); !ok || int(got) != 3 {
			t.Fatalf("expected response version 3, got %v", resp["version"])
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PATCH", "/functions/hello", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateFunctionFn: func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"memory_mb":256}`
		req := httptest.NewRequest("PATCH", "/functions/nonexistent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("invalid_backend", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"backend":"qemu"}`
		req := httptest.NewRequest("PATCH", "/functions/hello", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("empty_backend_normalizes_to_auto", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateFunctionFn: func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
				if update.Backend == nil || *update.Backend != domain.BackendAuto {
					t.Fatalf("expected backend auto, got %#v", update.Backend)
				}
				return &domain.Function{ID: "fn-1", Name: name, Runtime: "python", Backend: domain.BackendAuto}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"backend":""}`
		req := httptest.NewRequest("PATCH", "/functions/hello", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
	})
}

func TestGetFunctionCode(t *testing.T) {
	t.Run("found_with_code", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			getFunctionCodeFn: func(ctx context.Context, funcID string) (*domain.FunctionCode, error) {
				return &domain.FunctionCode{
					FunctionID:    funcID,
					SourceCode:    "print(1)",
					SourceHash:    "abc123",
					CompileStatus: domain.CompileStatusNotRequired,
				}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/code", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["source_code"] != "print(1)" {
			t.Fatalf("unexpected source_code: %v", resp["source_code"])
		}
	})

	t.Run("found_no_code", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/code", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["source_code"] != "" {
			t.Fatalf("expected empty source_code for nil code, got %v", resp["source_code"])
		}
		if resp["compile_status"] != string(domain.CompileStatusPending) {
			t.Fatalf("expected pending compile status, got %v", resp["compile_status"])
		}
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("function not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/nonexistent/code", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("code_store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			getFunctionCodeFn: func(ctx context.Context, funcID string) (*domain.FunctionCode, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/code", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestListFunctionFiles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name, Handler: "handler.py"}, nil
			},
			listFunctionFilesFn: func(ctx context.Context, funcID string) ([]store.FunctionFileInfo, error) {
				return []store.FunctionFileInfo{
					{Path: "handler.py", Size: 100},
					{Path: "utils.py", Size: 50},
				}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/files", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/files", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("function not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/nonexistent/files", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListFunctionVersions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			listVersionsFn: func(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionVersion, error) {
				return []*domain.FunctionVersion{{Version: 1}, {Version: 2}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/versions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("function not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/nonexistent/versions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("empty_versions", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/versions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestGetFunctionVersion(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			getVersionFn: func(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error) {
				return &domain.FunctionVersion{Version: version, FunctionID: funcID}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/versions/1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("function not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/nonexistent/versions/1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("invalid_version_number", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/versions/abc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("version_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			getVersionFn: func(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error) {
				return nil, fmt.Errorf("version not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/functions/hello/versions/99", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestDeleteFunction_FunctionNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("function not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/functions/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateFunctionCode_FunctionNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("function not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"code":"print(1)"}`
	req := httptest.NewRequest("PUT", "/functions/nonexistent/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateFunctionCode_BadJSON(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("PUT", "/functions/hello/code", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateFunctionCode_EmptyCode(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"code":""}`
	req := httptest.NewRequest("PUT", "/functions/hello/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestDetectEntryPoint(t *testing.T) {
	t.Run("python_handler", func(t *testing.T) {
		files := map[string][]byte{
			"handler.py": []byte("def handler(event, ctx): pass"),
			"utils.py":   []byte("# utils"),
		}
		ep := detectEntryPoint(files, domain.RuntimePython)
		if ep != "handler.py" {
			t.Fatalf("expected handler.py, got %s", ep)
		}
	})

	t.Run("node_index", func(t *testing.T) {
		files := map[string][]byte{
			"index.js": []byte("module.exports = {}"),
		}
		ep := detectEntryPoint(files, domain.RuntimeNode)
		if ep != "index.js" {
			t.Fatalf("expected index.js, got %s", ep)
		}
	})

	t.Run("fallback_first_file", func(t *testing.T) {
		files := map[string][]byte{
			"myfile.txt": []byte("content"),
		}
		ep := detectEntryPoint(files, domain.RuntimePython)
		if ep != "myfile.txt" {
			t.Fatalf("expected myfile.txt, got %s", ep)
		}
	})

	t.Run("empty_files", func(t *testing.T) {
		files := map[string][]byte{}
		ep := detectEntryPoint(files, domain.RuntimePython)
		if ep != "handler" {
			t.Fatalf("expected handler, got %s", ep)
		}
	})
}

func TestComputeFilesHash(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		files := map[string][]byte{
			"a.py": []byte("print(1)"),
			"b.py": []byte("print(2)"),
		}
		hash1 := computeFilesHash(files)
		hash2 := computeFilesHash(files)
		if hash1 != hash2 {
			t.Fatalf("hash should be deterministic, got %s vs %s", hash1, hash2)
		}
	})

	t.Run("different_content_different_hash", func(t *testing.T) {
		files1 := map[string][]byte{"a.py": []byte("print(1)")}
		files2 := map[string][]byte{"a.py": []byte("print(2)")}
		hash1 := computeFilesHash(files1)
		hash2 := computeFilesHash(files2)
		if hash1 == hash2 {
			t.Fatal("different content should produce different hashes")
		}
	})
}
