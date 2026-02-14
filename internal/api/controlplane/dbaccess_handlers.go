package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// ── DbResource handlers ────────────────────────────────────────────────────

// CreateDbResource handles POST /db-resources
func (h *Handler) CreateDbResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string                  `json:"name"`
		Type          domain.DbResourceType   `json:"type"`
		Endpoint      string                  `json:"endpoint"`
		Port          int                     `json:"port,omitempty"`
		DatabaseName  string                  `json:"database_name,omitempty"`
		Region        string                  `json:"region,omitempty"`
		TenantMode    domain.TenantMode       `json:"tenant_mode"`
		NetworkPolicy string                  `json:"network_policy,omitempty"`
		Capabilities  *domain.DbCapabilities  `json:"capabilities,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if !domain.IsValidDbResourceType(req.Type) {
		http.Error(w, "invalid type: must be postgres, mysql, redis, dynamo, or http", http.StatusBadRequest)
		return
	}
	if req.Endpoint == "" {
		http.Error(w, "endpoint is required", http.StatusBadRequest)
		return
	}
	if req.TenantMode == "" {
		req.TenantMode = domain.TenantModeSharedRLS
	}
	if !domain.IsValidTenantMode(req.TenantMode) {
		http.Error(w, "invalid tenant_mode: must be db_per_tenant, schema_per_tenant, or shared_rls", http.StatusBadRequest)
		return
	}

	rec := &store.DbResourceRecord{
		Name:          req.Name,
		Type:          req.Type,
		Endpoint:      req.Endpoint,
		Port:          req.Port,
		DatabaseName:  req.DatabaseName,
		Region:        req.Region,
		TenantMode:    req.TenantMode,
		NetworkPolicy: req.NetworkPolicy,
		Capabilities:  req.Capabilities,
	}

	created, err := h.Store.CreateDbResource(r.Context(), rec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// ListDbResources handles GET /db-resources
func (h *Handler) ListDbResources(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	resources, err := h.Store.ListDbResources(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resources == nil {
		resources = []*store.DbResourceRecord{}
	}
	total := estimatePaginatedTotal(limit, offset, len(resources))
	writePaginatedList(w, limit, offset, len(resources), total, resources)
}

// GetDbResource handles GET /db-resources/{name}
func (h *Handler) GetDbResource(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// UpdateDbResource handles PATCH /db-resources/{name}
func (h *Handler) UpdateDbResource(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}

	var update store.DbResourceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if update.TenantMode != nil && !domain.IsValidTenantMode(*update.TenantMode) {
		http.Error(w, "invalid tenant_mode", http.StatusBadRequest)
		return
	}

	updated, err := h.Store.UpdateDbResource(r.Context(), res.ID, &update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// DeleteDbResource handles DELETE /db-resources/{name}
func (h *Handler) DeleteDbResource(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}
	if err := h.Store.DeleteDbResource(r.Context(), res.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── DbBinding handlers ─────────────────────────────────────────────────────

// CreateDbBinding handles POST /db-resources/{name}/bindings
func (h *Handler) CreateDbBinding(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		FunctionID      string                  `json:"function_id"`
		VersionSelector string                  `json:"version_selector,omitempty"`
		Permissions     []domain.DbPermission   `json:"permissions"`
		Quota           *domain.DbBindingQuota  `json:"quota,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.FunctionID == "" {
		http.Error(w, "function_id is required", http.StatusBadRequest)
		return
	}
	if len(req.Permissions) == 0 {
		req.Permissions = []domain.DbPermission{domain.DbPermRead}
	}
	for _, p := range req.Permissions {
		if !domain.IsValidDbPermission(p) {
			http.Error(w, "invalid permission: "+string(p), http.StatusBadRequest)
			return
		}
	}
	if req.VersionSelector == "" {
		req.VersionSelector = "*"
	}

	rec := &store.DbBindingRecord{
		FunctionID:      req.FunctionID,
		VersionSelector: req.VersionSelector,
		DbResourceID:    res.ID,
		Permissions:     req.Permissions,
		Quota:           req.Quota,
	}

	created, err := h.Store.CreateDbBinding(r.Context(), rec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// ListDbBindings handles GET /db-resources/{name}/bindings
func (h *Handler) ListDbBindings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}

	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	bindings, err := h.Store.ListDbBindings(r.Context(), res.ID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if bindings == nil {
		bindings = []*store.DbBindingRecord{}
	}
	total := estimatePaginatedTotal(limit, offset, len(bindings))
	writePaginatedList(w, limit, offset, len(bindings), total, bindings)
}

// DeleteDbBinding handles DELETE /db-bindings/{id}
func (h *Handler) DeleteDbBinding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteDbBinding(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── CredentialPolicy handlers ───────────────────────────────────────────────

// SetCredentialPolicy handles PUT /db-resources/{name}/credential-policy
func (h *Handler) SetCredentialPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		AuthMode       domain.CredentialAuthMode `json:"auth_mode"`
		RotationDays   int                       `json:"rotation_days,omitempty"`
		StaticUsername string                     `json:"static_username,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !domain.IsValidCredentialAuthMode(req.AuthMode) {
		http.Error(w, "invalid auth_mode: must be static, iam, or token_exchange", http.StatusBadRequest)
		return
	}

	// Try to update existing; if not found, create new.
	existing, getErr := h.Store.GetCredentialPolicy(r.Context(), res.ID)
	if getErr == nil && existing != nil {
		update := &store.CredentialPolicyUpdate{
			AuthMode:       &req.AuthMode,
			RotationDays:   &req.RotationDays,
			StaticUsername: &req.StaticUsername,
		}
		updated, updateErr := h.Store.UpdateCredentialPolicy(r.Context(), res.ID, update)
		if updateErr != nil {
			http.Error(w, updateErr.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
		return
	}

	rec := &store.CredentialPolicyRecord{
		DbResourceID:   res.ID,
		AuthMode:       req.AuthMode,
		RotationDays:   req.RotationDays,
		StaticUsername: req.StaticUsername,
	}
	created, createErr := h.Store.CreateCredentialPolicy(r.Context(), rec)
	if createErr != nil {
		http.Error(w, createErr.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// GetCredentialPolicy handles GET /db-resources/{name}/credential-policy
func (h *Handler) GetCredentialPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}

	policy, err := h.Store.GetCredentialPolicy(r.Context(), res.ID)
	if err != nil {
		http.Error(w, "credential policy not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policy)
}

// DeleteCredentialPolicy handles DELETE /db-resources/{name}/credential-policy
func (h *Handler) DeleteCredentialPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}
	if err := h.Store.DeleteCredentialPolicy(r.Context(), res.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── DbRequestLog handlers ──────────────────────────────────────────────────

// ListDbRequestLogs handles GET /db-resources/{name}/logs
func (h *Handler) ListDbRequestLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, err := h.Store.GetDbResourceByName(r.Context(), name)
	if err != nil {
		http.Error(w, "db resource not found: "+name, http.StatusNotFound)
		return
	}

	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	logs, err := h.Store.ListDbRequestLogs(r.Context(), res.ID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []*domain.DbRequestLog{}
	}
	total := estimatePaginatedTotal(limit, offset, len(logs))
	writePaginatedList(w, limit, offset, len(logs), total, logs)
}
