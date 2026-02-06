package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oriys/nova/internal/executor"
)

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

	if snapshots == nil {
		snapshots = []snapshotInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshots)
}

// CreateSnapshot handles POST /functions/{name}/snapshot
func (h *Handler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
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

	if executor.HasSnapshot(h.Backend.SnapshotDir(), fn.ID) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "exists",
			"message": "Snapshot already exists for this function",
		})
		return
	}

	// Fetch code content from store
	codeRecord, err := h.Store.GetFunctionCode(r.Context(), fn.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("get function code: %v", err), http.StatusInternalServerError)
		return
	}
	if codeRecord == nil {
		http.Error(w, "function code not found", http.StatusNotFound)
		return
	}

	// Use compiled binary if available, otherwise use source code
	var codeContent []byte
	if len(codeRecord.CompiledBinary) > 0 {
		codeContent = codeRecord.CompiledBinary
	} else {
		codeContent = []byte(codeRecord.SourceCode)
	}

	pvm, err := h.Pool.Acquire(r.Context(), fn, codeContent)
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

	h.Pool.EvictVM(fn.ID, pvm)

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
