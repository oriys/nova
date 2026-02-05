package controlplane

import (
	"net/http"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/service"
	"github.com/oriys/nova/internal/store"
)

// Handler handles control plane HTTP requests (function lifecycle and snapshot management).
type Handler struct {
	Store           *store.Store
	Pool            *pool.Pool
	Backend         backend.Backend
	FCAdapter       *firecracker.Adapter // Optional: for Firecracker-specific features
	Compiler        *compiler.Compiler
	FunctionService *service.FunctionService
	RootfsDir       string // Directory where rootfs ext4 images are stored
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
	mux.HandleFunc("GET /functions/{name}/files", h.ListFunctionFiles)

	// Runtimes
	mux.HandleFunc("GET /runtimes", h.ListRuntimes)
	mux.HandleFunc("POST /runtimes", h.CreateRuntime)
	mux.HandleFunc("POST /runtimes/upload", h.UploadRuntime)
	mux.HandleFunc("DELETE /runtimes/{id}", h.DeleteRuntime)

	// Configuration
	mux.HandleFunc("GET /config", h.GetConfig)
	mux.HandleFunc("PUT /config", h.UpdateConfig)

	// Snapshot management
	mux.HandleFunc("GET /snapshots", h.ListSnapshots)
	mux.HandleFunc("POST /functions/{name}/snapshot", h.CreateSnapshot)
	mux.HandleFunc("DELETE /functions/{name}/snapshot", h.DeleteSnapshot)
}