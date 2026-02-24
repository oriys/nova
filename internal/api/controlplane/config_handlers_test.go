package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getConfigFn: func(ctx context.Context) (map[string]string, error) {
				return map[string]string{"max_global_vms": "10"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/config", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var cfg map[string]string
		if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if cfg["max_global_vms"] != "10" {
			t.Fatalf("unexpected config: %v", cfg)
		}
	})

	t.Run("nil_config_returns_empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/config", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var cfg map[string]string
		if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(cfg) != 0 {
			t.Fatalf("expected empty config, got %v", cfg)
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getConfigFn: func(ctx context.Context) (map[string]string, error) {
				return nil, fmt.Errorf("db down")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/config", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestUpdateConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var setKeys []string
		ms := &mockMetadataStore{
			setConfigFn: func(ctx context.Context, key, value string) error {
				setKeys = append(setKeys, key+"="+value)
				return nil
			},
			getConfigFn: func(ctx context.Context) (map[string]string, error) {
				return map[string]string{"foo": "bar"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"foo":"bar"}`
		req := httptest.NewRequest("PUT", "/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		if len(setKeys) != 1 || setKeys[0] != "foo=bar" {
			t.Fatalf("unexpected setKeys: %v", setKeys)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PUT", "/config", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("set_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			setConfigFn: func(ctx context.Context, key, value string) error {
				return fmt.Errorf("write failed")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"key":"val"}`
		req := httptest.NewRequest("PUT", "/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}
