package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// SetCapacityPolicy sets the capacity/admission control policy for a function.
func (h *Handler) SetCapacityPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	var policy domain.CapacityPolicy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateCapacityPolicy(&policy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if policy.ShedStatusCode == 0 {
		policy.ShedStatusCode = http.StatusServiceUnavailable
	}

	updated, err := h.Store.UpdateFunction(r.Context(), fn.Name, &store.FunctionUpdate{
		CapacityPolicy: &policy,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated.CapacityPolicy)
}

// GetCapacityPolicy returns the capacity policy for a function.
func (h *Handler) GetCapacityPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if fn.CapacityPolicy == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
		})
		return
	}

	json.NewEncoder(w).Encode(fn.CapacityPolicy)
}

// DeleteCapacityPolicy disables the capacity policy for a function.
func (h *Handler) DeleteCapacityPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	disabled := &domain.CapacityPolicy{Enabled: false}
	_, err = h.Store.UpdateFunction(r.Context(), fn.Name, &store.FunctionUpdate{
		CapacityPolicy: disabled,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "function": name})
}

func validateCapacityPolicy(policy *domain.CapacityPolicy) error {
	if policy.MaxInflight < 0 {
		return errors.New("max_inflight must be >= 0")
	}
	if policy.MaxQueueDepth < 0 {
		return errors.New("max_queue_depth must be >= 0")
	}
	if policy.MaxQueueWaitMs < 0 {
		return errors.New("max_queue_wait_ms must be >= 0")
	}
	if policy.RetryAfterS < 0 {
		return errors.New("retry_after_s must be >= 0")
	}
	if policy.ShedStatusCode != 0 &&
		policy.ShedStatusCode != http.StatusTooManyRequests &&
		policy.ShedStatusCode != http.StatusServiceUnavailable {
		return errors.New("shed_status_code must be 429 or 503")
	}
	if policy.BreakerErrorPct < 0 || policy.BreakerErrorPct > 100 {
		return errors.New("breaker_error_pct must be between 0 and 100")
	}
	if policy.BreakerWindowS < 0 || policy.BreakerOpenS < 0 || policy.HalfOpenProbes < 0 {
		return errors.New("breaker_window_s, breaker_open_s, half_open_probes must be >= 0")
	}
	return nil
}
