package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

func TestCreateRole(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			createRoleFn: func(ctx context.Context, role *store.RoleRecord) (*store.RoleRecord, error) {
				role.TenantID = "default"
				return role, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"admin","name":"Admin Role"}`
		req := httptest.NewRequest("POST", "/rbac/roles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/rbac/roles", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_fields", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"","name":""}`
		req := httptest.NewRequest("POST", "/rbac/roles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("store_error_conflict", func(t *testing.T) {
		ms := &mockMetadataStore{
			createRoleFn: func(ctx context.Context, role *store.RoleRecord) (*store.RoleRecord, error) {
				return nil, fmt.Errorf("already exists")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"admin","name":"Admin"}`
		req := httptest.NewRequest("POST", "/rbac/roles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})
}

func TestGetRole(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleFn: func(ctx context.Context, id string) (*store.RoleRecord, error) {
				return &store.RoleRecord{ID: id, Name: "Admin"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/roles/admin", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleFn: func(ctx context.Context, id string) (*store.RoleRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/roles/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListRoles(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/roles", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("with_data", func(t *testing.T) {
		ms := &mockMetadataStore{
			listRolesFn: func(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleRecord, error) {
				return []*store.RoleRecord{{ID: "admin", Name: "Admin"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/roles", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestDeleteRole(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/rbac/roles/admin", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteRoleFn: func(ctx context.Context, id string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/rbac/roles/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestCreatePermission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			createPermissionFn: func(ctx context.Context, perm *store.PermissionRecord) (*store.PermissionRecord, error) {
				return perm, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"perm1","code":"functions:read","resource_type":"function","action":"read"}`
		req := httptest.NewRequest("POST", "/rbac/permissions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/rbac/permissions", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_fields", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"","code":""}`
		req := httptest.NewRequest("POST", "/rbac/permissions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestGetPermission(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getPermissionFn: func(ctx context.Context, id string) (*store.PermissionRecord, error) {
				return &store.PermissionRecord{ID: id, Code: "functions:read"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/permissions/perm1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getPermissionFn: func(ctx context.Context, id string) (*store.PermissionRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/permissions/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListPermissions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/permissions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestDeletePermission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/rbac/permissions/perm1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deletePermissionFn: func(ctx context.Context, id string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/rbac/permissions/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListRolePermissions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			listRolePermissionsFn: func(ctx context.Context, roleID string) ([]*store.PermissionRecord, error) {
				return []*store.PermissionRecord{{ID: "p1", Code: "fn:read"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/roles/admin/permissions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			listRolePermissionsFn: func(ctx context.Context, roleID string) ([]*store.PermissionRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/roles/nope/permissions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestAssignPermissionToRole(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleFn: func(ctx context.Context, id string) (*store.RoleRecord, error) {
				return &store.RoleRecord{ID: id}, nil
			},
			getPermissionFn: func(ctx context.Context, id string) (*store.PermissionRecord, error) {
				return &store.PermissionRecord{ID: id}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"permission_id":"perm1"}`
		req := httptest.NewRequest("POST", "/rbac/roles/admin/permissions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/rbac/roles/admin/permissions", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_permission_id", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"permission_id":""}`
		req := httptest.NewRequest("POST", "/rbac/roles/admin/permissions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("role_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleFn: func(ctx context.Context, id string) (*store.RoleRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"permission_id":"perm1"}`
		req := httptest.NewRequest("POST", "/rbac/roles/nonexistent/permissions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("permission_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleFn: func(ctx context.Context, id string) (*store.RoleRecord, error) {
				return &store.RoleRecord{ID: id}, nil
			},
			getPermissionFn: func(ctx context.Context, id string) (*store.PermissionRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"permission_id":"nope"}`
		req := httptest.NewRequest("POST", "/rbac/roles/admin/permissions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestRevokePermissionFromRole(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/rbac/roles/admin/permissions/perm1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["status"] != "revoked" {
			t.Fatalf("unexpected status: %v", resp)
		}
	})
}

func TestCreateRoleAssignment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleFn: func(ctx context.Context, id string) (*store.RoleRecord, error) {
				return &store.RoleRecord{ID: id}, nil
			},
			createRoleAssignmentFn: func(ctx context.Context, ra *store.RoleAssignmentRecord) (*store.RoleAssignmentRecord, error) {
				return ra, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"ra1","principal_type":"user","principal_id":"user1","role_id":"admin","scope_type":"tenant","scope_id":"default"}`
		req := httptest.NewRequest("POST", "/rbac/assignments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/rbac/assignments", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_id", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"","principal_type":"user","principal_id":"user1","role_id":"admin"}`
		req := httptest.NewRequest("POST", "/rbac/assignments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_principal_and_role", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"id":"ra1","principal_type":"user","principal_id":"","role_id":""}`
		req := httptest.NewRequest("POST", "/rbac/assignments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("role_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleFn: func(ctx context.Context, id string) (*store.RoleRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"ra1","principal_type":"user","principal_id":"user1","role_id":"nope"}`
		req := httptest.NewRequest("POST", "/rbac/assignments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestGetRoleAssignment(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleAssignmentFn: func(ctx context.Context, id string) (*store.RoleAssignmentRecord, error) {
				return &store.RoleAssignmentRecord{ID: id, RoleID: "admin"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/assignments/ra1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getRoleAssignmentFn: func(ctx context.Context, id string) (*store.RoleAssignmentRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/assignments/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListRoleAssignments(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		ms := &mockMetadataStore{
			listRoleAssignmentsFn: func(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
				return []*store.RoleAssignmentRecord{{ID: "ra1"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/assignments", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("by_principal", func(t *testing.T) {
		ms := &mockMetadataStore{
			listRoleAssignmentsByPrincipalFn: func(ctx context.Context, tenantID string, pt domain.PrincipalType, pid string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
				return []*store.RoleAssignmentRecord{{ID: "ra1", PrincipalID: pid}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/rbac/assignments?principal_type=user&principal_id=user1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("invalid_principal_type", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("GET", "/rbac/assignments?principal_type=invalid&principal_id=user1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestDeleteRoleAssignment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/rbac/assignments/ra1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteRoleAssignmentFn: func(ctx context.Context, id string) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/rbac/assignments/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestListPermissionBindings(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("GET", "/rbac/permission-bindings", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestGetMyPermissions(t *testing.T) {
	t.Run("no_identity", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("GET", "/rbac/my-permissions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
}

func TestSortStrings(t *testing.T) {
	t.Run("already_sorted", func(t *testing.T) {
		s := []string{"a", "b", "c"}
		sortStrings(s)
		if s[0] != "a" || s[1] != "b" || s[2] != "c" {
			t.Fatalf("unexpected: %v", s)
		}
	})

	t.Run("reverse", func(t *testing.T) {
		s := []string{"c", "b", "a"}
		sortStrings(s)
		if s[0] != "a" || s[1] != "b" || s[2] != "c" {
			t.Fatalf("unexpected: %v", s)
		}
	})

	t.Run("empty", func(t *testing.T) {
		s := []string{}
		sortStrings(s) // should not panic
	})

	t.Run("single", func(t *testing.T) {
		s := []string{"x"}
		sortStrings(s)
		if s[0] != "x" {
			t.Fatalf("unexpected: %v", s)
		}
	})
}

func TestRbacHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"nil", nil, http.StatusOK},
		{"required", fmt.Errorf("field is required"), http.StatusBadRequest},
		{"invalid", fmt.Errorf("invalid value"), http.StatusBadRequest},
		{"not_found", fmt.Errorf("role not found"), http.StatusNotFound},
		{"already_exists", fmt.Errorf("already exists"), http.StatusConflict},
		{"duplicate_key", fmt.Errorf("duplicate key"), http.StatusConflict},
		{"unknown", fmt.Errorf("something went wrong"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rbacHTTPStatus(tt.err)
			if got != tt.expected {
				t.Errorf("rbacHTTPStatus(%v) = %d, want %d", tt.err, got, tt.expected)
			}
		})
	}
}
