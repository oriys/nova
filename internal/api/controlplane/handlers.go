package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

// Handler handles control plane HTTP requests (function lifecycle and snapshot management).
type Handler struct {
	Store     *store.Store
	Pool      *pool.Pool
	Backend   backend.Backend
	FCAdapter *firecracker.Adapter // Optional: for Firecracker-specific features
	Compiler  *compiler.Compiler
}

// RegisterRoutes registers all control plane routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Function CRUD
	mux.HandleFunc("POST /functions", h.CreateFunction)
	mux.HandleFunc("GET /functions", h.ListFunctions)
	mux.HandleFunc("GET /functions/{name}", h.GetFunction)
	mux.HandleFunc("PATCH /functions/{name}", h.UpdateFunction)
	mux.HandleFunc("DELETE /functions/{name}", h.DeleteFunction)

	// Function code
	mux.HandleFunc("GET /functions/{name}/code", h.GetFunctionCode)
	mux.HandleFunc("PUT /functions/{name}/code", h.UpdateFunctionCode)

	// Runtimes
	mux.HandleFunc("GET /runtimes", h.ListRuntimes)
	mux.HandleFunc("POST /runtimes", h.CreateRuntime)
	mux.HandleFunc("DELETE /runtimes/{id}", h.DeleteRuntime)

	// Configuration
	mux.HandleFunc("GET /config", h.GetConfig)
	mux.HandleFunc("PUT /config", h.UpdateConfig)

	// Snapshot management
	mux.HandleFunc("GET /snapshots", h.ListSnapshots)
	mux.HandleFunc("POST /functions/{name}/snapshot", h.CreateSnapshot)
	mux.HandleFunc("DELETE /functions/{name}/snapshot", h.DeleteSnapshot)
}

// CreateFunction handles POST /functions
func (h *Handler) CreateFunction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string                 `json:"name"`
		Runtime     string                 `json:"runtime"`
		Handler     string                 `json:"handler"`
		CodePath    string                 `json:"code_path"`
		Code        string                 `json:"code"`
		MemoryMB    int                    `json:"memory_mb"`
		TimeoutS    int                    `json:"timeout_s"`
		MinReplicas int                    `json:"min_replicas"`
		MaxReplicas int                    `json:"max_replicas"`
		Mode        string                 `json:"mode"`
		EnvVars     map[string]string      `json:"env_vars"`
		Limits      *domain.ResourceLimits `json:"limits"`
	}

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

	rt := domain.Runtime(req.Runtime)
	if !rt.IsValid() {
		http.Error(w, "invalid runtime", http.StatusBadRequest)
		return
	}

	// Check if function name already exists
	if existing, _ := h.Store.GetFunctionByName(r.Context(), req.Name); existing != nil {
		http.Error(w, fmt.Sprintf("function '%s' already exists", req.Name), http.StatusConflict)
		return
	}

	// Set defaults
	if req.Handler == "" {
		req.Handler = "main.handler"
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 128
	}
	if req.TimeoutS == 0 {
		req.TimeoutS = 30
	}
	if req.Mode == "" {
		req.Mode = "process"
	}

	// Determine code path - either from code_path or write inline code
	codePath := req.CodePath
	var codeHash string

	if req.Code != "" {
		// Inline code provided - will be handled by compiler
		codeHash = domain.HashSourceCode(req.Code)
		// Temp path until compiler writes it
		funcDir := filepath.Join(os.TempDir(), "nova-functions")
		os.MkdirAll(funcDir, 0755)
		ext := runtimeExtension(rt)
		codePath = filepath.Join(funcDir, req.Name+ext)
	} else {
		// Check if code file exists
		if _, err := os.Stat(req.CodePath); os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("code path not found: %s", req.CodePath), http.StatusBadRequest)
			return
		}
		codeHash, _ = domain.HashCodeFile(codePath)
	}

	fn := &domain.Function{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Runtime:     rt,
		Handler:     req.Handler,
		CodePath:    codePath,
		CodeHash:    codeHash,
		MemoryMB:    req.MemoryMB,
		TimeoutS:    req.TimeoutS,
		MinReplicas: req.MinReplicas,
		MaxReplicas: req.MaxReplicas,
		Mode:        domain.ExecutionMode(req.Mode),
		EnvVars:     req.EnvVars,
		Limits:      req.Limits,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.Store.SaveFunction(r.Context(), fn); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If inline code was provided, save it and trigger compilation
	if req.Code != "" {
		sourceHash := domain.HashSourceCode(req.Code)
		if err := h.Store.SaveFunctionCode(r.Context(), fn.ID, req.Code, sourceHash); err != nil {
			http.Error(w, fmt.Sprintf("save code: %v", err), http.StatusInternalServerError)
			return
		}

		// Trigger compilation (async for compiled languages, sync for interpreted)
		if h.Compiler != nil {
			h.Compiler.CompileAsync(r.Context(), fn, req.Code)
		}
	}

	// Build response with compile status if code was provided
	response := map[string]interface{}{
		"id":           fn.ID,
		"name":         fn.Name,
		"runtime":      fn.Runtime,
		"handler":      fn.Handler,
		"code_path":    fn.CodePath,
		"code_hash":    fn.CodeHash,
		"memory_mb":    fn.MemoryMB,
		"timeout_s":    fn.TimeoutS,
		"min_replicas": fn.MinReplicas,
		"max_replicas": fn.MaxReplicas,
		"mode":         fn.Mode,
		"env_vars":     fn.EnvVars,
		"limits":       fn.Limits,
		"created_at":   fn.CreatedAt,
		"updated_at":   fn.UpdatedAt,
	}

	if req.Code != "" {
		if domain.NeedsCompilation(rt) {
			response["compile_status"] = domain.CompileStatusCompiling
		} else {
			response["compile_status"] = domain.CompileStatusNotRequired
		}
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
	// Ensure we return empty array instead of null
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

	// Check if code is being updated
	codeChanged := update.CodePath != nil

	fn, err := h.Store.UpdateFunction(r.Context(), name, &update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Evict VMs and invalidate snapshot if code changed
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

	// Evict all VMs for this function first
	h.Pool.Evict(fn.ID)

	// Delete snapshot if exists
	_ = executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)

	// Delete all versions
	versions, _ := h.Store.ListVersions(r.Context(), fn.ID)
	for _, v := range versions {
		_ = h.Store.DeleteVersion(r.Context(), fn.ID, v.Version)
	}

	// Delete all aliases
	aliases, _ := h.Store.ListAliases(r.Context(), fn.ID)
	for _, a := range aliases {
		_ = h.Store.DeleteAlias(r.Context(), fn.ID, a.Name)
	}

	// Delete function code
	_ = h.Store.DeleteFunctionCode(r.Context(), fn.ID)

	// Finally delete the function
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
		// No code record - try to read from file path
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

	sourceHash := domain.HashSourceCode(req.Code)

	// Update code in database
	if err := h.Store.UpdateFunctionCode(r.Context(), fn.ID, req.Code, sourceHash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Evict VMs and invalidate snapshot
	h.Pool.Evict(fn.ID)
	executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)
	h.Pool.InvalidateSnapshotCache(fn.ID)
	logging.Op().Info("invalidated snapshot", "function", fn.Name, "reason", "code_updated")

	// Trigger compilation
	var compileStatus domain.CompileStatus
	if h.Compiler != nil {
		h.Compiler.CompileAsync(r.Context(), fn, req.Code)
		if domain.NeedsCompilation(fn.Runtime) {
			compileStatus = domain.CompileStatusCompiling
		} else {
			compileStatus = domain.CompileStatusNotRequired
		}
	} else {
		// No compiler - just write to file directly for interpreted languages
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

// runtimeExtension returns the file extension for a runtime
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

// ListRuntimes handles GET /runtimes
func (h *Handler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	runtimes, err := h.Store.ListRuntimes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure we return empty array instead of null
	if runtimes == nil {
		runtimes = []*store.RuntimeRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runtimes)
}

// CreateRuntime handles POST /runtimes
func (h *Handler) CreateRuntime(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Status  string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		http.Error(w, "version is required", http.StatusBadRequest)
		return
	}
	if req.Status == "" {
		req.Status = "available"
	}

	rt := &store.RuntimeRecord{
		ID:      req.ID,
		Name:    req.Name,
		Version: req.Version,
		Status:  req.Status,
	}

	if err := h.Store.SaveRuntime(r.Context(), rt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rt)
}

// DeleteRuntime handles DELETE /runtimes/{id}
func (h *Handler) DeleteRuntime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "runtime id is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteRuntime(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "deleted",
		"id":      id,
	})
}

// GetConfig handles GET /config
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.Store.GetConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure we return empty object instead of null
	if config == nil {
		config = make(map[string]string)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// UpdateConfig handles PUT /config
func (h *Handler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	for key, value := range updates {
		if err := h.Store.SetConfig(r.Context(), key, value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return updated config
	config, err := h.Store.GetConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// ListSnapshots handles GET /snapshots
func (h *Handler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	funcs, err := h.Store.ListFunctions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type snapshotInfo struct {
		FunctionID   string `json:"function_id"`
		FunctionName string `json:"function_name"`
		SnapSize     int64  `json:"snap_size"`
		MemSize      int64  `json:"mem_size"`
		TotalSize    int64  `json:"total_size"`
		CreatedAt    string `json:"created_at"`
	}

	var snapshots []snapshotInfo
	for _, fn := range funcs {
		if executor.HasSnapshot(h.Backend.SnapshotDir(), fn.ID) {
			snapPath := filepath.Join(h.Backend.SnapshotDir(), fn.ID+".snap")
			memPath := filepath.Join(h.Backend.SnapshotDir(), fn.ID+".mem")

			snapInfo, _ := os.Stat(snapPath)
			memInfo, _ := os.Stat(memPath)

			var snapSize, memSize int64
			var createdAt string
			if snapInfo != nil {
				snapSize = snapInfo.Size()
				createdAt = snapInfo.ModTime().Format(time.RFC3339)
			}
			if memInfo != nil {
				memSize = memInfo.Size()
			}

			snapshots = append(snapshots, snapshotInfo{
				FunctionID:   fn.ID,
				FunctionName: fn.Name,
				SnapSize:     snapSize,
				MemSize:      memSize,
				TotalSize:    snapSize + memSize,
				CreatedAt:    createdAt,
			})
		}
	}

	// Ensure we return empty array instead of null
	if snapshots == nil {
		snapshots = []snapshotInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshots)
}

// CreateSnapshot handles POST /functions/{name}/snapshot
func (h *Handler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	// Snapshots only supported with Firecracker backend
	if h.FCAdapter == nil {
		http.Error(w, "Snapshots are only supported with Firecracker backend", http.StatusNotImplemented)
		return
	}

	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Check if snapshot already exists
	if executor.HasSnapshot(h.Backend.SnapshotDir(), fn.ID) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "exists",
			"message": "Snapshot already exists for this function",
		})
		return
	}

	// Acquire a VM and create snapshot
	pvm, err := h.Pool.Acquire(r.Context(), fn)
	if err != nil {
		http.Error(w, fmt.Sprintf("acquire VM: %v", err), http.StatusInternalServerError)
		return
	}

	mgr := h.FCAdapter.Manager()
	if err := mgr.CreateSnapshot(r.Context(), pvm.VM.ID, fn.ID); err != nil {
		h.Pool.Release(pvm)
		http.Error(w, fmt.Sprintf("create snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	// Stop the VM after snapshotting (it's paused)
	pvm.Client.Close()
	h.Backend.StopVM(pvm.VM.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "created",
		"message": fmt.Sprintf("Snapshot created for %s", fn.Name),
	})
}

// DeleteSnapshot handles DELETE /functions/{name}/snapshot
func (h *Handler) DeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if !executor.HasSnapshot(h.Backend.SnapshotDir(), fn.ID) {
		http.Error(w, "No snapshot exists for this function", http.StatusNotFound)
		return
	}

	if err := executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID); err != nil {
		http.Error(w, fmt.Sprintf("delete snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "deleted",
		"message": fmt.Sprintf("Snapshot deleted for %s", fn.Name),
	})
}
