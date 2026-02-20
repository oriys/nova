package dataplane

import (
	"encoding/json"
	"net/http"
)

type prewarmRequest struct {
	TargetReplicas int `json:"target_replicas"`
}

// PrewarmFunction handles POST /functions/{name}/prewarm.
func (h *Handler) PrewarmFunction(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		http.Error(w, "pool is not configured", http.StatusServiceUnavailable)
		return
	}

	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	req := prewarmRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	target := req.TargetReplicas
	if target < fn.MinReplicas {
		target = fn.MinReplicas
	}
	if target < 1 {
		target = 1
	}
	if fn.MaxReplicas > 0 && target > fn.MaxReplicas {
		target = fn.MaxReplicas
	}

	codeRecord, err := h.Store.GetFunctionCode(r.Context(), fn.ID)
	if err != nil {
		http.Error(w, "load function code: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if codeRecord == nil {
		http.Error(w, "function code not found", http.StatusNotFound)
		return
	}

	codeContent := codeRecord.CompiledBinary
	if len(codeContent) == 0 {
		codeContent = []byte(codeRecord.SourceCode)
	}

	h.Pool.SetDesiredReplicas(fn.ID, target)
	if err := h.Pool.EnsureReady(r.Context(), fn, codeContent); err != nil {
		http.Error(w, "prewarm failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "ok",
		"function":        fn.Name,
		"target_replicas": target,
	})
}
