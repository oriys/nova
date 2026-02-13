package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// RBACHandler handles RBAC management endpoints.
type RBACHandler struct {
	Store *store.Store
}

func (h *RBACHandler) RegisterRoutes(mux *http.ServeMux) {
	// Roles
	mux.HandleFunc("POST /rbac/roles", h.CreateRole)
	mux.HandleFunc("GET /rbac/roles", h.ListRoles)
	mux.HandleFunc("GET /rbac/roles/{id}", h.GetRole)
	mux.HandleFunc("DELETE /rbac/roles/{id}", h.DeleteRole)

	// Permissions
	mux.HandleFunc("POST /rbac/permissions", h.CreatePermission)
	mux.HandleFunc("GET /rbac/permissions", h.ListPermissions)
	mux.HandleFunc("GET /rbac/permissions/{id}", h.GetPermission)
	mux.HandleFunc("DELETE /rbac/permissions/{id}", h.DeletePermission)

	// Role ↔ Permission mapping
	mux.HandleFunc("GET /rbac/roles/{id}/permissions", h.ListRolePermissions)
	mux.HandleFunc("POST /rbac/roles/{id}/permissions", h.AssignPermissionToRole)
	mux.HandleFunc("DELETE /rbac/roles/{roleId}/permissions/{permId}", h.RevokePermissionFromRole)

	// Role Assignments
	mux.HandleFunc("POST /rbac/assignments", h.CreateRoleAssignment)
	mux.HandleFunc("GET /rbac/assignments", h.ListRoleAssignments)
	mux.HandleFunc("GET /rbac/assignments/{id}", h.GetRoleAssignment)
	mux.HandleFunc("DELETE /rbac/assignments/{id}", h.DeleteRoleAssignment)
}

// ─── Roles ──────────────────────────────────────────────────────────────────

func (h *RBACHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		TenantID string `json:"tenant_id"`
		Name     string `json:"name"`
		IsSystem bool   `json:"is_system"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "id and name are required", http.StatusBadRequest)
		return
	}

	role, err := h.Store.CreateRole(r.Context(), &store.RoleRecord{
		ID:       req.ID,
		TenantID: req.TenantID,
		Name:     req.Name,
		IsSystem: req.IsSystem,
	})
	if err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(role)
}

func (h *RBACHandler) GetRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	role, err := h.Store.GetRole(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(role)
}

func (h *RBACHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	roles, err := h.Store.ListRoles(r.Context(), tenantID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if roles == nil {
		roles = []*store.RoleRecord{}
	}
	total := estimatePaginatedTotal(limit, offset, len(roles))
	writePaginatedList(w, limit, offset, len(roles), total, roles)
}

func (h *RBACHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteRole(r.Context(), id); err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": id})
}

// ─── Permissions ────────────────────────────────────────────────────────────

func (h *RBACHandler) CreatePermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID           string `json:"id"`
		Code         string `json:"code"`
		ResourceType string `json:"resource_type"`
		Action       string `json:"action"`
		Description  string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Code) == "" {
		http.Error(w, "id and code are required", http.StatusBadRequest)
		return
	}

	perm, err := h.Store.CreatePermission(r.Context(), &store.PermissionRecord{
		ID:           req.ID,
		Code:         req.Code,
		ResourceType: req.ResourceType,
		Action:       req.Action,
		Description:  req.Description,
	})
	if err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(perm)
}

func (h *RBACHandler) GetPermission(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	perm, err := h.Store.GetPermission(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(perm)
}

func (h *RBACHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	perms, err := h.Store.ListPermissions(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if perms == nil {
		perms = []*store.PermissionRecord{}
	}
	total := estimatePaginatedTotal(limit, offset, len(perms))
	writePaginatedList(w, limit, offset, len(perms), total, perms)
}

func (h *RBACHandler) DeletePermission(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeletePermission(r.Context(), id); err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": id})
}

// ─── Role ↔ Permission ─────────────────────────────────────────────────────

func (h *RBACHandler) ListRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)
	perms, err := h.Store.ListRolePermissions(r.Context(), roleID)
	if err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	if perms == nil {
		perms = []*store.PermissionRecord{}
	}
	pagedPerms, total := paginateSliceWindow(perms, limit, offset)
	writePaginatedList(w, limit, offset, len(pagedPerms), int64(total), pagedPerms)
}

func (h *RBACHandler) AssignPermissionToRole(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("id")
	var req struct {
		PermissionID string `json:"permission_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.PermissionID) == "" {
		http.Error(w, "permission_id is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.AssignPermissionToRole(r.Context(), roleID, req.PermissionID); err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "assigned", "role_id": roleID, "permission_id": req.PermissionID})
}

func (h *RBACHandler) RevokePermissionFromRole(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("roleId")
	permID := r.PathValue("permId")
	if err := h.Store.RevokePermissionFromRole(r.Context(), roleID, permID); err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked", "role_id": roleID, "permission_id": permID})
}

// ─── Role Assignments ───────────────────────────────────────────────────────

func (h *RBACHandler) CreateRoleAssignment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            string `json:"id"`
		TenantID      string `json:"tenant_id"`
		PrincipalType string `json:"principal_type"`
		PrincipalID   string `json:"principal_id"`
		RoleID        string `json:"role_id"`
		ScopeType     string `json:"scope_type"`
		ScopeID       string `json:"scope_id"`
		CreatedBy     string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.PrincipalID) == "" || strings.TrimSpace(req.RoleID) == "" {
		http.Error(w, "principal_id and role_id are required", http.StatusBadRequest)
		return
	}

	ra, err := h.Store.CreateRoleAssignment(r.Context(), &store.RoleAssignmentRecord{
		ID:            req.ID,
		TenantID:      req.TenantID,
		PrincipalType: domain.PrincipalType(req.PrincipalType),
		PrincipalID:   req.PrincipalID,
		RoleID:        req.RoleID,
		ScopeType:     domain.ScopeType(req.ScopeType),
		ScopeID:       req.ScopeID,
		CreatedBy:     req.CreatedBy,
	})
	if err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ra)
}

func (h *RBACHandler) GetRoleAssignment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ra, err := h.Store.GetRoleAssignment(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ra)
}

func (h *RBACHandler) ListRoleAssignments(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	// If principal filtering is requested
	principalType := r.URL.Query().Get("principal_type")
	principalID := r.URL.Query().Get("principal_id")

	if principalType != "" && principalID != "" {
		if !domain.ValidPrincipalType(domain.PrincipalType(principalType)) {
			http.Error(w, "invalid principal_type: must be user, group, or service_account", http.StatusBadRequest)
			return
		}
		assignments, err := h.Store.ListRoleAssignmentsByPrincipal(r.Context(), tenantID, domain.PrincipalType(principalType), principalID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if assignments == nil {
			assignments = []*store.RoleAssignmentRecord{}
		}
		pagedAssignments, total := paginateSliceWindow(assignments, limit, offset)
		writePaginatedList(w, limit, offset, len(pagedAssignments), int64(total), pagedAssignments)
		return
	}

	assignments, err := h.Store.ListRoleAssignments(r.Context(), tenantID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if assignments == nil {
		assignments = []*store.RoleAssignmentRecord{}
	}
	total := estimatePaginatedTotal(limit, offset, len(assignments))
	writePaginatedList(w, limit, offset, len(assignments), total, assignments)
}

func (h *RBACHandler) DeleteRoleAssignment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteRoleAssignment(r.Context(), id); err != nil {
		http.Error(w, err.Error(), rbacHTTPStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": id})
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func rbacHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "required"), strings.Contains(msg, "invalid"):
		return http.StatusBadRequest
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound
	case strings.Contains(msg, "already exists"), strings.Contains(msg, "duplicate key"):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
