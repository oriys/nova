package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// SetScalingPolicy sets the auto-scaling policy for a function.
func (h *Handler) SetScalingPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	var policy domain.AutoScalePolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if policy.MaxReplicas < policy.MinReplicas {
		http.Error(w, "max_replicas must be >= min_replicas", http.StatusBadRequest)
		return
	}
	if policy.TargetUtilization < 0 || policy.TargetUtilization > 1 {
		http.Error(w, "target_utilization must be between 0 and 1", http.StatusBadRequest)
		return
	}
	if policy.CooldownScaleUpS < 0 || policy.CooldownScaleDownS < 0 {
		http.Error(w, "cooldown values must be >= 0", http.StatusBadRequest)
		return
	}
	if policy.ScaleDownStep < 0 || policy.ScaleUpStepMax < 0 || policy.ScaleDownStabilizationS < 0 || policy.MinSampleCount < 0 {
		http.Error(w, "step/stabilization/sample values must be >= 0", http.StatusBadRequest)
		return
	}
	if policy.TargetUtilization == 0 {
		policy.TargetUtilization = 0.7
	}

	updated, err := h.Store.UpdateFunction(r.Context(), fn.Name, &store.FunctionUpdate{
		AutoScalePolicy: &policy,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated.AutoScalePolicy)
}

// GetScalingPolicy returns the auto-scaling policy for a function.
func (h *Handler) GetScalingPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if fn.AutoScalePolicy == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
		})
		return
	}

	json.NewEncoder(w).Encode(fn.AutoScalePolicy)
}

// DeleteScalingPolicy removes the auto-scaling policy from a function.
func (h *Handler) DeleteScalingPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	disabledPolicy := &domain.AutoScalePolicy{Enabled: false}
	_, err = h.Store.UpdateFunction(r.Context(), fn.Name, &store.FunctionUpdate{
		AutoScalePolicy: disabledPolicy,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "function": name})
}
