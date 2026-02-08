package controlplane

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/oriys/nova/internal/store"
)

// ListTenants handles GET /tenants
func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.Store.ListTenants(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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

	namespaces, err := h.Store.ListNamespaces(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), tenancyHTTPStatus(err))
		return
	}
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

	var update store.NamespaceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
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
