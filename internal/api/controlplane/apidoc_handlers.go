package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/store"
)

// APIDocHandler handles API documentation generation and sharing.
type APIDocHandler struct {
	AIService *ai.Service
	Store     *store.Store
}

// RegisterRoutes registers API documentation routes on the mux.
func (h *APIDocHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /ai/generate-docs", h.GenerateDocs)
	mux.HandleFunc("POST /ai/generate-workflow-docs", h.GenerateWorkflowDocs)
	mux.HandleFunc("POST /api-docs/shares", h.CreateShare)
	mux.HandleFunc("GET /api-docs/shares", h.ListShares)
	mux.HandleFunc("DELETE /api-docs/shares/{id}", h.DeleteShare)
	mux.HandleFunc("GET /api-docs/shared/{token}", h.GetSharedDoc)

	// Per-function persisted docs
	mux.HandleFunc("GET /functions/{name}/docs", h.GetFunctionDoc)
	mux.HandleFunc("PUT /functions/{name}/docs", h.SaveFunctionDoc)
	mux.HandleFunc("DELETE /functions/{name}/docs", h.DeleteFunctionDoc)

	// Per-workflow persisted docs
	mux.HandleFunc("GET /workflows/{name}/docs", h.GetWorkflowDoc)
	mux.HandleFunc("PUT /workflows/{name}/docs", h.SaveWorkflowDoc)
	mux.HandleFunc("DELETE /workflows/{name}/docs", h.DeleteWorkflowDoc)
}

func (h *APIDocHandler) GenerateDocs(w http.ResponseWriter, r *http.Request) {
	var req ai.GenerateDocsRequest
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

	resp, err := h.AIService.GenerateDocs(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIDocHandler) GenerateWorkflowDocs(w http.ResponseWriter, r *http.Request) {
	var req ai.GenerateWorkflowDocsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.WorkflowName == "" {
		http.Error(w, "workflow_name is required", http.StatusBadRequest)
		return
	}
	if req.Nodes == "" {
		http.Error(w, "nodes is required", http.StatusBadRequest)
		return
	}

	resp, err := h.AIService.GenerateWorkflowDocs(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: failed to read random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (h *APIDocHandler) CreateShare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FunctionName string          `json:"function_name"`
		Title        string          `json:"title"`
		DocContent   json.RawMessage `json:"doc_content"`
		ExpiresIn    string          `json:"expires_in,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if req.DocContent == nil {
		http.Error(w, "doc_content is required", http.StatusBadRequest)
		return
	}

	token := generateToken()
	now := time.Now()

	share := &store.APIDocShare{
		ID:           "doc_" + generateToken()[:16],
		TenantID:     r.Header.Get("X-Tenant-ID"),
		Namespace:    r.Header.Get("X-Namespace"),
		FunctionName: req.FunctionName,
		Title:        req.Title,
		Token:        token,
		DocContent:   req.DocContent,
		CreatedBy:    r.Header.Get("X-User"),
		AccessCount:  0,
		CreatedAt:    now,
	}
	if share.TenantID == "" {
		share.TenantID = "default"
	}
	if share.Namespace == "" {
		share.Namespace = "default"
	}

	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			// Try parsing as days (e.g. "7d" -> "168h")
			if len(req.ExpiresIn) > 1 && req.ExpiresIn[len(req.ExpiresIn)-1] == 'd' {
				days, err2 := strconv.Atoi(req.ExpiresIn[:len(req.ExpiresIn)-1])
				if err2 != nil {
					http.Error(w, "invalid expires_in format", http.StatusBadRequest)
					return
				}
				d = time.Duration(days) * 24 * time.Hour
			} else {
				http.Error(w, "invalid expires_in format", http.StatusBadRequest)
				return
			}
		}
		exp := now.Add(d)
		share.ExpiresAt = &exp
	}

	if err := h.Store.SaveAPIDocShare(r.Context(), share); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         share.ID,
		"token":      share.Token,
		"share_url":  "/api-docs/shared/" + share.Token,
		"expires_at": share.ExpiresAt,
		"created_at": share.CreatedAt,
	})
}

func (h *APIDocHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-ID")
	namespace := r.Header.Get("X-Namespace")
	if tenantID == "" {
		tenantID = "default"
	}
	if namespace == "" {
		namespace = "default"
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	shares, err := h.Store.ListAPIDocShares(r.Context(), tenantID, namespace, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if shares == nil {
		shares = []*store.APIDocShare{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(shares)
}

func (h *APIDocHandler) DeleteShare(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteAPIDocShare(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": id})
}

func (h *APIDocHandler) GetSharedDoc(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}

	share, err := h.Store.GetAPIDocShareByToken(r.Context(), token)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}

	// Check expiration
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		http.Error(w, "this shared document has expired", http.StatusGone)
		return
	}

	// Increment access count
	_ = h.Store.IncrementAPIDocShareAccess(r.Context(), token)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(share)
}

// --- Per-function persisted docs ---

func (h *APIDocHandler) GetFunctionDoc(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	doc, err := h.Store.GetFunctionDoc(r.Context(), name)
	if err != nil {
		http.Error(w, "documentation not found for function: "+name, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (h *APIDocHandler) SaveFunctionDoc(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	var req struct {
		DocContent json.RawMessage `json:"doc_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.DocContent == nil {
		http.Error(w, "doc_content is required", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// Try to get existing doc to preserve created_at
	existing, _ := h.Store.GetFunctionDoc(r.Context(), name)
	createdAt := now
	if existing != nil {
		createdAt = existing.CreatedAt
	}

	doc := &store.FunctionDoc{
		FunctionName: name,
		DocContent:   req.DocContent,
		UpdatedAt:    now,
		CreatedAt:    createdAt,
	}

	if err := h.Store.SaveFunctionDoc(r.Context(), doc); err != nil {
		http.Error(w, "failed to save documentation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (h *APIDocHandler) DeleteFunctionDoc(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteFunctionDoc(r.Context(), name); err != nil {
		http.Error(w, "failed to delete documentation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "function_name": name})
}

// --- Per-workflow persisted docs ---

func (h *APIDocHandler) GetWorkflowDoc(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	doc, err := h.Store.GetWorkflowDoc(r.Context(), name)
	if err != nil {
		http.Error(w, "documentation not found for workflow: "+name, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (h *APIDocHandler) SaveWorkflowDoc(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	var req struct {
		DocContent json.RawMessage `json:"doc_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.DocContent == nil {
		http.Error(w, "doc_content is required", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// Try to get existing doc to preserve created_at
	existing, _ := h.Store.GetWorkflowDoc(r.Context(), name)
	createdAt := now
	if existing != nil {
		createdAt = existing.CreatedAt
	}

	doc := &store.WorkflowDoc{
		WorkflowName: name,
		DocContent:   req.DocContent,
		UpdatedAt:    now,
		CreatedAt:    createdAt,
	}

	if err := h.Store.SaveWorkflowDoc(r.Context(), doc); err != nil {
		http.Error(w, "failed to save documentation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (h *APIDocHandler) DeleteWorkflowDoc(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteWorkflowDoc(r.Context(), name); err != nil {
		http.Error(w, "failed to delete documentation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "workflow_name": name})
}
