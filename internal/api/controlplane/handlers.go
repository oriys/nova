package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

// Handler handles control plane HTTP requests (function lifecycle and snapshot management).
type Handler struct {
	Store *store.Store
	Pool  *pool.Pool
	Mgr   *firecracker.Manager
}

// RegisterRoutes registers all control plane routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Function CRUD
	mux.HandleFunc("POST /functions", h.CreateFunction)
	mux.HandleFunc("GET /functions", h.ListFunctions)
	mux.HandleFunc("GET /functions/{name}", h.GetFunction)
	mux.HandleFunc("PATCH /functions/{name}", h.UpdateFunction)
	mux.HandleFunc("DELETE /functions/{name}", h.DeleteFunction)

	// Runtimes
	mux.HandleFunc("GET /runtimes", h.ListRuntimes)

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

	// If code is provided directly, write it to a temp file
	codePath := req.CodePath
	if req.Code != "" {
		// Determine file extension based on runtime
		ext := map[string]string{
			"python": ".py",
			"go":     ".go",
			"rust":   ".rs",
			"node":   ".js",
			"ruby":   ".rb",
			"java":   ".java",
			"deno":   ".ts",
			"bun":    ".ts",
			"wasm":   ".wasm",
			"php":    ".php",
			"dotnet": ".cs",
			"elixir": ".exs",
			"kotlin": ".kt",
			"swift":  ".swift",
			"zig":    ".zig",
			"lua":    ".lua",
			"perl":   ".pl",
			"r":      ".R",
			"julia":  ".jl",
			"scala":  ".scala",
		}[req.Runtime]
		if ext == "" {
			ext = ".txt"
		}

		// Create functions directory if not exists
		funcDir := filepath.Join(os.TempDir(), "nova-functions")
		if err := os.MkdirAll(funcDir, 0755); err != nil {
			http.Error(w, fmt.Sprintf("failed to create functions dir: %v", err), http.StatusInternalServerError)
			return
		}

		// Write code to file
		codePath = filepath.Join(funcDir, req.Name+ext)
		if err := os.WriteFile(codePath, []byte(req.Code), 0644); err != nil {
			http.Error(w, fmt.Sprintf("failed to write code file: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Check if code file exists
		if _, err := os.Stat(req.CodePath); os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("code path not found: %s", req.CodePath), http.StatusBadRequest)
			return
		}
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

	// Calculate code hash
	codeHash, _ := domain.HashCodeFile(codePath)

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(fn)
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
		executor.InvalidateSnapshot(h.Mgr.SnapshotDir(), fn.ID)
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
	_ = executor.InvalidateSnapshot(h.Mgr.SnapshotDir(), fn.ID)

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

// ListRuntimes handles GET /runtimes
func (h *Handler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	runtimes := []map[string]interface{}{
		// Python versions
		{"id": "python", "name": "Python", "version": "3.12", "status": "available"},
		{"id": "python3.11", "name": "Python", "version": "3.11", "status": "available"},
		{"id": "python3.10", "name": "Python", "version": "3.10", "status": "available"},
		{"id": "python3.9", "name": "Python", "version": "3.9", "status": "available"},
		// Go versions
		{"id": "go", "name": "Go", "version": "1.22", "status": "available"},
		{"id": "go1.21", "name": "Go", "version": "1.21", "status": "available"},
		{"id": "go1.20", "name": "Go", "version": "1.20", "status": "available"},
		// Node.js versions
		{"id": "node", "name": "Node.js", "version": "22.x", "status": "available"},
		{"id": "node20", "name": "Node.js", "version": "20.x", "status": "available"},
		{"id": "node18", "name": "Node.js", "version": "18.x", "status": "available"},
		// Rust versions
		{"id": "rust", "name": "Rust", "version": "1.76", "status": "available"},
		{"id": "rust1.75", "name": "Rust", "version": "1.75", "status": "available"},
		// Deno & Bun
		{"id": "deno", "name": "Deno", "version": "1.40", "status": "available"},
		{"id": "bun", "name": "Bun", "version": "1.0", "status": "available"},
		// Ruby versions
		{"id": "ruby", "name": "Ruby", "version": "3.3", "status": "available"},
		{"id": "ruby3.2", "name": "Ruby", "version": "3.2", "status": "available"},
		// JVM languages
		{"id": "java", "name": "Java", "version": "21", "status": "available"},
		{"id": "java17", "name": "Java", "version": "17", "status": "available"},
		{"id": "java11", "name": "Java", "version": "11", "status": "available"},
		{"id": "kotlin", "name": "Kotlin", "version": "1.9", "status": "available"},
		{"id": "scala", "name": "Scala", "version": "3.3", "status": "available"},
		// Other languages
		{"id": "php", "name": "PHP", "version": "8.3", "status": "available"},
		{"id": "php8.2", "name": "PHP", "version": "8.2", "status": "available"},
		{"id": "dotnet", "name": ".NET", "version": "8.0", "status": "available"},
		{"id": "dotnet7", "name": ".NET", "version": "7.0", "status": "available"},
		{"id": "elixir", "name": "Elixir", "version": "1.16", "status": "available"},
		{"id": "swift", "name": "Swift", "version": "5.9", "status": "available"},
		{"id": "zig", "name": "Zig", "version": "0.11", "status": "available"},
		{"id": "lua", "name": "Lua", "version": "5.4", "status": "available"},
		{"id": "perl", "name": "Perl", "version": "5.38", "status": "available"},
		{"id": "r", "name": "R", "version": "4.3", "status": "available"},
		{"id": "julia", "name": "Julia", "version": "1.10", "status": "available"},
		{"id": "wasm", "name": "WebAssembly", "version": "wasmtime", "status": "available"},
	}

	// Count functions per runtime
	funcs, _ := h.Store.ListFunctions(r.Context())
	runtimeCounts := make(map[string]int)
	for _, fn := range funcs {
		runtimeCounts[string(fn.Runtime)]++
	}

	// Add function counts to runtimes
	for i := range runtimes {
		id := runtimes[i]["id"].(string)
		runtimes[i]["functions_count"] = runtimeCounts[id]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runtimes)
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
		if executor.HasSnapshot(h.Mgr.SnapshotDir(), fn.ID) {
			snapPath := filepath.Join(h.Mgr.SnapshotDir(), fn.ID+".snap")
			memPath := filepath.Join(h.Mgr.SnapshotDir(), fn.ID+".mem")

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
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Check if snapshot already exists
	if executor.HasSnapshot(h.Mgr.SnapshotDir(), fn.ID) {
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

	if err := h.Mgr.CreateSnapshot(r.Context(), pvm.VM.ID, fn.ID); err != nil {
		h.Pool.Release(pvm)
		http.Error(w, fmt.Sprintf("create snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	// Stop the VM after snapshotting (it's paused)
	pvm.Client.Close()
	h.Mgr.StopVM(pvm.VM.ID)

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

	if !executor.HasSnapshot(h.Mgr.SnapshotDir(), fn.ID) {
		http.Error(w, "No snapshot exists for this function", http.StatusNotFound)
		return
	}

	if err := executor.InvalidateSnapshot(h.Mgr.SnapshotDir(), fn.ID); err != nil {
		http.Error(w, fmt.Sprintf("delete snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "deleted",
		"message": fmt.Sprintf("Snapshot deleted for %s", fn.Name),
	})
}
