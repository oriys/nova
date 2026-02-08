package controlplane

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/oriys/nova/internal/domain"
)

// RegisterWorkflowRoutes registers all workflow-related routes.
func (h *Handler) RegisterWorkflowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /workflows", h.CreateWorkflow)
	mux.HandleFunc("GET /workflows", h.ListWorkflows)
	mux.HandleFunc("GET /workflows/{name}", h.GetWorkflow)
	mux.HandleFunc("DELETE /workflows/{name}", h.DeleteWorkflow)
	mux.HandleFunc("POST /workflows/{name}/versions", h.PublishWorkflowVersion)
	mux.HandleFunc("GET /workflows/{name}/versions", h.ListWorkflowVersions)
	mux.HandleFunc("GET /workflows/{name}/versions/{version}", h.GetWorkflowVersion)
	mux.HandleFunc("POST /workflows/{name}/runs", h.TriggerWorkflowRun)
	mux.HandleFunc("GET /workflows/{name}/runs", h.ListWorkflowRuns)
	mux.HandleFunc("GET /workflows/{name}/runs/{runID}", h.GetWorkflowRun)
}

func (h *Handler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	wf, err := h.WorkflowService.CreateWorkflow(r.Context(), req.Name, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wfWriteJSON(w, http.StatusCreated, wf)
}

func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	wfs, err := h.WorkflowService.ListWorkflows(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if wfs == nil {
		wfs = []*domain.Workflow{}
	}
	wfWriteJSON(w, http.StatusOK, wfs)
}

func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	wf, err := h.WorkflowService.GetWorkflow(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	wfWriteJSON(w, http.StatusOK, wf)
}

func (h *Handler) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.WorkflowService.DeleteWorkflow(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wfWriteJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

func (h *Handler) PublishWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var def domain.WorkflowDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	v, err := h.WorkflowService.PublishVersion(r.Context(), name, &def)
	if err != nil {
		status := http.StatusInternalServerError
		if isValidationError(err) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	wfWriteJSON(w, http.StatusCreated, v)
}

func (h *Handler) ListWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	versions, err := h.WorkflowService.ListVersions(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if versions == nil {
		versions = []*domain.WorkflowVersion{}
	}
	wfWriteJSON(w, http.StatusOK, versions)
}

func (h *Handler) GetWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	versionStr := r.PathValue("version")
	versionNum, err := strconv.Atoi(versionStr)
	if err != nil {
		http.Error(w, "invalid version number", http.StatusBadRequest)
		return
	}

	v, err := h.WorkflowService.GetVersion(r.Context(), name, versionNum)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	wfWriteJSON(w, http.StatusOK, v)
}

func (h *Handler) TriggerWorkflowRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Input json.RawMessage `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	run, err := h.WorkflowService.TriggerRun(r.Context(), name, req.Input, "api")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wfWriteJSON(w, http.StatusCreated, run)
}

func (h *Handler) ListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	runs, err := h.WorkflowService.ListRuns(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []*domain.WorkflowRun{}
	}
	wfWriteJSON(w, http.StatusOK, runs)
}

func (h *Handler) GetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	runID := r.PathValue("runID")
	run, err := h.WorkflowService.GetRun(r.Context(), runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if run.WorkflowName != name {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	wfWriteJSON(w, http.StatusOK, run)
}

func wfWriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func isValidationError(err error) bool {
	msg := err.Error()
	return len(msg) > 12 && msg[:12] == "invalid DAG:"
}
