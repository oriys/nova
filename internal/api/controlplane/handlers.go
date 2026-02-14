package controlplane

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/layer"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/service"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/volume"
	"github.com/oriys/nova/internal/workflow"
)

// Handler handles control plane HTTP requests (function lifecycle and snapshot management).
type Handler struct {
	Store           *store.Store
	Pool            *pool.Pool
	Backend         backend.Backend
	FCAdapter       *firecracker.Adapter // Optional: for Firecracker-specific features
	Compiler        *compiler.Compiler
	FunctionService *service.FunctionService
	WorkflowService *workflow.Service
	APIKeyManager   *auth.APIKeyManager
	SecretsStore    *secrets.Store
	Scheduler       *scheduler.Scheduler
	RootfsDir       string          // Directory where rootfs ext4 images are stored
	GatewayEnabled  bool            // Whether gateway route management is enabled
	LayerManager    *layer.Manager  // Optional: for shared dependency layers
	VolumeManager   *volume.Manager // Optional: for persistent volume management
	AIService       *ai.Service     // Optional: for AI-powered code operations
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

	// Function versions
	mux.HandleFunc("GET /functions/{name}/versions", h.ListFunctionVersions)
	mux.HandleFunc("GET /functions/{name}/versions/{version}", h.GetFunctionVersion)

	// Runtimes
	mux.HandleFunc("GET /runtimes", h.ListRuntimes)
	mux.HandleFunc("POST /runtimes", h.CreateRuntime)
	mux.HandleFunc("POST /runtimes/upload", h.UploadRuntime)
	mux.HandleFunc("DELETE /runtimes/{id}", h.DeleteRuntime)

	// Backends
	mux.HandleFunc("GET /backends", h.ListBackends)

	// Configuration
	mux.HandleFunc("GET /config", h.GetConfig)
	mux.HandleFunc("PUT /config", h.UpdateConfig)

	// UI notifications
	mux.HandleFunc("GET /notifications", h.ListNotifications)
	mux.HandleFunc("GET /notifications/unread-count", h.GetUnreadNotificationCount)
	mux.HandleFunc("POST /notifications/{id}/read", h.MarkNotificationRead)
	mux.HandleFunc("POST /notifications/read-all", h.MarkAllNotificationsRead)

	// Tenant / namespace management
	mux.HandleFunc("GET /tenants", h.ListTenants)
	mux.HandleFunc("POST /tenants", h.CreateTenant)
	mux.HandleFunc("PATCH /tenants/{tenantID}", h.UpdateTenant)
	mux.HandleFunc("DELETE /tenants/{tenantID}", h.DeleteTenant)
	mux.HandleFunc("GET /tenants/{tenantID}/namespaces", h.ListNamespaces)
	mux.HandleFunc("POST /tenants/{tenantID}/namespaces", h.CreateNamespace)
	mux.HandleFunc("PATCH /tenants/{tenantID}/namespaces/{namespace}", h.UpdateNamespace)
	mux.HandleFunc("DELETE /tenants/{tenantID}/namespaces/{namespace}", h.DeleteNamespace)
	mux.HandleFunc("GET /tenants/{tenantID}/quotas", h.ListTenantQuotas)
	mux.HandleFunc("PUT /tenants/{tenantID}/quotas/{dimension}", h.UpsertTenantQuota)
	mux.HandleFunc("DELETE /tenants/{tenantID}/quotas/{dimension}", h.DeleteTenantQuota)
	mux.HandleFunc("GET /tenants/{tenantID}/usage", h.GetTenantUsage)
	mux.HandleFunc("GET /tenants/{tenantID}/menu-permissions", h.ListTenantMenuPermissions)
	mux.HandleFunc("PUT /tenants/{tenantID}/menu-permissions/{menuKey}", h.UpsertTenantMenuPermission)
	mux.HandleFunc("DELETE /tenants/{tenantID}/menu-permissions/{menuKey}", h.DeleteTenantMenuPermission)
	mux.HandleFunc("GET /tenants/{tenantID}/button-permissions", h.ListTenantButtonPermissions)
	mux.HandleFunc("PUT /tenants/{tenantID}/button-permissions/{permissionKey}", h.UpsertTenantButtonPermission)
	mux.HandleFunc("DELETE /tenants/{tenantID}/button-permissions/{permissionKey}", h.DeleteTenantButtonPermission)

	// Snapshot management
	mux.HandleFunc("GET /snapshots", h.ListSnapshots)
	mux.HandleFunc("POST /functions/{name}/snapshot", h.CreateSnapshot)
	mux.HandleFunc("DELETE /functions/{name}/snapshot", h.DeleteSnapshot)

	// Auto-scaling
	mux.HandleFunc("PUT /functions/{name}/scaling", h.SetScalingPolicy)
	mux.HandleFunc("GET /functions/{name}/scaling", h.GetScalingPolicy)
	mux.HandleFunc("DELETE /functions/{name}/scaling", h.DeleteScalingPolicy)

	// Capacity policy (admission control)
	mux.HandleFunc("PUT /functions/{name}/capacity", h.SetCapacityPolicy)
	mux.HandleFunc("GET /functions/{name}/capacity", h.GetCapacityPolicy)
	mux.HandleFunc("DELETE /functions/{name}/capacity", h.DeleteCapacityPolicy)

	// SLO policy
	mux.HandleFunc("PUT /functions/{name}/slo", h.SetSLOPolicy)
	mux.HandleFunc("GET /functions/{name}/slo", h.GetSLOPolicy)
	mux.HandleFunc("DELETE /functions/{name}/slo", h.DeleteSLOPolicy)

	// Event bus
	mux.HandleFunc("POST /topics", h.CreateEventTopic)
	mux.HandleFunc("GET /topics", h.ListEventTopics)
	mux.HandleFunc("GET /topics/{name}", h.GetEventTopic)
	mux.HandleFunc("DELETE /topics/{name}", h.DeleteEventTopic)
	mux.HandleFunc("POST /topics/{name}/publish", h.PublishEvent)
	mux.HandleFunc("POST /topics/{name}/outbox", h.CreateEventOutbox)
	mux.HandleFunc("GET /topics/{name}/outbox", h.ListEventOutbox)
	mux.HandleFunc("GET /topics/{name}/messages", h.ListTopicMessages)
	mux.HandleFunc("POST /topics/{name}/subscriptions", h.CreateEventSubscription)
	mux.HandleFunc("GET /topics/{name}/subscriptions", h.ListEventSubscriptions)
	mux.HandleFunc("GET /subscriptions/{id}", h.GetEventSubscription)
	mux.HandleFunc("PATCH /subscriptions/{id}", h.UpdateEventSubscription)
	mux.HandleFunc("DELETE /subscriptions/{id}", h.DeleteEventSubscription)
	mux.HandleFunc("GET /subscriptions/{id}/deliveries", h.ListEventDeliveries)
	mux.HandleFunc("POST /subscriptions/{id}/replay", h.ReplayEventSubscription)
	mux.HandleFunc("POST /subscriptions/{id}/seek", h.SeekEventSubscription)
	mux.HandleFunc("GET /deliveries/{id}", h.GetEventDelivery)
	mux.HandleFunc("POST /deliveries/{id}/retry", h.RetryEventDelivery)
	mux.HandleFunc("POST /outbox/{id}/retry", h.RetryEventOutbox)

	// Workflows
	h.RegisterWorkflowRoutes(mux)

	// API Keys
	if h.APIKeyManager != nil {
		akHandler := &APIKeyHandler{Manager: h.APIKeyManager}
		akHandler.RegisterRoutes(mux)
	}

	// Secrets
	if h.SecretsStore != nil {
		secHandler := &SecretHandler{Store: h.SecretsStore}
		secHandler.RegisterRoutes(mux)
	}

	// RBAC management
	rbacHandler := &RBACHandler{Store: h.Store}
	rbacHandler.RegisterRoutes(mux)

	// Schedules
	schedHandler := &ScheduleHandler{Store: h.Store, Scheduler: h.Scheduler}
	schedHandler.RegisterRoutes(mux)

	// Gateway routes
	if h.GatewayEnabled {
		gwHandler := &GatewayHandler{Store: h.Store}
		gwHandler.RegisterRoutes(mux)
	}

	// Layers
	mux.HandleFunc("POST /layers", h.CreateLayer)
	mux.HandleFunc("GET /layers", h.ListLayers)
	mux.HandleFunc("GET /layers/{name}", h.GetLayer)
	mux.HandleFunc("DELETE /layers/{name}", h.DeleteLayer)
	mux.HandleFunc("PUT /functions/{name}/layers", h.SetFunctionLayers)
	mux.HandleFunc("GET /functions/{name}/layers", h.GetFunctionLayers)

	// Volumes
	mux.HandleFunc("POST /volumes", h.CreateVolume)
	mux.HandleFunc("GET /volumes", h.ListVolumes)
	mux.HandleFunc("GET /volumes/{name}", h.GetVolume)
	mux.HandleFunc("DELETE /volumes/{name}", h.DeleteVolume)

	// AI-powered code operations
	if h.AIService != nil {
		aiHandler := &AIHandler{Service: h.AIService, Store: h.Store}
		aiHandler.RegisterRoutes(mux)
	}

	// API Documentation handler (requires AI service)
	if h.AIService != nil {
		apiDocHandler := &APIDocHandler{AIService: h.AIService, Store: h.Store}
		apiDocHandler.RegisterRoutes(mux)
	}

	// Test Suite handler (CRUD always available, AI generation requires AI service)
	tsHandler := &TestSuiteHandler{AIService: h.AIService, Store: h.Store}
	tsHandler.RegisterRoutes(mux)

	// Database Access (DbResource / DbBinding / CredentialPolicy / Audit Logs)
	mux.HandleFunc("POST /db-resources", h.CreateDbResource)
	mux.HandleFunc("GET /db-resources", h.ListDbResources)
	mux.HandleFunc("GET /db-resources/{name}", h.GetDbResource)
	mux.HandleFunc("PATCH /db-resources/{name}", h.UpdateDbResource)
	mux.HandleFunc("DELETE /db-resources/{name}", h.DeleteDbResource)
	mux.HandleFunc("POST /db-resources/{name}/bindings", h.CreateDbBinding)
	mux.HandleFunc("GET /db-resources/{name}/bindings", h.ListDbBindings)
	mux.HandleFunc("DELETE /db-bindings/{id}", h.DeleteDbBinding)
	mux.HandleFunc("PUT /db-resources/{name}/credential-policy", h.SetCredentialPolicy)
	mux.HandleFunc("GET /db-resources/{name}/credential-policy", h.GetCredentialPolicy)
	mux.HandleFunc("DELETE /db-resources/{name}/credential-policy", h.DeleteCredentialPolicy)
	mux.HandleFunc("GET /db-resources/{name}/logs", h.ListDbRequestLogs)
}

// ListFunctionVersions returns all versions of a function.
func (h *Handler) ListFunctionVersions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	versions, err := h.Store.ListVersions(r.Context(), fn.ID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if versions == nil {
		versions = []*domain.FunctionVersion{}
	}
	total := estimatePaginatedTotal(limit, offset, len(versions))
	writePaginatedList(w, limit, offset, len(versions), total, versions)
}

// GetFunctionVersion returns a specific version of a function.
func (h *Handler) GetFunctionVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	versionStr := r.PathValue("version")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	v, err := strconv.Atoi(versionStr)
	if err != nil {
		http.Error(w, "invalid version number", http.StatusBadRequest)
		return
	}

	version, err := h.Store.GetVersion(r.Context(), fn.ID, v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(version)
}
