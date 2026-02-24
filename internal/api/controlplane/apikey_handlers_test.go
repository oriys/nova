package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oriys/nova/internal/auth"
)

func setupAPIKeyHandler(t *testing.T, akStore *mockAPIKeyStore) *http.ServeMux {
	t.Helper()
	if akStore == nil {
		akStore = &mockAPIKeyStore{}
	}
	mgr := auth.NewAPIKeyManager(akStore)
	h := &APIKeyHandler{Manager: mgr}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestCreateAPIKey(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return nil, fmt.Errorf("not found")
			},
			saveFn: func(_ context.Context, key *auth.APIKey) error { return nil },
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"name":"test-key"}`
		req := httptest.NewRequest("POST", "/apikeys", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("bad_json", func(t *testing.T) {
		mux := setupAPIKeyHandler(t, nil)
		req := httptest.NewRequest("POST", "/apikeys", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_name", func(t *testing.T) {
		mux := setupAPIKeyHandler(t, nil)
		body := `{"tier":"premium"}`
		req := httptest.NewRequest("POST", "/apikeys", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("duplicate_name", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return &auth.APIKey{Name: name}, nil
			},
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"name":"existing"}`
		req := httptest.NewRequest("POST", "/apikeys", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusConflict)
	})
}

func TestListAPIKeys(t *testing.T) {
	akStore := &mockAPIKeyStore{
		listFn: func(_ context.Context) ([]*auth.APIKey, error) {
			return []*auth.APIKey{{Name: "k1", Enabled: true, CreatedAt: time.Now()}}, nil
		},
	}
	mux := setupAPIKeyHandler(t, akStore)
	req := httptest.NewRequest("GET", "/apikeys", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListAPIKeys_Error(t *testing.T) {
	akStore := &mockAPIKeyStore{
		listFn: func(_ context.Context) ([]*auth.APIKey, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	mux := setupAPIKeyHandler(t, akStore)
	req := httptest.NewRequest("GET", "/apikeys", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestDeleteAPIKey(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		akStore := &mockAPIKeyStore{}
		mux := setupAPIKeyHandler(t, akStore)
		req := httptest.NewRequest("DELETE", "/apikeys/test-key", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			deleteFn: func(_ context.Context, name string) error {
				return fmt.Errorf("not found")
			},
		}
		mux := setupAPIKeyHandler(t, akStore)
		req := httptest.NewRequest("DELETE", "/apikeys/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestToggleAPIKey(t *testing.T) {
	t.Run("disable", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return &auth.APIKey{Name: name, Enabled: true}, nil
			},
			saveFn: func(_ context.Context, key *auth.APIKey) error { return nil },
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"enabled":false}`
		req := httptest.NewRequest("PATCH", "/apikeys/test-key", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("enable", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return &auth.APIKey{Name: name, Enabled: false}, nil
			},
			saveFn: func(_ context.Context, key *auth.APIKey) error { return nil },
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PATCH", "/apikeys/test-key", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("bad_json", func(t *testing.T) {
		mux := setupAPIKeyHandler(t, nil)
		req := httptest.NewRequest("PATCH", "/apikeys/test-key", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("key_not_found", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PATCH", "/apikeys/nope", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == http.StatusOK {
			t.Fatal("expected error for not found key")
		}
	})

	t.Run("permissions_update", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return &auth.APIKey{Name: name, Enabled: true}, nil
			},
			saveFn: func(_ context.Context, key *auth.APIKey) error { return nil },
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"permissions":[{"resource":"functions","actions":["read"]}]}`
		req := httptest.NewRequest("PATCH", "/apikeys/test-key", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("enable_error", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PATCH", "/apikeys/test-key", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})

	t.Run("revoke_error_conflict", func(t *testing.T) {
		akStore := &mockAPIKeyStore{
			getByNameFn: func(_ context.Context, name string) (*auth.APIKey, error) {
				return nil, fmt.Errorf("already exists")
			},
		}
		mux := setupAPIKeyHandler(t, akStore)
		body := `{"enabled":false}`
		req := httptest.NewRequest("PATCH", "/apikeys/test-key", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusConflict)
	})
}

func TestDeleteAPIKey_Error(t *testing.T) {
	akStore := &mockAPIKeyStore{
		deleteFn: func(_ context.Context, name string) error {
			return fmt.Errorf("not found")
		},
	}
	mux := setupAPIKeyHandler(t, akStore)
	req := httptest.NewRequest("DELETE", "/apikeys/test-key", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}
