package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/store"
)

func TestEnforceTenantAccess(t *testing.T) {
	t.Run("no_identity", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		if !enforceTenantAccess(w, r, "tenant-1") {
			t.Fatal("expected true when no identity")
		}
	})

	t.Run("unrestricted_identity", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		ctx := auth.WithIdentity(r.Context(), &auth.Identity{Subject: "user"})
		r = r.WithContext(ctx)
		if !enforceTenantAccess(w, r, "tenant-1") {
			t.Fatal("expected true when no scope restrictions")
		}
	})

	t.Run("allowed_tenant", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		ctx := auth.WithIdentity(r.Context(), &auth.Identity{
			Subject:       "user",
			AllowedScopes: []auth.TenantScope{{TenantID: "tenant-1"}},
		})
		r = r.WithContext(ctx)
		if !enforceTenantAccess(w, r, "tenant-1") {
			t.Fatal("expected true for allowed tenant")
		}
	})

	t.Run("wildcard_tenant", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		ctx := auth.WithIdentity(r.Context(), &auth.Identity{
			Subject:       "user",
			AllowedScopes: []auth.TenantScope{{TenantID: "*"}},
		})
		r = r.WithContext(ctx)
		if !enforceTenantAccess(w, r, "any-tenant") {
			t.Fatal("expected true for wildcard")
		}
	})

	t.Run("forbidden_tenant", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		ctx := auth.WithIdentity(r.Context(), &auth.Identity{
			Subject:       "user",
			AllowedScopes: []auth.TenantScope{{TenantID: "tenant-1"}},
		})
		r = r.WithContext(ctx)
		if enforceTenantAccess(w, r, "other-tenant") {
			t.Fatal("expected false for forbidden tenant")
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	})
}

func TestEnforceNamespaceAccess(t *testing.T) {
	t.Run("no_identity", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		if !enforceNamespaceAccess(w, r, "t1", "ns1") {
			t.Fatal("expected true")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		ctx := auth.WithIdentity(r.Context(), &auth.Identity{
			Subject:       "user",
			AllowedScopes: []auth.TenantScope{{TenantID: "t1", Namespace: "ns-allowed"}},
		})
		r = r.WithContext(ctx)
		if enforceNamespaceAccess(w, r, "t1", "ns-other") {
			t.Fatal("expected false for forbidden namespace")
		}
	})
}

func TestVisibleTenantIDs(t *testing.T) {
	t.Run("nil_identity", func(t *testing.T) {
		ids, all := visibleTenantIDs(nil)
		if !all || ids != nil {
			t.Fatal("expected all=true, ids=nil")
		}
	})

	t.Run("unrestricted", func(t *testing.T) {
		ids, all := visibleTenantIDs(&auth.Identity{Subject: "user"})
		if !all || ids != nil {
			t.Fatal("expected all=true")
		}
	})

	t.Run("wildcard", func(t *testing.T) {
		ids, all := visibleTenantIDs(&auth.Identity{
			Subject:       "user",
			AllowedScopes: []auth.TenantScope{{TenantID: "*"}},
		})
		if !all || ids != nil {
			t.Fatal("expected all=true for wildcard")
		}
	})

	t.Run("specific_tenants", func(t *testing.T) {
		ids, all := visibleTenantIDs(&auth.Identity{
			Subject: "user",
			AllowedScopes: []auth.TenantScope{
				{TenantID: "t1"},
				{TenantID: "t2"},
			},
		})
		if all {
			t.Fatal("expected all=false")
		}
		if len(ids) != 2 {
			t.Fatalf("expected 2 tenants, got %d", len(ids))
		}
	})
}

func TestFilterVisibleNamespaces(t *testing.T) {
	t.Run("nil_identity", func(t *testing.T) {
		nss := []*store.NamespaceRecord{{Name: "ns1"}}
		result := filterVisibleNamespaces(nil, "t1", nss)
		if len(result) != 1 {
			t.Fatal("expected all namespaces")
		}
	})

	t.Run("filtered", func(t *testing.T) {
		identity := &auth.Identity{
			Subject:       "user",
			AllowedScopes: []auth.TenantScope{{TenantID: "t1", Namespace: "ns-allowed"}},
		}
		nss := []*store.NamespaceRecord{
			{Name: "ns-allowed"},
			{Name: "ns-other"},
		}
		result := filterVisibleNamespaces(identity, "t1", nss)
		if len(result) != 1 || result[0].Name != "ns-allowed" {
			t.Fatalf("expected 1 allowed namespace, got %d", len(result))
		}
	})
}

func TestVisibleTenantIDs_DuplicateTenants(t *testing.T) {
	ids, all := visibleTenantIDs(&auth.Identity{
		Subject: "user",
		AllowedScopes: []auth.TenantScope{
			{TenantID: "t1"},
			{TenantID: "t1"},
			{TenantID: "t2"},
		},
	})
	if all {
		t.Fatal("expected all=false")
	}
	// Deduplicated
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique tenants, got %d", len(ids))
	}
}

func withIdentityCtx(r *http.Request, identity *auth.Identity) *http.Request {
	return r.WithContext(auth.WithIdentity(r.Context(), identity))
}
