package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/store"
)

// AIHandler handles AI-powered code operations.
type AIHandler struct {
	Service *ai.Service
	Store   *store.Store
}

// RegisterRoutes registers AI routes on the mux.
func (h *AIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /ai/generate", h.Generate)
	mux.HandleFunc("POST /ai/review", h.Review)
	mux.HandleFunc("POST /ai/rewrite", h.Rewrite)
	mux.HandleFunc("POST /ai/analyze-diagnostics", h.AnalyzeDiagnostics)
	mux.HandleFunc("GET /ai/prompts", h.ListPrompts)
	mux.HandleFunc("GET /ai/prompts/{name}", h.GetPrompt)
	mux.HandleFunc("PUT /ai/prompts/{name}", h.UpdatePrompt)
	mux.HandleFunc("GET /ai/status", h.Status)
	mux.HandleFunc("GET /ai/config", h.GetConfig)
	mux.HandleFunc("PUT /ai/config", h.UpdateConfig)
	mux.HandleFunc("GET /ai/models", h.ListModels)
}

// Status returns the AI service status.
func (h *AIHandler) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": h.Service.Enabled(),
	})
}

// GetConfig returns the current AI configuration (with masked API key).
func (h *AIHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.Service.GetConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// UpdateConfig updates the AI configuration and persists it to the store.
func (h *AIHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled   *bool   `json:"enabled,omitempty"`
		APIKey    *string `json:"api_key,omitempty"`
		Model     *string `json:"model,omitempty"`
		BaseURL   *string `json:"base_url,omitempty"`
		PromptDir *string `json:"prompt_dir,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Load current full config from store (includes unmasked key)
	cfg := h.loadAIConfigFromStore(r)

	// Apply partial updates
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.APIKey != nil {
		cfg.APIKey = *req.APIKey
	}
	if req.Model != nil && *req.Model != "" {
		cfg.Model = *req.Model
	}
	if req.BaseURL != nil && *req.BaseURL != "" {
		cfg.BaseURL = *req.BaseURL
	}
	if req.PromptDir != nil {
		cfg.PromptDir = *req.PromptDir
	}

	// Persist to store
	if h.Store != nil {
		ctx := r.Context()
		if req.Enabled != nil {
			val := "false"
			if cfg.Enabled {
				val = "true"
			}
			_ = h.Store.SetConfig(ctx, "ai_enabled", val)
		}
		if req.APIKey != nil {
			_ = h.Store.SetConfig(ctx, "ai_api_key", cfg.APIKey)
		}
		if req.Model != nil {
			_ = h.Store.SetConfig(ctx, "ai_model", cfg.Model)
		}
		if req.BaseURL != nil {
			_ = h.Store.SetConfig(ctx, "ai_base_url", cfg.BaseURL)
		}
		if req.PromptDir != nil {
			_ = h.Store.SetConfig(ctx, "ai_prompt_dir", cfg.PromptDir)
		}
	}

	// Apply to running service
	h.Service.UpdateConfig(cfg)

	// Return masked config
	masked := h.Service.GetConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(masked)
}

// loadAIConfigFromStore reads the full AI config from the database store.
func (h *AIHandler) loadAIConfigFromStore(r *http.Request) ai.Config {
	cfg := ai.DefaultConfig()
	if h.Store == nil {
		return cfg
	}
	all, err := h.Store.GetConfig(r.Context())
	if err != nil {
		return cfg
	}
	if v, ok := all["ai_enabled"]; ok {
		cfg.Enabled = v == "true" || v == "1"
	}
	if v, ok := all["ai_api_key"]; ok && v != "" {
		cfg.APIKey = v
	}
	if v, ok := all["ai_model"]; ok && v != "" {
		cfg.Model = v
	}
	if v, ok := all["ai_base_url"]; ok && v != "" {
		cfg.BaseURL = v
	}
	if v, ok := all["ai_prompt_dir"]; ok {
		cfg.PromptDir = v
	}
	return cfg
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

// ListModels fetches available models from the configured AI provider.
func (h *AIHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.Service.ListModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

// AnalyzeDiagnostics analyzes function performance diagnostics.
func (h *AIHandler) AnalyzeDiagnostics(w http.ResponseWriter, r *http.Request) {
	var req ai.DiagnosticsAnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.FunctionName == "" {
		http.Error(w, "function_name is required", http.StatusBadRequest)
		return
	}

	resp, err := h.Service.AnalyzeDiagnostics(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListPrompts returns supported AI prompt templates.
func (h *AIHandler) ListPrompts(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListPromptTemplates()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
	})
}

// GetPrompt returns one AI prompt template by name.
func (h *AIHandler) GetPrompt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	item, err := h.Service.GetPromptTemplate(name)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ai.ErrPromptTemplateNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// UpdatePrompt updates a prompt template override and reloads active prompts.
func (h *AIHandler) UpdatePrompt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	item, err := h.Service.UpdatePromptTemplate(name, req.Content)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ai.ErrPromptTemplateNotFound):
			status = http.StatusNotFound
		case errors.Is(err, ai.ErrInvalidPromptTemplate):
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}
