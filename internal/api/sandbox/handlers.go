// Package sandbox provides HTTP handlers for the sandbox API.
package sandbox

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pkg/httpjson"
	sb "github.com/oriys/nova/internal/sandbox"
)

// Handler provides sandbox REST API endpoints.
type Handler struct {
	Manager *sb.Manager
}

// RegisterRoutes registers all sandbox routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sandboxes", h.CreateSandbox)
	mux.HandleFunc("GET /sandboxes", h.ListSandboxes)
	mux.HandleFunc("GET /sandboxes/{id}", h.GetSandbox)
	mux.HandleFunc("DELETE /sandboxes/{id}", h.DestroySandbox)
	mux.HandleFunc("PATCH /sandboxes/{id}/keepalive", h.Keepalive)

	// Code execution
	mux.HandleFunc("POST /sandboxes/{id}/exec", h.ExecCommand)
	mux.HandleFunc("POST /sandboxes/{id}/code", h.ExecCode)

	// File operations
	mux.HandleFunc("GET /sandboxes/{id}/files", h.FileReadOrList)
	mux.HandleFunc("PUT /sandboxes/{id}/files", h.FileWrite)
	mux.HandleFunc("DELETE /sandboxes/{id}/files", h.FileDelete)

	// Process management
	mux.HandleFunc("GET /sandboxes/{id}/processes", h.ProcessList)
	mux.HandleFunc("DELETE /sandboxes/{id}/processes/{pid}", h.ProcessKill)
}

// CreateSandbox handles POST /sandboxes.
func (h *Handler) CreateSandbox(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	sandbox, err := h.Manager.Create(r.Context(), &req)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpjson.WriteJSON(w, http.StatusCreated, sandbox)
}

// ListSandboxes handles GET /sandboxes.
func (h *Handler) ListSandboxes(w http.ResponseWriter, r *http.Request) {
	sandboxes := h.Manager.List()
	httpjson.WriteJSON(w, http.StatusOK, map[string]any{
		"sandboxes": sandboxes,
		"count":     len(sandboxes),
	})
}

// GetSandbox handles GET /sandboxes/{id}.
func (h *Handler) GetSandbox(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sandbox, err := h.Manager.Get(id)
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, sandbox)
}

// DestroySandbox handles DELETE /sandboxes/{id}.
func (h *Handler) DestroySandbox(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Manager.Destroy(id); err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
}

// Keepalive handles PATCH /sandboxes/{id}/keepalive.
func (h *Handler) Keepalive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sandbox, err := h.Manager.Keepalive(id)
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, sandbox)
}

// ExecCommand handles POST /sandboxes/{id}/exec.
func (h *Handler) ExecCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req domain.SandboxExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		httpjson.Error(w, http.StatusBadRequest, "command is required")
		return
	}

	resp, err := h.Manager.Exec(id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, resp)
}

// ExecCode handles POST /sandboxes/{id}/code.
func (h *Handler) ExecCode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req domain.SandboxCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		httpjson.Error(w, http.StatusBadRequest, "code is required")
		return
	}
	if strings.TrimSpace(req.Language) == "" {
		httpjson.Error(w, http.StatusBadRequest, "language is required")
		return
	}

	resp, err := h.Manager.CodeExec(id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, resp)
}

// FileReadOrList handles GET /sandboxes/{id}/files?path=...
func (h *Handler) FileReadOrList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/home/sandbox"
	}

	// Try listing as directory first, fall back to file read
	listResp, err := h.Manager.FileList(id, path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	if listResp.Error != "" {
		// Not a directory — try reading as file
		readResp, err := h.Manager.FileRead(id, path)
		if err != nil {
			httpjson.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if readResp.Error != "" {
			httpjson.Error(w, http.StatusNotFound, readResp.Error)
			return
		}
		httpjson.WriteJSON(w, http.StatusOK, map[string]string{
			"path":    path,
			"content": readResp.Content,
		})
		return
	}

	httpjson.WriteJSON(w, http.StatusOK, map[string]any{
		"path":    path,
		"entries": listResp.Entries,
	})
}

// FileWrite handles PUT /sandboxes/{id}/files?path=...
func (h *Handler) FileWrite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req domain.SandboxFileWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		// Fall back to query param
		req.Path = r.URL.Query().Get("path")
	}
	if strings.TrimSpace(req.Path) == "" {
		httpjson.Error(w, http.StatusBadRequest, "path is required")
		return
	}

	perm := req.Perm
	if perm == 0 {
		perm = 0644
	}

	resp, err := h.Manager.FileWrite(id, req.Path, req.Content, perm)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp.Error != "" {
		httpjson.Error(w, http.StatusInternalServerError, resp.Error)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"status": "written"})
}

// FileDelete handles DELETE /sandboxes/{id}/files?path=...
func (h *Handler) FileDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := r.URL.Query().Get("path")
	if path == "" {
		httpjson.Error(w, http.StatusBadRequest, "path query parameter is required")
		return
	}

	resp, err := h.Manager.FileDelete(id, path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp.Error != "" {
		httpjson.Error(w, http.StatusInternalServerError, resp.Error)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ProcessList handles GET /sandboxes/{id}/processes.
func (h *Handler) ProcessList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resp, err := h.Manager.ProcessList(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp.Error != "" {
		httpjson.Error(w, http.StatusInternalServerError, resp.Error)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]any{
		"processes": resp.Processes,
	})
}

// ProcessKill handles DELETE /sandboxes/{id}/processes/{pid}.
func (h *Handler) ProcessKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pidStr := r.PathValue("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid pid: "+pidStr)
		return
	}

	signal := 15 // SIGTERM
	if sigStr := r.URL.Query().Get("signal"); sigStr != "" {
		if s, err := strconv.Atoi(sigStr); err == nil {
			signal = s
		}
	}

	resp, err := h.Manager.ProcessKill(id, pid, signal)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httpjson.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resp.Error != "" {
		httpjson.Error(w, http.StatusInternalServerError, resp.Error)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}
