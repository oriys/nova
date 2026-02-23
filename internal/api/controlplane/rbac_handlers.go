package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/authz"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// RBACHandler handles RBAC management endpoints.
type RBACHandler struct {
	Store *store.Store
}

func (h *RBACHandler) RegisterRoutes(mux *http.ServeMux) {
	// Permission bindings & effective permissions (read-only, no rbac:manage required)
	mux.HandleFunc("GET /rbac/permission-bindings", h.ListPermissionBindings)
	mux.HandleFunc("GET /rbac/my-permissions", h.GetMyPermissions)

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
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "id and name are required", http.StatusBadRequest)
		return
	}

	scope := store.TenantScopeFromContext(r.Context())
	role, err := h.Store.CreateRole(r.Context(), &store.RoleRecord{
		ID:       req.ID,
		TenantID: scope.TenantID,
		Name:     req.Name,
		IsSystem: false,
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
	scope := store.TenantScopeFromContext(r.Context())
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	roles, err := h.Store.ListRoles(r.Context(), scope.TenantID, limit, offset)
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

	// Validate role exists
	if _, err := h.Store.GetRole(r.Context(), roleID); err != nil {
		http.Error(w, "role not found: "+roleID, http.StatusNotFound)
		return
	}
	// Validate permission exists
	if _, err := h.Store.GetPermission(r.Context(), req.PermissionID); err != nil {
		http.Error(w, "permission not found: "+req.PermissionID, http.StatusNotFound)
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
		PrincipalType string `json:"principal_type"`
		PrincipalID   string `json:"principal_id"`
		RoleID        string `json:"role_id"`
		ScopeType     string `json:"scope_type"`
		ScopeID       string `json:"scope_id"`
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

	// Validate role exists
	if _, err := h.Store.GetRole(r.Context(), req.RoleID); err != nil {
		http.Error(w, "role not found: "+req.RoleID, http.StatusNotFound)
		return
	}

	scope := store.TenantScopeFromContext(r.Context())

	// Derive created_by from authenticated identity
	createdBy := ""
	if identity := auth.GetIdentity(r.Context()); identity != nil {
		createdBy = identity.Subject
	}

	ra, err := h.Store.CreateRoleAssignment(r.Context(), &store.RoleAssignmentRecord{
		ID:            req.ID,
		TenantID:      scope.TenantID,
		PrincipalType: domain.PrincipalType(req.PrincipalType),
		PrincipalID:   req.PrincipalID,
		RoleID:        req.RoleID,
		ScopeType:     domain.ScopeType(req.ScopeType),
		ScopeID:       req.ScopeID,
		CreatedBy:     createdBy,
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
	scope := store.TenantScopeFromContext(r.Context())
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
		assignments, err := h.Store.ListRoleAssignmentsByPrincipal(r.Context(), scope.TenantID, domain.PrincipalType(principalType), principalID, limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if assignments == nil {
			assignments = []*store.RoleAssignmentRecord{}
		}
		total := estimatePaginatedTotal(limit, offset, len(assignments))
		writePaginatedList(w, limit, offset, len(assignments), total, assignments)
		return
	}

	assignments, err := h.Store.ListRoleAssignments(r.Context(), scope.TenantID, limit, offset)
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

// ─── Permission Bindings & Effective Permissions ────────────────────────────

// ListPermissionBindings returns the full permission → API routes → UI buttons
// mapping.  This is a read-only, public-within-auth endpoint.
func (h *RBACHandler) ListPermissionBindings(w http.ResponseWriter, r *http.Request) {
	bindings := authz.BuildPermissionBindings()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bindings)
}

// EffectivePermissionsResponse is the response for GET /rbac/my-permissions.
type EffectivePermissionsResponse struct {
	Subject           string            `json:"subject"`
	TenantID          string            `json:"tenant_id"`
	Permissions       []string          `json:"permissions"`
	DBRoles           []string          `json:"db_roles"`
	ButtonPermissions map[string]bool   `json:"button_permissions"`
	MenuPermissions   map[string]bool   `json:"menu_permissions"`
}

// GetMyPermissions returns the effective permissions for the current user.
// It combines: identity policies (JWT/API key) + DB RBAC role assignments +
// tenant-level button/menu permissions.
func (h *RBACHandler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
	identity := auth.GetIdentity(r.Context())
	if identity == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	scope := store.TenantScopeFromContext(r.Context())
	tenantID := scope.TenantID
	if tenantID == "" {
		tenantID = store.DefaultTenantID
	}

	resp := EffectivePermissionsResponse{
		Subject:  identity.Subject,
		TenantID: tenantID,
	}

	// 1. Collect permissions from identity policies (hardcoded role map).
	permSet := make(map[string]bool)
	for _, pb := range identity.Policies {
		if pb.Effect == domain.EffectDeny {
			continue
		}
		if pb.Role == domain.RoleAdmin {
			// Admin gets all permissions.
			for _, perms := range domain.RolePermissions {
				for _, p := range perms {
					permSet[string(p)] = true
				}
			}
			break
		}
		if perms, ok := domain.RolePermissions[pb.Role]; ok {
			for _, p := range perms {
				permSet[string(p)] = true
			}
		}
	}

	// 2. Resolve DB RBAC role assignments.
	dbPerms, err := h.Store.ResolveEffectivePermissions(r.Context(), tenantID, identity.Subject)
	if err == nil {
		for _, code := range dbPerms {
			permSet[code] = true
		}
	}

	// Collect DB role IDs for reference.
	dbRoleAssignments, err := h.Store.ListRoleAssignmentsByPrincipal(
		r.Context(), tenantID, domain.PrincipalUser, identity.Subject, 100, 0,
	)
	if err == nil {
		for _, ra := range dbRoleAssignments {
			resp.DBRoles = append(resp.DBRoles, ra.RoleID)
		}
	}
	// Also include tenant-level group assignments.
	groupAssignments, err := h.Store.ListRoleAssignmentsByPrincipal(
		r.Context(), tenantID, domain.PrincipalGroup, tenantID, 100, 0,
	)
	if err == nil {
		for _, ra := range groupAssignments {
			resp.DBRoles = append(resp.DBRoles, ra.RoleID)
		}
	}

	// 3. Build sorted permission list.
	codes := make([]string, 0, len(permSet))
	for code := range permSet {
		codes = append(codes, code)
	}
	sortStrings(codes)
	resp.Permissions = codes

	// 4. Fetch tenant-level button permissions.
	resp.ButtonPermissions = make(map[string]bool)
	if btnPerms, err := h.Store.ListTenantButtonPermissions(r.Context(), tenantID); err == nil {
		for _, bp := range btnPerms {
			resp.ButtonPermissions[bp.PermissionKey] = bp.Enabled && permSet[bp.PermissionKey]
		}
	}

	// 5. Fetch tenant-level menu permissions.
	resp.MenuPermissions = make(map[string]bool)
	if menuPerms, err := h.Store.ListTenantMenuPermissions(r.Context(), tenantID); err == nil {
		for _, mp := range menuPerms {
			resp.MenuPermissions[mp.MenuKey] = mp.Enabled
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
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
