package controlplane

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/store"
)

// TestSuiteHandler handles test suite CRUD and AI generation.
type TestSuiteHandler struct {
	AIService *ai.Service
	Store     *store.Store
}

// RegisterRoutes registers test suite routes on the mux.
func (h *TestSuiteHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /functions/{name}/test-suite", h.GetTestSuite)
	mux.HandleFunc("PUT /functions/{name}/test-suite", h.SaveTestSuite)
	mux.HandleFunc("DELETE /functions/{name}/test-suite", h.DeleteTestSuite)
	mux.HandleFunc("POST /ai/generate-tests", h.GenerateTests)
}

// GetTestSuite returns the persisted test suite for a function.
func (h *TestSuiteHandler) GetTestSuite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	ts, err := h.Store.GetTestSuite(r.Context(), name)
	if err != nil {
		http.Error(w, "test suite not found for function: "+name, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ts)
}

// SaveTestSuite saves or updates the test suite for a function.
func (h *TestSuiteHandler) SaveTestSuite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	var req struct {
		TestCases json.RawMessage `json:"test_cases"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.TestCases == nil {
		http.Error(w, "test_cases is required", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// Preserve created_at if existing
	existing, _ := h.Store.GetTestSuite(r.Context(), name)
	createdAt := now
	if existing != nil {
		createdAt = existing.CreatedAt
	}

	ts := &store.TestSuite{
		FunctionName: name,
		TestCases:    req.TestCases,
		UpdatedAt:    now,
		CreatedAt:    createdAt,
	}

	if err := h.Store.SaveTestSuite(r.Context(), ts); err != nil {
		http.Error(w, "failed to save test suite", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ts)
}

// DeleteTestSuite removes the test suite for a function.
func (h *TestSuiteHandler) DeleteTestSuite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteTestSuite(r.Context(), name); err != nil {
		http.Error(w, "failed to delete test suite", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "function_name": name})
}

// GenerateTests generates a test suite using AI.
func (h *TestSuiteHandler) GenerateTests(w http.ResponseWriter, r *http.Request) {
	if h.AIService == nil {
		http.Error(w, "AI service is not configured", http.StatusServiceUnavailable)
		return
	}

	var req ai.GenerateTestsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.FunctionName == "" {
		http.Error(w, "function_name is required", http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	resp, err := h.AIService.GenerateTests(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
