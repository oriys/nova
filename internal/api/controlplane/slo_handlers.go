package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

const (
	defaultSLOWindowSeconds = 900
	defaultSLOMinSamples    = 20
)

func validateSLOPolicy(policy *domain.SLOPolicy) error {
	if policy == nil {
		return fmt.Errorf("slo policy is required")
	}
	if policy.WindowS < 0 {
		return fmt.Errorf("window_s must be >= 0")
	}
	if policy.MinSamples < 0 {
		return fmt.Errorf("min_samples must be >= 0")
	}
	if policy.Objectives.SuccessRatePct < 0 || policy.Objectives.SuccessRatePct > 100 {
		return fmt.Errorf("objectives.success_rate_pct must be between 0 and 100")
	}
	if policy.Objectives.P95DurationMs < 0 {
		return fmt.Errorf("objectives.p95_duration_ms must be >= 0")
	}
	if policy.Objectives.ColdStartRatePct < 0 || policy.Objectives.ColdStartRatePct > 100 {
		return fmt.Errorf("objectives.cold_start_rate_pct must be between 0 and 100")
	}
	return nil
}

func normalizeSLOPolicy(policy *domain.SLOPolicy) *domain.SLOPolicy {
	if policy == nil {
		return nil
	}
	cp := *policy
	if cp.WindowS == 0 {
		cp.WindowS = defaultSLOWindowSeconds
	}
	if cp.MinSamples == 0 {
		cp.MinSamples = defaultSLOMinSamples
	}
	if cp.Notifications == nil {
		cp.Notifications = []domain.SLONotificationTarget{}
	}
	return &cp
}

// GetSLOPolicy handles GET /functions/{name}/slo
func (h *Handler) GetSLOPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	policy := normalizeSLOPolicy(fn.SLOPolicy)
	if policy == nil {
		policy = &domain.SLOPolicy{
			Enabled:       false,
			WindowS:       defaultSLOWindowSeconds,
			MinSamples:    defaultSLOMinSamples,
			Notifications: []domain.SLONotificationTarget{},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policy)
}

// SetSLOPolicy handles PUT /functions/{name}/slo
func (h *Handler) SetSLOPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	req := &domain.SLOPolicy{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := validateSLOPolicy(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	policy := normalizeSLOPolicy(req)
	updatedFn, err := h.Store.UpdateFunction(r.Context(), name, &store.FunctionUpdate{
		SLOPolicy: policy,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if updatedFn.SLOPolicy == nil {
		updatedFn.SLOPolicy = policy
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedFn.SLOPolicy)
}

// DeleteSLOPolicy handles DELETE /functions/{name}/slo
func (h *Handler) DeleteSLOPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	disabled := &domain.SLOPolicy{
		Enabled:       false,
		WindowS:       defaultSLOWindowSeconds,
		MinSamples:    defaultSLOMinSamples,
		Notifications: []domain.SLONotificationTarget{},
	}
	_, err := h.Store.UpdateFunction(r.Context(), name, &store.FunctionUpdate{
		SLOPolicy: disabled,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "deleted",
		"function": name,
	})
}
