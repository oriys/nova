package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/secrets"
)

func setupSecretHandler(t *testing.T, backend *mockSecretsBackend) *http.ServeMux {
	t.Helper()
	if backend == nil {
		backend = &mockSecretsBackend{}
	}
	cipher, err := secrets.NewCipher("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	ss := secrets.NewStore(backend, cipher)
	sh := &SecretHandler{Store: ss}
	mux := http.NewServeMux()
	sh.RegisterRoutes(mux)
	return mux
}

func TestCreateSecret(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux := setupSecretHandler(t, &mockSecretsBackend{})
		body := `{"name":"MY_SECRET","value":"supersecret"}`
		req := httptest.NewRequest("POST", "/secrets", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("bad_json", func(t *testing.T) {
		mux := setupSecretHandler(t, nil)
		req := httptest.NewRequest("POST", "/secrets", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_name", func(t *testing.T) {
		mux := setupSecretHandler(t, nil)
		body := `{"value":"supersecret"}`
		req := httptest.NewRequest("POST", "/secrets", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_value", func(t *testing.T) {
		mux := setupSecretHandler(t, nil)
		body := `{"name":"MY_SECRET"}`
		req := httptest.NewRequest("POST", "/secrets", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("store_error", func(t *testing.T) {
		backend := &mockSecretsBackend{
			saveFn: func(_ context.Context, name, encValue string) error {
				return fmt.Errorf("db error")
			},
		}
		mux := setupSecretHandler(t, backend)
		body := `{"name":"MY_SECRET","value":"supersecret"}`
		req := httptest.NewRequest("POST", "/secrets", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusInternalServerError)
	})
}

func TestListSecrets(t *testing.T) {
	backend := &mockSecretsBackend{
		listFn: func(_ context.Context) (map[string]string, error) {
			return map[string]string{"KEY1": "2024-01-01T00:00:00Z", "KEY2": "2024-01-02T00:00:00Z"}, nil
		},
	}
	mux := setupSecretHandler(t, backend)
	req := httptest.NewRequest("GET", "/secrets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListSecrets_Error(t *testing.T) {
	backend := &mockSecretsBackend{
		listFn: func(_ context.Context) (map[string]string, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	mux := setupSecretHandler(t, backend)
	req := httptest.NewRequest("GET", "/secrets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestDeleteSecret(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux := setupSecretHandler(t, &mockSecretsBackend{})
		req := httptest.NewRequest("DELETE", "/secrets/MY_SECRET", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("error", func(t *testing.T) {
		backend := &mockSecretsBackend{
			deleteFn: func(_ context.Context, name string) error {
				return fmt.Errorf("not found")
			},
		}
		mux := setupSecretHandler(t, backend)
		req := httptest.NewRequest("DELETE", "/secrets/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusInternalServerError)
	})
}

func TestDeleteSecret_Error(t *testing.T) {
	backend := &mockSecretsBackend{
		deleteFn: func(_ context.Context, name string) error { return fmt.Errorf("not found") },
	}
	mux := setupSecretHandler(t, backend)
	req := httptest.NewRequest("DELETE", "/secrets/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusOK || w.Code == http.StatusNoContent {
		t.Fatal("expected error status")
	}
}
