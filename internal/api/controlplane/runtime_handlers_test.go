package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/store"
)

func TestListRuntimes(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/runtimes", nil)
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
			listRuntimesFn: func(ctx context.Context, limit, offset int) ([]*store.RuntimeRecord, error) {
				return []*store.RuntimeRecord{
					{ID: "python", Name: "python", Version: "3.11", Status: "available"},
					{ID: "node", Name: "node", Version: "20", Status: "available"},
				}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/runtimes", nil)
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
			t.Fatalf("expected 2 runtimes, got %d", len(items))
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			listRuntimesFn: func(ctx context.Context, limit, offset int) ([]*store.RuntimeRecord, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/runtimes", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestCreateRuntime(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		body := `{
			"id":"custom-py",
			"name":"custom-python",
			"version":"3.12",
			"image_name":"custom-python.ext4",
			"entrypoint":["python3"],
			"file_extension":".py"
		}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var rt map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&rt); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if rt["id"] != "custom-py" {
			t.Fatalf("unexpected id: %v", rt["id"])
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_id", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"name":"test","image_name":"test.ext4","entrypoint":["python3"],"file_extension":".py"}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_name", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"test","image_name":"test.ext4","entrypoint":["python3"],"file_extension":".py"}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_image_name", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"test","name":"test","entrypoint":["python3"],"file_extension":".py"}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_entrypoint", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"test","name":"test","image_name":"test.ext4","file_extension":".py"}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_file_extension", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"test","name":"test","image_name":"test.ext4","entrypoint":["python3"]}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("defaults_version_and_status", func(t *testing.T) {
		var saved *store.RuntimeRecord
		ms := &mockMetadataStore{
			saveRuntimeFn: func(ctx context.Context, rt *store.RuntimeRecord) error {
				saved = rt
				return nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"test","name":"test","image_name":"test.ext4","entrypoint":["python3"],"file_extension":".py"}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		if saved == nil {
			t.Fatal("expected runtime to be saved")
		}
		if saved.Version != "dynamic" {
			t.Fatalf("expected default version 'dynamic', got %q", saved.Version)
		}
		if saved.Status != "available" {
			t.Fatalf("expected default status 'available', got %q", saved.Status)
		}
	})

	t.Run("save_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			saveRuntimeFn: func(ctx context.Context, rt *store.RuntimeRecord) error {
				return fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"test","name":"test","image_name":"test.ext4","entrypoint":["python3"],"file_extension":".py"}`
		req := httptest.NewRequest("POST", "/runtimes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestDeleteRuntime(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/runtimes/python", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteRuntimeFn: func(ctx context.Context, id string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/runtimes/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestUploadRuntime(t *testing.T) {
	t.Run("no_rootfs_dir", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/runtimes/upload", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "rootfs directory not configured") {
			t.Fatalf("unexpected body: %s", w.Body.String())
		}
	})
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"python", "python"},
		{"my-runtime_v2", "my-runtime_v2"},
		{"../../etc/passwd", "passwd"},
		{"bad chars!@#$", "badchars"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidateExt4Header(t *testing.T) {
	t.Run("too_short", func(t *testing.T) {
		r := strings.NewReader("short")
		if validateExt4Header(r) {
			t.Fatal("expected false for too-short data")
		}
	})

	t.Run("valid_magic", func(t *testing.T) {
		// Create buffer with ext4 magic at offset 1024+0x38
		buf := make([]byte, 1024+0x38+2)
		buf[1024+0x38] = 0x53 // 0xEF53 little endian
		buf[1024+0x38+1] = 0xEF
		r := bytes.NewReader(buf)
		if !validateExt4Header(r) {
			t.Fatal("expected true for valid ext4 magic")
		}
	})

	t.Run("invalid_magic", func(t *testing.T) {
		buf := make([]byte, 1024+0x38+2)
		buf[1024+0x38] = 0x00
		buf[1024+0x38+1] = 0x00
		r := bytes.NewReader(buf)
		if validateExt4Header(r) {
			t.Fatal("expected false for invalid magic")
		}
	})
}

func TestDeleteRuntime_Error(t *testing.T) {
	ms := &mockMetadataStore{
		deleteRuntimeFn: func(_ context.Context, id string) error { return fmt.Errorf("db error") },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/runtimes/python", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}
