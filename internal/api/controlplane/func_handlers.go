package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pkg/crypto"
	"github.com/oriys/nova/internal/service"
	"github.com/oriys/nova/internal/store"
)

// CreateFunction handles POST /functions
func (h *Handler) CreateFunction(w http.ResponseWriter, r *http.Request) {
	var req service.CreateFunctionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Runtime == "" {
		http.Error(w, "runtime is required", http.StatusBadRequest)
		return
	}
	if req.CodePath == "" && req.Code == "" {
		http.Error(w, "code_path or code is required", http.StatusBadRequest)
		return
	}

	fn, compileStatus, err := h.FunctionService.CreateFunction(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response
	response := map[string]interface{}{
		"id":             fn.ID,
		"name":           fn.Name,
		"runtime":        fn.Runtime,
		"handler":        fn.Handler,
		"code_path":      fn.CodePath,
		"code_hash":      fn.CodeHash,
		"memory_mb":      fn.MemoryMB,
		"timeout_s":      fn.TimeoutS,
		"min_replicas":   fn.MinReplicas,
		"max_replicas":   fn.MaxReplicas,
		"mode":           fn.Mode,
		"env_vars":       fn.EnvVars,
		"limits":         fn.Limits,
		"created_at":     fn.CreatedAt,
		"updated_at":     fn.UpdatedAt,
		"compile_status": compileStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ListFunctions handles GET /functions
func (h *Handler) ListFunctions(w http.ResponseWriter, r *http.Request) {
	funcs, err := h.Store.ListFunctions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if funcs == nil {
		funcs = []*domain.Function{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(funcs)
}

// GetFunction handles GET /functions/{name}
func (h *Handler) GetFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fn)
}

// UpdateFunction handles PATCH /functions/{name}
func (h *Handler) UpdateFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var update store.FunctionUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	codeChanged := update.CodePath != nil

	fn, err := h.Store.UpdateFunction(r.Context(), name, &update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if codeChanged {
		h.Pool.Evict(fn.ID)
		executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)
		h.Pool.InvalidateSnapshotCache(fn.ID)
		logging.Op().Info("invalidated snapshot", "function", fn.Name, "reason", "code_changed")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fn)
}

// DeleteFunction handles DELETE /functions/{name}
func (h *Handler) DeleteFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	h.Pool.Evict(fn.ID)
	_ = executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)

	versions, _ := h.Store.ListVersions(r.Context(), fn.ID)
	for _, v := range versions {
		_ = h.Store.DeleteVersion(r.Context(), fn.ID, v.Version)
	}

	aliases, _ := h.Store.ListAliases(r.Context(), fn.ID)
	for _, a := range aliases {
		_ = h.Store.DeleteAlias(r.Context(), fn.ID, a.Name)
	}

	_ = h.Store.DeleteFunctionCode(r.Context(), fn.ID)

	if err := h.Store.DeleteFunction(r.Context(), fn.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "deleted",
		"name":             name,
		"versions_deleted": len(versions),
		"aliases_deleted":  len(aliases),
	})
}

// GetFunctionCode handles GET /functions/{name}/code
func (h *Handler) GetFunctionCode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	code, err := h.Store.GetFunctionCode(r.Context(), fn.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"function_id": fn.ID,
	}

	if code != nil {
		response["source_code"] = code.SourceCode
		response["source_hash"] = code.SourceHash
		response["compile_status"] = code.CompileStatus
		if code.CompileError != "" {
			response["compile_error"] = code.CompileError
		}
		if code.BinaryHash != "" {
			response["binary_hash"] = code.BinaryHash
		}
	} else {
		if fn.CodePath != "" {
			if data, err := os.ReadFile(fn.CodePath); err == nil {
				response["source_code"] = string(data)
				response["compile_status"] = domain.CompileStatusNotRequired
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateFunctionCode handles PUT /functions/{name}/code
func (h *Handler) UpdateFunctionCode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	sourceHash := crypto.HashString(req.Code)

	if err := h.Store.UpdateFunctionCode(r.Context(), fn.ID, req.Code, sourceHash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Pool.Evict(fn.ID)
	executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)
	h.Pool.InvalidateSnapshotCache(fn.ID)
	logging.Op().Info("invalidated snapshot", "function", fn.Name, "reason", "code_updated")

	var compileStatus domain.CompileStatus
	if h.Compiler != nil {
		h.Compiler.CompileAsync(r.Context(), fn, req.Code)
		if domain.NeedsCompilation(fn.Runtime) {
			compileStatus = domain.CompileStatusCompiling
		} else {
			compileStatus = domain.CompileStatusNotRequired
		}
	} else {
		if !domain.NeedsCompilation(fn.Runtime) {
			funcDir := filepath.Join(os.TempDir(), "nova-functions")
			os.MkdirAll(funcDir, 0755)
			ext := runtimeExtension(fn.Runtime)
			codePath := filepath.Join(funcDir, fn.Name+ext)
			if err := os.WriteFile(codePath, []byte(req.Code), 0644); err != nil {
				http.Error(w, fmt.Sprintf("write code: %v", err), http.StatusInternalServerError)
				return
			}
			fn.CodePath = codePath
			fn.CodeHash = sourceHash
			h.Store.SaveFunction(r.Context(), fn)
			h.Store.UpdateCompileResult(r.Context(), fn.ID, []byte(req.Code), sourceHash, domain.CompileStatusNotRequired, "")
			compileStatus = domain.CompileStatusNotRequired
		} else {
			compileStatus = domain.CompileStatusPending
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"compile_status": compileStatus,
		"source_hash":    sourceHash,
	})
}

func runtimeExtension(runtime domain.Runtime) string {
	exts := map[domain.Runtime]string{
		domain.RuntimePython: ".py",
		domain.RuntimeGo:     ".go",
		domain.RuntimeRust:   ".rs",
		domain.RuntimeNode:   ".js",
		domain.RuntimeRuby:   ".rb",
		domain.RuntimeJava:   ".java",
		domain.RuntimeDeno:   ".ts",
		domain.RuntimeBun:    ".ts",
		domain.RuntimeWasm:   ".wasm",
		domain.RuntimePHP:    ".php",
		domain.RuntimeDotnet: ".cs",
		domain.RuntimeElixir: ".exs",
		domain.RuntimeKotlin: ".kt",
		domain.RuntimeSwift:  ".swift",
		domain.RuntimeZig:    ".zig",
		domain.RuntimeLua:    ".lua",
		domain.RuntimePerl:   ".pl",
		domain.RuntimeR:      ".R",
		domain.RuntimeJulia:  ".jl",
		domain.RuntimeScala:  ".scala",
	}
	if ext, ok := exts[runtime]; ok {
		return ext
	}
	return ".txt"
}
