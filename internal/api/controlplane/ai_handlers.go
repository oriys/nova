package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/ai"
)

// AIHandler handles AI-powered code operations.
type AIHandler struct {
	Service *ai.Service
}

// RegisterRoutes registers AI routes on the mux.
func (h *AIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /ai/generate", h.Generate)
	mux.HandleFunc("POST /ai/review", h.Review)
	mux.HandleFunc("POST /ai/rewrite", h.Rewrite)
	mux.HandleFunc("GET /ai/status", h.Status)
}

// Status returns the AI service status.
func (h *AIHandler) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": h.Service.Enabled(),
	})
}

// Generate creates function code from a natural language description.
func (h *AIHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req ai.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Description == "" {
		http.Error(w, "description is required", http.StatusBadRequest)
		return
	}
	if req.Runtime == "" {
		http.Error(w, "runtime is required", http.StatusBadRequest)
		return
	}

	resp, err := h.Service.Generate(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Review analyzes function code and provides feedback.
func (h *AIHandler) Review(w http.ResponseWriter, r *http.Request) {
	var req ai.ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}
	if req.Runtime == "" {
		http.Error(w, "runtime is required", http.StatusBadRequest)
		return
	}

	resp, err := h.Service.Review(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Rewrite improves or rewrites function code.
func (h *AIHandler) Rewrite(w http.ResponseWriter, r *http.Request) {
	var req ai.RewriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}
	if req.Runtime == "" {
		http.Error(w, "runtime is required", http.StatusBadRequest)
		return
	}

	resp, err := h.Service.Rewrite(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
