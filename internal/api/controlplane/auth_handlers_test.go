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
	"github.com/oriys/nova/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func setupAuthHandler(t *testing.T, ms *mockMetadataStore) (*AuthHandler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	h := &AuthHandler{Store: s, JWTSecret: "test-secret-key-at-least-32-bytes"}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestAuthRegister(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
				return nil, fmt.Errorf("not found")
			},
			createTenantFn: func(_ context.Context, tenant *store.TenantRecord) (*store.TenantRecord, error) {
				return tenant, nil
			},
		}
		_, mux := setupAuthHandler(t, ms)
		body := `{"tenant_id":"test-tenant","password":"password123"}`
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_tenant_id", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		body := `{"password":"password123"}`
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_password", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		body := `{"tenant_id":"test"}`
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("short_password", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		body := `{"tenant_id":"test","password":"short"}`
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("tenant_exists", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
				return &store.TenantRecord{ID: id}, nil
			},
		}
		_, mux := setupAuthHandler(t, ms)
		body := `{"tenant_id":"existing","password":"password123"}`
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusConflict)
	})
}

func TestAuthLogin(t *testing.T) {
	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_tenant_id", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		body := `{"password":"password123"}`
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_password", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		body := `{"tenant_id":"test"}`
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("tenant_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupAuthHandler(t, ms)
		body := `{"tenant_id":"nope","password":"password123"}`
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusUnauthorized)
	})

	t.Run("empty_password_hash", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
				return &store.TenantRecord{ID: id, PasswordHash: ""}, nil
			},
		}
		_, mux := setupAuthHandler(t, ms)
		body := `{"tenant_id":"bootstrap","password":"password123"}`
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusUnauthorized)
	})
}

func TestAuthLogout(t *testing.T) {
	_, mux := setupAuthHandler(t, nil)
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestAuthChangePassword(t *testing.T) {
	t.Run("no_identity", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		body := `{"old_password":"old","new_password":"newpass123"}`
		req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusUnauthorized)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupAuthHandler(t, nil)
		req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		// No identity -> 401 before JSON parsing
		expectStatus(t, w, http.StatusUnauthorized)
	})
}

func TestIsTokenRevoked(t *testing.T) {
	h := &AuthHandler{}
	if h.IsTokenRevoked("nonexistent") {
		t.Fatal("expected false for non-revoked token")
	}

	h.revokeToken("test-token", time.Now().Add(time.Hour))
	if !h.IsTokenRevoked("test-token") {
		t.Fatal("expected true for revoked token")
	}
}

func TestAuthLoginSuccess(t *testing.T) {
	// Hash password "password123" with bcrypt
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	ms := &mockMetadataStore{
		getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
			return &store.TenantRecord{ID: id, PasswordHash: string(hash)}, nil
		},
	}
	_, mux := setupAuthHandler(t, ms)
	body := `{"tenant_id":"mytenant","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestAuthLoginWrongPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	ms := &mockMetadataStore{
		getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
			return &store.TenantRecord{ID: id, PasswordHash: string(hash)}, nil
		},
	}
	_, mux := setupAuthHandler(t, ms)
	body := `{"tenant_id":"mytenant","password":"wrong"}`
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusUnauthorized)
}

func TestAuthLogoutWithIdentity(t *testing.T) {
	_, mux := setupAuthHandler(t, nil)
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "user:tenant1"})
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestChangePassword_WithIdentity(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.MinCost)
	ms := &mockMetadataStore{
		getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
			return &store.TenantRecord{ID: id, PasswordHash: string(hash)}, nil
		},
		setTenantPasswordHashFn: func(_ context.Context, id, pw string) error { return nil },
	}
	_, mux := setupAuthHandler(t, ms)
	body := `{"old_password":"oldpass123","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader(body))
	ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "user:mytenant"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestChangePassword_BadJSON(t *testing.T) {
	_, mux := setupAuthHandler(t, nil)
	req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader("{bad"))
	ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "user:mytenant"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestChangePassword_ShortPassword(t *testing.T) {
	_, mux := setupAuthHandler(t, nil)
	body := `{"old_password":"oldpass123","new_password":"short"}`
	req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader(body))
	ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "user:mytenant"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestChangePassword_MissingFields(t *testing.T) {
	_, mux := setupAuthHandler(t, nil)
	body := `{"old_password":"","new_password":""}`
	req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader(body))
	ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "user:mytenant"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestChangePassword_InvalidSubject(t *testing.T) {
	_, mux := setupAuthHandler(t, nil)
	body := `{"old_password":"oldpass123","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader(body))
	ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "noprefixsubject"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.MinCost)
	ms := &mockMetadataStore{
		getTenantFn: func(_ context.Context, id string) (*store.TenantRecord, error) {
			return &store.TenantRecord{ID: id, PasswordHash: string(hash)}, nil
		},
	}
	_, mux := setupAuthHandler(t, ms)
	body := `{"old_password":"wrong","new_password":"newpass123"}`
	req := httptest.NewRequest("POST", "/auth/change-password", strings.NewReader(body))
	ctx := auth.WithIdentity(req.Context(), &auth.Identity{Subject: "user:mytenant"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusUnauthorized)
}
