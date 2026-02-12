package controlplane

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/store"
)

// ListTenants handles GET /tenants
func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	identity := auth.GetIdentity(r.Context())
	tenantIDs, unrestricted := visibleTenantIDs(identity)

	var (
		tenants []*store.TenantRecord
		err     error
	)
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	if unrestricted {
		tenants, err = h.Store.ListTenants(r.Context(), limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		tenants = make([]*store.TenantRecord, 0, len(tenantIDs))
		for _, tenantID := range tenantIDs {
			tenant, getErr := h.Store.GetTenant(r.Context(), tenantID)
			if getErr != nil {
				if strings.Contains(strings.ToLower(getErr.Error()), "not found") {
					continue
				}
				http.Error(w, getErr.Error(), tenancyHTTPStatus(getErr))
				return
			}
			tenants = append(tenants, tenant)
		}
	}

	if tenants == nil {
		tenants = []*store.TenantRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenants)
}

// CreateTenant handles POST /tenants
func (h *Handler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Tier   string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ID) != "" && !enforceTenantAccess(w, r, req.ID) {
		return
	}

	tenant, err := h.Store.CreateTenant(r.Context(), &store.TenantRecord{
		ID:     req.ID,
		Name:   req.Name,
		Status: req.Status,
		Tier:   req.Tier,
	})
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

// UpdateTenant handles PATCH /tenants/{tenantID}
func (h *Handler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	var update store.TenantUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	tenant, err := h.Store.UpdateTenant(r.Context(), tenantID, &update)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}

// DeleteTenant handles DELETE /tenants/{tenantID}
func (h *Handler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	if err := h.Store.DeleteTenant(r.Context(), tenantID); err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
		"id":     tenantID,
	})
}

// ListNamespaces handles GET /tenants/{tenantID}/namespaces
func (h *Handler) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	namespaces, err := h.Store.ListNamespaces(r.Context(), tenantID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}
	namespaces = filterVisibleNamespaces(auth.GetIdentity(r.Context()), tenantID, namespaces)
	if namespaces == nil {
		namespaces = []*store.NamespaceRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(namespaces)
}

// CreateNamespace handles POST /tenants/{tenantID}/namespaces
func (h *Handler) CreateNamespace(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) != "" && !enforceNamespaceAccess(w, r, tenantID, req.Name) {
		return
	}

	namespace, err := h.Store.CreateNamespace(r.Context(), &store.NamespaceRecord{
		TenantID: tenantID,
		Name:     req.Name,
	})
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(namespace)
}

// UpdateNamespace handles PATCH /tenants/{tenantID}/namespaces/{namespace}
func (h *Handler) UpdateNamespace(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	namespaceName := r.PathValue("namespace")
	if !enforceNamespaceAccess(w, r, tenantID, namespaceName) {
		return
	}

	var update store.NamespaceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if update.Name != nil && !enforceNamespaceAccess(w, r, tenantID, strings.TrimSpace(*update.Name)) {
		return
	}

	namespace, err := h.Store.UpdateNamespace(r.Context(), tenantID, namespaceName, &update)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(namespace)
}

// DeleteNamespace handles DELETE /tenants/{tenantID}/namespaces/{namespace}
func (h *Handler) DeleteNamespace(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	namespaceName := r.PathValue("namespace")
	if !enforceNamespaceAccess(w, r, tenantID, namespaceName) {
		return
	}

	if err := h.Store.DeleteNamespace(r.Context(), tenantID, namespaceName); err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "deleted",
		"tenant_id": tenantID,
		"name":      namespaceName,
	})
}

// ListTenantQuotas handles GET /tenants/{tenantID}/quotas
func (h *Handler) ListTenantQuotas(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	quotas, err := h.Store.ListTenantQuotas(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}
	if quotas == nil {
		quotas = []*store.TenantQuotaRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(quotas)
}

// UpsertTenantQuota handles PUT /tenants/{tenantID}/quotas/{dimension}
func (h *Handler) UpsertTenantQuota(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	dimension := r.PathValue("dimension")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	var req struct {
		HardLimit int64 `json:"hard_limit"`
		SoftLimit int64 `json:"soft_limit"`
		Burst     int64 `json:"burst"`
		WindowS   int   `json:"window_s"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	quota, err := h.Store.UpsertTenantQuota(r.Context(), &store.TenantQuotaRecord{
		TenantID:  tenantID,
		Dimension: dimension,
		HardLimit: req.HardLimit,
		SoftLimit: req.SoftLimit,
		Burst:     req.Burst,
		WindowS:   req.WindowS,
	})
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(quota)
}

// DeleteTenantQuota handles DELETE /tenants/{tenantID}/quotas/{dimension}
func (h *Handler) DeleteTenantQuota(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	dimension := r.PathValue("dimension")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	if err := h.Store.DeleteTenantQuota(r.Context(), tenantID, dimension); err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "deleted",
		"tenant_id": tenantID,
		"dimension": dimension,
	})
}

// GetTenantUsage handles GET /tenants/{tenantID}/usage
func (h *Handler) GetTenantUsage(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}
	refresh := true
	if raw := strings.TrimSpace(r.URL.Query().Get("refresh")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			refresh = parsed
		}
	}

	var (
		usage []*store.TenantUsageRecord
		err   error
	)
	if refresh {
		usage, err = h.Store.RefreshTenantUsage(r.Context(), tenantID)
	} else {
		usage, err = h.Store.ListTenantUsage(r.Context(), tenantID)
	}
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}
	if usage == nil {
		usage = []*store.TenantUsageRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(usage)
}

// ─── Tenant Menu Permissions ────────────────────────────────────────────────

// ListTenantMenuPermissions handles GET /tenants/{tenantID}/menu-permissions
func (h *Handler) ListTenantMenuPermissions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	perms, err := h.Store.ListTenantMenuPermissions(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}
	if perms == nil {
		perms = []*store.MenuPermissionRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(perms)
}

// UpsertTenantMenuPermission handles PUT /tenants/{tenantID}/menu-permissions/{menuKey}
func (h *Handler) UpsertTenantMenuPermission(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	menuKey := r.PathValue("menuKey")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	perm, err := h.Store.UpsertTenantMenuPermission(r.Context(), tenantID, menuKey, req.Enabled)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(perm)
}

// DeleteTenantMenuPermission handles DELETE /tenants/{tenantID}/menu-permissions/{menuKey}
func (h *Handler) DeleteTenantMenuPermission(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	menuKey := r.PathValue("menuKey")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	if err := h.Store.DeleteTenantMenuPermission(r.Context(), tenantID, menuKey); err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "deleted",
		"tenant_id": tenantID,
		"menu_key":  menuKey,
	})
}

// ─── Tenant Button Permissions ──────────────────────────────────────────────

// ListTenantButtonPermissions handles GET /tenants/{tenantID}/button-permissions
func (h *Handler) ListTenantButtonPermissions(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	perms, err := h.Store.ListTenantButtonPermissions(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}
	if perms == nil {
		perms = []*store.ButtonPermissionRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(perms)
}

// UpsertTenantButtonPermission handles PUT /tenants/{tenantID}/button-permissions/{permissionKey}
func (h *Handler) UpsertTenantButtonPermission(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	permissionKey := r.PathValue("permissionKey")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	perm, err := h.Store.UpsertTenantButtonPermission(r.Context(), tenantID, permissionKey, req.Enabled)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(perm)
}

// DeleteTenantButtonPermission handles DELETE /tenants/{tenantID}/button-permissions/{permissionKey}
func (h *Handler) DeleteTenantButtonPermission(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	permissionKey := r.PathValue("permissionKey")
	if !enforceTenantAccess(w, r, tenantID) {
		return
	}

	if err := h.Store.DeleteTenantButtonPermission(r.Context(), tenantID, permissionKey); err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":         "deleted",
		"tenant_id":      tenantID,
		"permission_key": permissionKey,
	})
}

func tenancyHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "required"),
		strings.Contains(msg, "must match"),
		strings.Contains(msg, "invalid"),
		strings.Contains(msg, "unsupported"):
		return http.StatusBadRequest
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound
	case strings.Contains(msg, "already exists"),
		strings.Contains(msg, "duplicate key"),
		strings.Contains(msg, "cannot delete"),
		strings.Contains(msg, "cannot rename"),
		strings.Contains(msg, "last namespace"),
		strings.Contains(msg, "still has managed resources"):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
