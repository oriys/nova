package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/store"
)

func TestListTenants(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants", nil)
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
			listTenantsFn: func(ctx context.Context, limit, offset int) ([]*store.TenantRecord, error) {
				return []*store.TenantRecord{
					{ID: "t1", Name: "Tenant 1"},
					{ID: "t2", Name: "Tenant 2"},
				}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants", nil)
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
			t.Fatalf("expected 2 tenants, got %d", len(items))
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			listTenantsFn: func(ctx context.Context, limit, offset int) ([]*store.TenantRecord, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		ms := &mockMetadataStore{
			listTenantsFn: func(ctx context.Context, limit, offset int) ([]*store.TenantRecord, error) {
				if limit != 10 || offset != 5 {
					t.Fatalf("expected limit=10, offset=5, got %d, %d", limit, offset)
				}
				return []*store.TenantRecord{{ID: "t1"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants?limit=10&offset=5", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestCreateTenant(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			createTenantFn: func(ctx context.Context, tenant *store.TenantRecord) (*store.TenantRecord, error) {
				tenant.Status = "active"
				return tenant, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"newt","name":"New Tenant"}`
		req := httptest.NewRequest("POST", "/tenants", strings.NewReader(body))
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
		if resp["id"] != "newt" {
			t.Fatalf("unexpected id: %v", resp["id"])
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/tenants", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		ms := &mockMetadataStore{
			createTenantFn: func(ctx context.Context, tenant *store.TenantRecord) (*store.TenantRecord, error) {
				return nil, fmt.Errorf("already exists")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"existing","name":"Existing"}`
		req := httptest.NewRequest("POST", "/tenants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})
}

func TestUpdateTenant(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateTenantFn: func(ctx context.Context, id string, update *store.TenantUpdate) (*store.TenantRecord, error) {
				return &store.TenantRecord{ID: id, Name: "Updated"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"Updated"}`
		req := httptest.NewRequest("PATCH", "/tenants/t1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PATCH", "/tenants/t1", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateTenantFn: func(ctx context.Context, id string, update *store.TenantUpdate) (*store.TenantRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"Updated"}`
		req := httptest.NewRequest("PATCH", "/tenants/nope", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestDeleteTenant(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["status"] != "deleted" {
			t.Fatalf("unexpected status: %v", resp)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteTenantFn: func(ctx context.Context, id string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListNamespaces(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			listNamespacesFn: func(ctx context.Context, tenantID string, limit, offset int) ([]*store.NamespaceRecord, error) {
				return []*store.NamespaceRecord{{TenantID: tenantID, Name: "default"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/namespaces", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/namespaces", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestCreateNamespace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			createNamespaceFn: func(ctx context.Context, namespace *store.NamespaceRecord) (*store.NamespaceRecord, error) {
				return namespace, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"staging"}`
		req := httptest.NewRequest("POST", "/tenants/t1/namespaces", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/tenants/t1/namespaces", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			createNamespaceFn: func(ctx context.Context, namespace *store.NamespaceRecord) (*store.NamespaceRecord, error) {
				return nil, fmt.Errorf("already exists")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"default"}`
		req := httptest.NewRequest("POST", "/tenants/t1/namespaces", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})
}

func TestUpdateNamespace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateNamespaceFn: func(ctx context.Context, tenantID, name string, update *store.NamespaceUpdate) (*store.NamespaceRecord, error) {
				return &store.NamespaceRecord{TenantID: tenantID, Name: "renamed"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"renamed"}`
		req := httptest.NewRequest("PATCH", "/tenants/t1/namespaces/default", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PATCH", "/tenants/t1/namespaces/default", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestDeleteNamespace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/namespaces/staging", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteNamespaceFn: func(ctx context.Context, tenantID, name string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/namespaces/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListTenantQuotas(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/quotas", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("with_data", func(t *testing.T) {
		ms := &mockMetadataStore{
			listTenantQuotasFn: func(ctx context.Context, tenantID string) ([]*store.TenantQuotaRecord, error) {
				return []*store.TenantQuotaRecord{{TenantID: tenantID, Dimension: "invocations", HardLimit: 1000}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/quotas", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestUpsertTenantQuota(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			upsertTenantQuotaFn: func(ctx context.Context, quota *store.TenantQuotaRecord) (*store.TenantQuotaRecord, error) {
				return quota, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"hard_limit":1000,"soft_limit":900}`
		req := httptest.NewRequest("PUT", "/tenants/t1/quotas/invocations", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PUT", "/tenants/t1/quotas/invocations", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestDeleteTenantQuota(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/quotas/invocations", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteTenantQuotaFn: func(ctx context.Context, tenantID, dimension string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/quotas/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestGetTenantUsage(t *testing.T) {
	t.Run("with_refresh", func(t *testing.T) {
		ms := &mockMetadataStore{
			refreshTenantUsageFn: func(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error) {
				return []*store.TenantUsageRecord{{TenantID: tenantID, Dimension: "invocations"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/usage", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("without_refresh", func(t *testing.T) {
		ms := &mockMetadataStore{
			listTenantUsageFn: func(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error) {
				return []*store.TenantUsageRecord{}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/usage?refresh=false", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestListTenantMenuPermissions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/menu-permissions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("with_data", func(t *testing.T) {
		ms := &mockMetadataStore{
			listTenantMenuPermissionsFn: func(ctx context.Context, tenantID string) ([]*store.MenuPermissionRecord, error) {
				return []*store.MenuPermissionRecord{{TenantID: tenantID, MenuKey: "functions", Enabled: true}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/menu-permissions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestUpsertTenantMenuPermission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			upsertTenantMenuPermissionFn: func(ctx context.Context, tenantID, menuKey string, enabled bool) (*store.MenuPermissionRecord, error) {
				return &store.MenuPermissionRecord{TenantID: tenantID, MenuKey: menuKey, Enabled: enabled}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PUT", "/tenants/t1/menu-permissions/functions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PUT", "/tenants/t1/menu-permissions/functions", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestDeleteTenantMenuPermission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/menu-permissions/functions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteTenantMenuPermissionFn: func(ctx context.Context, tenantID, menuKey string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/menu-permissions/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListTenantButtonPermissions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/tenants/t1/button-permissions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestUpsertTenantButtonPermission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			upsertTenantButtonPermissionFn: func(ctx context.Context, tenantID, permKey string, enabled bool) (*store.ButtonPermissionRecord, error) {
				return &store.ButtonPermissionRecord{TenantID: tenantID, PermissionKey: permKey, Enabled: enabled}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PUT", "/tenants/t1/button-permissions/functions.create", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PUT", "/tenants/t1/button-permissions/functions.create", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestDeleteTenantButtonPermission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/button-permissions/functions.create", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteTenantButtonPermissionFn: func(ctx context.Context, tenantID, permKey string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/tenants/t1/button-permissions/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestTenancyHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"nil", nil, http.StatusOK},
		{"required", fmt.Errorf("name is required"), http.StatusBadRequest},
		{"not_found", fmt.Errorf("tenant not found"), http.StatusNotFound},
		{"already_exists", fmt.Errorf("tenant already exists"), http.StatusConflict},
		{"duplicate_key", fmt.Errorf("duplicate key"), http.StatusConflict},
		{"unknown", fmt.Errorf("something else"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tenancyHTTPStatus(tt.err)
			if got != tt.expected {
				t.Errorf("tenancyHTTPStatus(%v) = %d, want %d", tt.err, got, tt.expected)
			}
		})
	}
}

func TestListTenants_WithIdentity(t *testing.T) {
	ms := &mockMetadataStore{
		getTenantFn: func(ctx context.Context, id string) (*store.TenantRecord, error) {
			return &store.TenantRecord{ID: id, Name: "Tenant " + id}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)

	identity := &auth.Identity{
		Subject: "user1",
		AllowedScopes: []auth.TenantScope{
			{TenantID: "t1"},
			{TenantID: "t2"},
		},
	}
	ctx := auth.WithIdentity(context.Background(), identity)
	req := httptest.NewRequest("GET", "/tenants", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestListTenants_WithIdentity_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getTenantFn: func(ctx context.Context, id string) (*store.TenantRecord, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)

	identity := &auth.Identity{
		Subject: "user1",
		AllowedScopes: []auth.TenantScope{
			{TenantID: "t1"},
		},
	}
	ctx := auth.WithIdentity(context.Background(), identity)
	req := httptest.NewRequest("GET", "/tenants", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Tenant not found gets skipped, returns empty
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
	}
}
