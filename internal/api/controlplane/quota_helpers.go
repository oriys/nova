package controlplane

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/oriys/nova/internal/store"
)

func writeTenantQuotaExceededResponse(w http.ResponseWriter, decision *store.TenantQuotaDecision) {
	if decision == nil {
		http.Error(w, "tenant quota exceeded", http.StatusTooManyRequests)
		return
	}
	if decision.RetryAfterS > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(decision.RetryAfterS))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":         "tenant quota exceeded",
		"tenant_id":     decision.TenantID,
		"dimension":     decision.Dimension,
		"used":          decision.Used,
		"limit":         decision.Limit,
		"window_s":      decision.WindowS,
		"retry_after_s": decision.RetryAfterS,
	})
}

