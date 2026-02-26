package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
)

// ActivateFunctionVersion handles POST /functions/{name}/versions/{version}/activate
// It rolls back the function to the specified version's configuration and code.
func (h *Handler) ActivateFunctionVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	versionStr := r.PathValue("version")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found", http.StatusNotFound)
		return
	}

	v, err := strconv.Atoi(versionStr)
	if err != nil {
		http.Error(w, "invalid version number", http.StatusBadRequest)
		return
	}

	ver, err := h.Store.GetVersion(r.Context(), fn.ID, v)
	if err != nil {
		http.Error(w, fmt.Sprintf("version %d not found", v), http.StatusNotFound)
		return
	}

	// Apply version config to the live function
	fn.Handler = ver.Handler
	fn.MemoryMB = ver.MemoryMB
	fn.TimeoutS = ver.TimeoutS
	fn.Mode = ver.Mode
	fn.Limits = ver.Limits
	fn.EnvVars = ver.EnvVars
	fn.CodeHash = ver.CodeHash
	fn.Version = ver.Version

	if err := h.Store.SaveFunction(r.Context(), fn); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Restore code if the version has it
	if ver.Code != "" {
		if err := h.Store.UpdateFunctionCode(r.Context(), fn.ID, ver.Code, ver.CodeHash); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Hot reload or evict VMs
		if !domain.NeedsCompilation(fn.Runtime) {
			reloadFiles := map[string][]byte{"handler": []byte(ver.Code)}
			if err := h.Pool.ReloadCodeWithHash(fn.ID, reloadFiles, ver.CodeHash); err == nil {
				logging.Op().Info("hot reloaded version code", "function", fn.Name, "version", v)
			} else {
				h.Pool.Evict(fn.ID)
			}
		} else {
			h.Pool.Evict(fn.ID)
			if h.Compiler != nil {
				h.Compiler.CompileAsync(r.Context(), fn, ver.Code)
			}
		}
		executor.InvalidateSnapshot(h.Backend.SnapshotDir(), fn.ID)
		h.Pool.InvalidateSnapshotCache(fn.ID)
	} else {
		// Config-only change, evict to pick up new settings
		h.Pool.Evict(fn.ID)
	}

	logging.Op().Info("activated version", "function", fn.Name, "version", v)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "activated",
		"version": v,
	})
}

// VersionDiff describes a single changed field between two versions.
type VersionDiff struct {
	Field string      `json:"field"`
	From  interface{} `json:"from"`
	To    interface{} `json:"to"`
}

// CompareFunctionVersions handles GET /functions/{name}/versions/{v1}/diff/{v2}
func (h *Handler) CompareFunctionVersions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	v1Str := r.PathValue("v1")
	v2Str := r.PathValue("v2")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found", http.StatusNotFound)
		return
	}

	v1, err := strconv.Atoi(v1Str)
	if err != nil {
		http.Error(w, "invalid version v1", http.StatusBadRequest)
		return
	}
	v2, err := strconv.Atoi(v2Str)
	if err != nil {
		http.Error(w, "invalid version v2", http.StatusBadRequest)
		return
	}

	ver1, err := h.Store.GetVersion(r.Context(), fn.ID, v1)
	if err != nil {
		http.Error(w, fmt.Sprintf("version %d not found", v1), http.StatusNotFound)
		return
	}
	ver2, err := h.Store.GetVersion(r.Context(), fn.ID, v2)
	if err != nil {
		http.Error(w, fmt.Sprintf("version %d not found", v2), http.StatusNotFound)
		return
	}

	diffs := diffVersions(ver1, ver2)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"function": name,
		"v1":       v1,
		"v2":       v2,
		"changes":  diffs,
	})
}

// diffVersions computes a field-by-field diff between two FunctionVersions.
func diffVersions(a, b *domain.FunctionVersion) []VersionDiff {
	var diffs []VersionDiff

	add := func(field string, from, to interface{}) {
		diffs = append(diffs, VersionDiff{Field: field, From: from, To: to})
	}

	if a.Handler != b.Handler {
		add("handler", a.Handler, b.Handler)
	}
	if a.MemoryMB != b.MemoryMB {
		add("memory_mb", a.MemoryMB, b.MemoryMB)
	}
	if a.TimeoutS != b.TimeoutS {
		add("timeout_s", a.TimeoutS, b.TimeoutS)
	}
	if a.Mode != b.Mode {
		add("mode", a.Mode, b.Mode)
	}
	if a.CodeHash != b.CodeHash {
		add("code_hash", a.CodeHash, b.CodeHash)
	}
	if a.Code != b.Code {
		add("code", a.Code, b.Code)
	}
	if !reflect.DeepEqual(a.Limits, b.Limits) {
		add("limits", a.Limits, b.Limits)
	}
	if !reflect.DeepEqual(a.EnvVars, b.EnvVars) {
		add("env_vars", a.EnvVars, b.EnvVars)
	}

	return diffs
}
