package controlplane

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

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
	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	fn, compileStatus, err := h.FunctionService.CreateFunction(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response
	response := map[string]interface{}{
		"id":                   fn.ID,
		"name":                 fn.Name,
		"runtime":              fn.Runtime,
		"handler":              fn.Handler,
		"code_hash":            fn.CodeHash,
		"memory_mb":            fn.MemoryMB,
		"timeout_s":            fn.TimeoutS,
		"min_replicas":         fn.MinReplicas,
		"max_replicas":         fn.MaxReplicas,
		"mode":                 fn.Mode,
		"instance_concurrency": fn.InstanceConcurrency,
		"env_vars":             fn.EnvVars,
		"limits":               fn.Limits,
		"created_at":           fn.CreatedAt,
		"updated_at":           fn.UpdatedAt,
		"compile_status":       compileStatus,
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

	codeChanged := update.Code != nil

	fn, err := h.Store.UpdateFunction(r.Context(), name, &update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if codeChanged {
		// Update code in store
		sourceHash := crypto.HashString(*update.Code)
		if err := h.Store.UpdateFunctionCode(r.Context(), fn.ID, *update.Code, sourceHash); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Try hot reload for interpreted languages
		hotReloaded := false
		if !domain.NeedsCompilation(fn.Runtime) {
			reloadFiles := map[string][]byte{"handler": []byte(*update.Code)}
			if err := h.Pool.ReloadCode(fn.ID, reloadFiles); err == nil {
				hotReloaded = true
				logging.Op().Info("hot reloaded code", "function", fn.Name)
			}
		}
		if !hotReloaded {
			h.Pool.Evict(fn.ID)
		}
		executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)
		h.Pool.InvalidateSnapshotCache(fn.ID)
		logging.Op().Info("invalidated snapshot", "function", fn.Name, "reason", "code_changed")

		// Trigger recompilation if compiler is available
		if h.Compiler != nil {
			h.Compiler.CompileAsync(r.Context(), fn, *update.Code)
		}
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
		response["source_code"] = ""
		response["compile_status"] = domain.CompileStatusPending
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateFunctionCode handles PUT /functions/{name}/code
// Supports both JSON body (single file) and multipart/form-data (archive upload)
func (h *Handler) UpdateFunctionCode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	contentType := r.Header.Get("Content-Type")

	var sourceCode string
	var sourceHash string
	var files map[string][]byte
	var entryPoint string

	if strings.Contains(contentType, "multipart/form-data") {
		// Handle multipart upload (archive or multiple files)
		if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
			http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}

		entryPoint = r.FormValue("entry_point")
		archiveType := r.FormValue("archive_type")

		// Check for archive file
		archiveFile, archiveHeader, err := r.FormFile("archive")
		if err == nil {
			defer archiveFile.Close()

			// Auto-detect archive type from filename if not provided
			if archiveType == "" {
				filename := strings.ToLower(archiveHeader.Filename)
				if strings.HasSuffix(filename, ".zip") {
					archiveType = "zip"
				} else if strings.HasSuffix(filename, ".tar.gz") || strings.HasSuffix(filename, ".tgz") {
					archiveType = "tar.gz"
				} else if strings.HasSuffix(filename, ".tar") {
					archiveType = "tar"
				}
			}

			archiveData, err := io.ReadAll(archiveFile)
			if err != nil {
				http.Error(w, "failed to read archive: "+err.Error(), http.StatusBadRequest)
				return
			}

			// Auto-detect from content if still unknown
			if archiveType == "" {
				archiveType = DetectArchiveType(archiveData)
			}

			if archiveType == "" {
				http.Error(w, "cannot detect archive type, please specify archive_type", http.StatusBadRequest)
				return
			}

			files, err = ExtractArchive(archiveData, archiveType)
			if err != nil {
				http.Error(w, "failed to extract archive: "+err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			// Check for single code file
			codeFile, _, err := r.FormFile("code")
			if err == nil {
				defer codeFile.Close()
				codeData, err := io.ReadAll(codeFile)
				if err != nil {
					http.Error(w, "failed to read code file: "+err.Error(), http.StatusBadRequest)
					return
				}
				sourceCode = string(codeData)
			} else if code := r.FormValue("code"); code != "" {
				// Check for code as form field
				sourceCode = code
			} else {
				http.Error(w, "either archive or code is required", http.StatusBadRequest)
				return
			}
		}
	} else {
		// Handle JSON body (backward compatible)
		var req struct {
			Code       string `json:"code"`
			EntryPoint string `json:"entry_point,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Code == "" {
			http.Error(w, "code is required", http.StatusBadRequest)
			return
		}

		sourceCode = req.Code
		entryPoint = req.EntryPoint
	}

	// Handle multi-file case
	if files != nil && len(files) > 0 {
		// Install dependencies if available (requirements.txt, package.json)
		if h.Compiler != nil {
			filesWithDeps, err := h.Compiler.CompileWithDeps(r.Context(), fn, files)
			if err != nil {
				logging.Op().Warn("dependency installation failed", "function", fn.Name, "error", err)
				// Continue without deps
			} else if len(filesWithDeps) > len(files) {
				files = filesWithDeps
				logging.Op().Info("dependencies installed", "function", fn.Name, "total_files", len(files))
			}
		}

		// Save files to database
		if err := h.Store.SaveFunctionFiles(r.Context(), fn.ID, files); err != nil {
			http.Error(w, "failed to save files: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Determine entry point if not specified
		if entryPoint == "" {
			entryPoint = detectEntryPoint(files, fn.Runtime)
		}

		// Compute hash from all files
		sourceHash = computeFilesHash(files)

		// For backward compatibility, store entry point file as source code
		if entryContent, ok := files[entryPoint]; ok {
			sourceCode = string(entryContent)
		} else {
			// Use first file as source if entry point not found
			for _, content := range files {
				sourceCode = string(content)
				break
			}
		}
	} else {
		// Single file case
		sourceHash = crypto.HashString(sourceCode)
		// Clear any existing multi-file storage
		_ = h.Store.DeleteFunctionFiles(r.Context(), fn.ID)
	}

	if err := h.Store.UpdateFunctionCode(r.Context(), fn.ID, sourceCode, sourceHash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update function entry point if specified
	if entryPoint != "" && entryPoint != fn.Handler {
		fn.Handler = entryPoint
		_ = h.Store.SaveFunction(r.Context(), fn)
	}

	// Hot reload or evict VMs
	hotReloaded := false
	if !domain.NeedsCompilation(fn.Runtime) {
		// For interpreted languages, try hot reload first
		var reloadFiles map[string][]byte
		if files != nil && len(files) > 0 {
			reloadFiles = files
		} else {
			reloadFiles = map[string][]byte{"handler": []byte(sourceCode)}
		}
		if err := h.Pool.ReloadCode(fn.ID, reloadFiles); err == nil {
			hotReloaded = true
			logging.Op().Info("hot reloaded code", "function", fn.Name)
		} else {
			logging.Op().Warn("hot reload failed, falling back to evict", "function", fn.Name, "error", err)
		}
	}

	if !hotReloaded {
		h.Pool.Evict(fn.ID)
	}
	executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)
	h.Pool.InvalidateSnapshotCache(fn.ID)
	logging.Op().Info("invalidated snapshot", "function", fn.Name, "reason", "code_updated")

	var compileStatus domain.CompileStatus
	if h.Compiler != nil {
		h.Compiler.CompileAsync(r.Context(), fn, sourceCode)
		if domain.NeedsCompilation(fn.Runtime) {
			compileStatus = domain.CompileStatusCompiling
		} else {
			compileStatus = domain.CompileStatusNotRequired
		}
	} else {
		if !domain.NeedsCompilation(fn.Runtime) {
			// Store source as compiled artifact for interpreted languages
			h.Store.UpdateCompileResult(r.Context(), fn.ID, []byte(sourceCode), sourceHash, domain.CompileStatusNotRequired, "")
			compileStatus = domain.CompileStatusNotRequired
		} else {
			compileStatus = domain.CompileStatusPending
		}
	}

	// Update function's code hash
	fn.CodeHash = sourceHash
	h.Store.SaveFunction(r.Context(), fn)

	response := map[string]interface{}{
		"compile_status": compileStatus,
		"source_hash":    sourceHash,
	}
	if files != nil {
		response["file_count"] = len(files)
	}
	if entryPoint != "" {
		response["entry_point"] = entryPoint
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListFunctionFiles handles GET /functions/{name}/files
func (h *Handler) ListFunctionFiles(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	files, err := h.Store.ListFunctionFiles(r.Context(), fn.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add is_entry flag
	type fileResponse struct {
		Path    string `json:"path"`
		Size    int    `json:"size"`
		IsEntry bool   `json:"is_entry"`
	}

	response := make([]fileResponse, 0, len(files))
	for _, f := range files {
		response = append(response, fileResponse{
			Path:    f.Path,
			Size:    f.Size,
			IsEntry: f.Path == fn.Handler,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"files": response,
	})
}

// detectEntryPoint tries to find the entry point file based on runtime conventions
func detectEntryPoint(files map[string][]byte, runtime domain.Runtime) string {
	// Common entry point names by runtime
	entryPoints := map[domain.Runtime][]string{
		domain.RuntimePython: {"handler.py", "main.py", "app.py", "index.py"},
		domain.RuntimeNode:   {"handler.js", "index.js", "main.js", "app.js"},
		domain.RuntimeGo:     {"handler", "main.go", "handler.go"},
		domain.RuntimeRust:   {"handler", "main.rs"},
		domain.RuntimeRuby:   {"handler.rb", "main.rb", "app.rb"},
		domain.RuntimeJava:   {"Handler.java", "Main.java", "handler.jar"},
		domain.RuntimePHP:    {"handler.php", "index.php", "main.php"},
		domain.RuntimeDeno:   {"handler.ts", "main.ts", "index.ts"},
		domain.RuntimeBun:    {"handler.ts", "handler.js", "index.ts", "index.js"},
	}

	// Check runtime-specific entry points
	if candidates, ok := entryPoints[runtime]; ok {
		for _, candidate := range candidates {
			if _, exists := files[candidate]; exists {
				return candidate
			}
		}
	}

	// Fallback: look for common patterns
	commonNames := []string{"handler", "main", "index", "app"}
	for _, name := range commonNames {
		for path := range files {
			base := strings.TrimSuffix(path, "/")
			if idx := strings.LastIndex(base, "/"); idx >= 0 {
				base = base[idx+1:]
			}
			base = strings.TrimSuffix(base, "."+getExtension(base))
			if base == name {
				return path
			}
		}
	}

	// Last resort: return first file
	for path := range files {
		return path
	}

	return "handler"
}

func getExtension(filename string) string {
	if idx := strings.LastIndex(filename, "."); idx >= 0 {
		return filename[idx+1:]
	}
	return ""
}

// computeFilesHash computes a combined hash of all files
func computeFilesHash(files map[string][]byte) string {
	// Sort paths for deterministic hash
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	// Simple sort
	for i := range paths {
		for j := i + 1; j < len(paths); j++ {
			if paths[i] > paths[j] {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	// Concatenate all content with paths for hashing
	var combined strings.Builder
	for _, path := range paths {
		combined.WriteString(path)
		combined.WriteByte(0)
		combined.Write(files[path])
		combined.WriteByte(0)
	}

	return crypto.HashString(combined.String())
}
