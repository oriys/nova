package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/backend"
)

// ListBackends returns available execution backends detected on the current system.
func (h *Handler) ListBackends(w http.ResponseWriter, r *http.Request) {
	backends := backend.DetectAvailableBackends()
	defaultBackend := backend.DetectDefaultBackend()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"backends":        backends,
		"default_backend": defaultBackend,
	})
}
