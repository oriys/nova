package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/store"
)

func TestTenantScopeMiddlewareDefaultsWhenNoIdentityAndNoHeaders(t *testing.T) {
	scope, status := runTenantScopeMiddleware(t, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", status)
	}
	if scope.TenantID != store.DefaultTenantID || scope.Namespace != store.DefaultNamespace {
		t.Fatalf("unexpected default scope: %+v", scope)
	}
}

func TestTenantScopeMiddlewareUsesPrimaryScopeForScopedIdentity(t *testing.T) {
	identity := &auth.Identity{
		Subject: "apikey:test",
		AllowedScopes: []auth.TenantScope{
			{TenantID: "team-a", Namespace: "prod"},
		},
	}

	scope, status := runTenantScopeMiddleware(t, identity, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", status)
	}
	if scope.TenantID != "team-a" || scope.Namespace != "prod" {
		t.Fatalf("unexpected scope: %+v", scope)
	}
}

func TestTenantScopeMiddlewareRejectsOutOfScopeHeaders(t *testing.T) {
	identity := &auth.Identity{
		Subject: "apikey:test",
		AllowedScopes: []auth.TenantScope{
			{TenantID: "team-a", Namespace: "prod"},
		},
	}

	_, status := runTenantScopeMiddleware(t, identity, map[string]string{
		"X-Nova-Tenant":    "team-b",
		"X-Nova-Namespace": "prod",
	})
	if status != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", status)
	}
}

func TestTenantScopeMiddlewareAllowsInScopeHeaders(t *testing.T) {
	identity := &auth.Identity{
		Subject: "user:test",
		AllowedScopes: []auth.TenantScope{
			{TenantID: "team-a", Namespace: "*"},
		},
	}

	scope, status := runTenantScopeMiddleware(t, identity, map[string]string{
		"X-Nova-Tenant":    "team-a",
		"X-Nova-Namespace": "dev",
	})
	if status != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", status)
	}
	if scope.TenantID != "team-a" || scope.Namespace != "dev" {
		t.Fatalf("unexpected scope: %+v", scope)
	}
}

func TestTenantScopeMiddlewareRequiresExplicitScopeForWildcardPrimary(t *testing.T) {
	identity := &auth.Identity{
		Subject: "user:test",
		AllowedScopes: []auth.TenantScope{
			{TenantID: "team-a", Namespace: "*"},
		},
	}

	_, status := runTenantScopeMiddleware(t, identity, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", status)
	}
}

func TestTenantScopeMiddlewareRejectsInvalidHeaders(t *testing.T) {
	_, status := runTenantScopeMiddleware(t, nil, map[string]string{
		"X-Nova-Tenant": "bad tenant",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", status)
	}
}

func runTenantScopeMiddleware(t *testing.T, identity *auth.Identity, headers map[string]string) (store.TenantScope, int) {
	t.Helper()

	var resolved store.TenantScope
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = store.TenantScopeFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})

	handler := tenantScopeMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/functions", nil)
	if identity != nil {
		req = req.WithContext(auth.WithIdentity(req.Context(), identity))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return resolved, rr.Code
}
