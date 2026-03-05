package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// PermissionResolver looks up the effective permission codes for a given
// tenant + subject from the database.  Satisfied by *store.Store.
type PermissionResolver interface {
	ResolveEffectivePermissions(ctx context.Context, tenantID, subject string) ([]string, error)
}

// Authorizer checks whether an identity has the required permission
type Authorizer struct {
	defaultRole domain.Role
	resolver    PermissionResolver // optional; when set, DB RBAC is consulted
}

// New creates an Authorizer with the given default role
func New(defaultRole domain.Role) *Authorizer {
	if !domain.ValidRole(defaultRole) {
		defaultRole = domain.RoleViewer
	}
	return &Authorizer{defaultRole: defaultRole}
}

// WithResolver sets an optional PermissionResolver for DB-backed RBAC.
func (a *Authorizer) WithResolver(r PermissionResolver) *Authorizer {
	a.resolver = r
	return a
}

// Check verifies that identity holds perm, optionally scoped to resource.
// DENY policies are evaluated first. Returns nil if allowed, non-nil error otherwise.
func (a *Authorizer) Check(identity *auth.Identity, perm domain.Permission, resource string) error {
	if identity == nil {
		return errForbidden
	}

	policies := identity.Policies
	if len(policies) == 0 {
		// No policies: deny by default (secure by default)
		return errForbidden
	}

	isWorkflowPerm := isWorkflowPermission(perm)

	// Phase 1: Check DENY policies first
	for _, pb := range policies {
		if pb.Effect != domain.EffectDeny {
			continue
		}
		perms, ok := domain.RolePermissions[pb.Role]
		if !ok {
			continue
		}
		for _, p := range perms {
			if p != perm {
				continue
			}
			if isWorkflowPerm {
				if matchScope(pb.Workflows, resource) {
					return errForbidden
				}
			} else {
				if matchScope(pb.Functions, resource) {
					return errForbidden
				}
			}
		}
	}

	// Phase 2: Check ALLOW policies (hardcoded role map)
	for _, pb := range policies {
		if pb.Effect == domain.EffectDeny {
			continue // already processed
		}
		if pb.Role == domain.RoleAdmin {
			return nil // admin bypasses all checks
		}
		perms, ok := domain.RolePermissions[pb.Role]
		if !ok {
			continue
		}
		for _, p := range perms {
			if p != perm {
				continue
			}
			if isWorkflowPerm {
				if matchScope(pb.Workflows, resource) {
					return nil
				}
			} else {
				if matchScope(pb.Functions, resource) {
					return nil
				}
			}
		}
	}
	return errForbidden
}

// CheckWithContext is like Check but also consults DB-stored RBAC role
// assignments when a PermissionResolver is configured.
func (a *Authorizer) CheckWithContext(ctx context.Context, identity *auth.Identity, perm domain.Permission, resource string) error {
	// First try the in-memory policy check.
	if err := a.Check(identity, perm, resource); err == nil {
		return nil
	}

	// Fall through to DB RBAC if a resolver is available.
	if a.resolver == nil || identity == nil {
		return errForbidden
	}

	scope := store.TenantScopeFromContext(ctx)
	if scope.TenantID == "" {
		return errForbidden
	}

	codes, err := a.resolver.ResolveEffectivePermissions(ctx, scope.TenantID, identity.Subject)
	if err != nil {
		logging.Op().Warn("failed to resolve DB RBAC permissions",
			"error", err,
			"subject", identity.Subject,
			"tenant_id", scope.TenantID,
		)
		return errForbidden
	}

	permStr := string(perm)
	for _, code := range codes {
		if code == permStr {
			return nil
		}
	}
	return errForbidden
}

// isWorkflowPermission returns true if the permission is workflow-specific.
func isWorkflowPermission(perm domain.Permission) bool {
	return perm == domain.PermWorkflowInvoke || perm == domain.PermWorkflowManage
}

// matchScope checks whether the resource matches a set of function scope patterns.
// Supports glob patterns (e.g. "staging-*", "team-?-*") via path.Match.
func matchScope(functions []string, resource string) bool {
	if len(functions) == 0 {
		return true // no scope restriction
	}
	if resource == "" {
		return true // non-function-scoped permission
	}
	for _, pattern := range functions {
		if pattern == resource {
			return true
		}
		if matched, _ := path.Match(pattern, resource); matched {
			return true
		}
	}
	return false
}

var errForbidden = &forbiddenError{}

type forbiddenError struct{}

func (e *forbiddenError) Error() string { return "forbidden: insufficient permissions" }

// routePermission maps an HTTP method + path pattern to a required permission.
type routePermission struct {
	method     string
	prefix     string
	permission domain.Permission
}

var routeTable = []routePermission{
	// Functions
	{"POST", "/functions", domain.PermFunctionCreate},
	{"GET", "/functions", domain.PermFunctionRead},
	{"PATCH", "/functions/", domain.PermFunctionUpdate},
	{"DELETE", "/functions/", domain.PermFunctionDelete},

	// Function invoke
	{"POST", "/functions/", domain.PermFunctionInvoke}, // /functions/{name}/invoke

	// Function code
	{"GET", "/functions/", domain.PermFunctionRead},
	{"PUT", "/functions/", domain.PermFunctionUpdate},

	// Runtimes
	{"GET", "/runtimes", domain.PermRuntimeRead},
	{"POST", "/runtimes", domain.PermRuntimeWrite},
	{"DELETE", "/runtimes/", domain.PermRuntimeWrite},

	// Config
	{"GET", "/config", domain.PermConfigRead},
	{"PUT", "/config", domain.PermConfigWrite},

	// Snapshots
	{"GET", "/snapshots", domain.PermSnapshotRead},
	{"POST", "/functions/", domain.PermSnapshotWrite}, // /functions/{name}/snapshot
	{"DELETE", "/functions/", domain.PermSnapshotWrite},

	// API Keys
	{"POST", "/apikeys", domain.PermAPIKeyManage},
	{"GET", "/apikeys", domain.PermAPIKeyManage},
	{"DELETE", "/apikeys/", domain.PermAPIKeyManage},
	{"PATCH", "/apikeys/", domain.PermAPIKeyManage},

	// Secrets
	{"POST", "/secrets", domain.PermSecretManage},
	{"GET", "/secrets", domain.PermSecretManage},
	{"DELETE", "/secrets/", domain.PermSecretManage},

	// Workflows
	{"POST", "/workflows", domain.PermWorkflowManage},
	{"GET", "/workflows", domain.PermWorkflowManage},
	{"PATCH", "/workflows/", domain.PermWorkflowManage},
	{"DELETE", "/workflows/", domain.PermWorkflowManage},

	// Schedules
	{"POST", "/schedules", domain.PermScheduleManage},
	{"GET", "/schedules", domain.PermScheduleManage},
	{"PATCH", "/schedules/", domain.PermScheduleManage},
	{"DELETE", "/schedules/", domain.PermScheduleManage},

	// Gateway
	{"POST", "/gateway/", domain.PermGatewayManage},
	{"GET", "/gateway/", domain.PermGatewayManage},
	{"PATCH", "/gateway/", domain.PermGatewayManage},
	{"DELETE", "/gateway/", domain.PermGatewayManage},

	// Logs & Metrics
	{"GET", "/metrics", domain.PermMetricsRead},
	{"GET", "/notifications", domain.PermMetricsRead},
	{"POST", "/notifications", domain.PermMetricsRead},

	// Tenants / Namespaces / Quotas
	{"GET", "/tenants", domain.PermConfigRead},
	{"POST", "/tenants", domain.PermConfigWrite},
	{"PATCH", "/tenants/", domain.PermConfigWrite},
	{"PUT", "/tenants/", domain.PermConfigWrite},
	{"DELETE", "/tenants/", domain.PermConfigWrite},

	// RBAC management
	{"POST", "/rbac/", domain.PermRBACManage},
	{"GET", "/rbac/", domain.PermRBACManage},
	{"DELETE", "/rbac/", domain.PermRBACManage},

	// Triggers
	{"POST", "/triggers", domain.PermFunctionUpdate},
	{"GET", "/triggers", domain.PermFunctionRead},
	{"PATCH", "/triggers/", domain.PermFunctionUpdate},
	{"DELETE", "/triggers/", domain.PermFunctionDelete},

	// Layers
	{"POST", "/layers", domain.PermConfigWrite},
	{"GET", "/layers", domain.PermConfigRead},
	{"DELETE", "/layers/", domain.PermConfigWrite},

	// Volumes
	{"POST", "/volumes", domain.PermConfigWrite},
	{"GET", "/volumes", domain.PermConfigRead},
	{"DELETE", "/volumes/", domain.PermConfigWrite},

	// Cluster nodes
	{"POST", "/cluster/", domain.PermConfigWrite},
	{"GET", "/cluster/", domain.PermConfigRead},
	{"DELETE", "/cluster/", domain.PermConfigWrite},

	// Backends
	{"GET", "/backends", domain.PermConfigRead},

	// Async invocations
	{"GET", "/async-invocations", domain.PermFunctionRead},
	{"GET", "/async-invocations/", domain.PermFunctionRead},

	// Invocations / stats
	{"GET", "/invocations", domain.PermLogRead},
	{"GET", "/stats", domain.PermMetricsRead},

	// Cost
	{"GET", "/cost/", domain.PermMetricsRead},

	// Database access
	{"POST", "/db-resources", domain.PermConfigWrite},
	{"GET", "/db-resources", domain.PermConfigRead},
	{"PATCH", "/db-resources/", domain.PermConfigWrite},
	{"DELETE", "/db-resources/", domain.PermConfigWrite},
	{"PUT", "/db-resources/", domain.PermConfigWrite},
	{"DELETE", "/db-bindings/", domain.PermConfigWrite},
}

// resolvePermission determines the required permission for a request.
func resolvePermission(method, path string) domain.Permission {
	// Special case: invoke endpoint
	if method == "POST" && strings.Contains(path, "/invoke") {
		if strings.HasPrefix(path, "/workflows/") {
			return domain.PermWorkflowInvoke
		}
		return domain.PermFunctionInvoke
	}
	// Special case: workflow runs
	if method == "POST" && strings.HasPrefix(path, "/workflows/") && strings.HasSuffix(path, "/runs") {
		return domain.PermWorkflowInvoke
	}
	// Special case: AI configuration/prompts
	if strings.HasPrefix(path, "/ai/config") || strings.HasPrefix(path, "/ai/prompts") {
		if method == "GET" || method == "HEAD" {
			return domain.PermConfigRead
		}
		return domain.PermConfigWrite
	}
	// Special case: snapshot endpoints
	if strings.Contains(path, "/snapshot") {
		if method == "POST" || method == "DELETE" {
			return domain.PermSnapshotWrite
		}
		return domain.PermSnapshotRead
	}
	// Special case: logs
	if strings.Contains(path, "/logs") {
		return domain.PermLogRead
	}
	// Special case: metrics per function
	if strings.Contains(path, "/metrics") {
		return domain.PermMetricsRead
	}
	// Special case: UI notifications
	if strings.Contains(path, "/notifications") {
		return domain.PermMetricsRead
	}

	// Special case: RBAC read-only endpoints accessible to any authenticated user.
	// /rbac/my-permissions and /rbac/permission-bindings require only basic read.
	if method == "GET" && (path == "/rbac/my-permissions" || path == "/rbac/permission-bindings") {
		return domain.PermFunctionRead
	}

	for _, rp := range routeTable {
		if rp.method != method {
			continue
		}
		if strings.HasSuffix(rp.prefix, "/") {
			if strings.HasPrefix(path, rp.prefix) {
				return rp.permission
			}
		} else {
			if path == rp.prefix || strings.HasPrefix(path, rp.prefix+"?") {
				return rp.permission
			}
		}
	}

	// Default: read for GET, write for everything else
	if method == "GET" || method == "HEAD" {
		return domain.PermFunctionRead
	}
	return domain.PermFunctionCreate
}

// extractFunctionName extracts the function name from URL path if applicable.
func extractFunctionName(path string) string {
	// /functions/{name}... -> name
	const prefix = "/functions/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

// extractWorkflowName extracts the workflow name from URL path if applicable.
func extractWorkflowName(path string) string {
	// /workflows/{name}... -> name
	const prefix = "/workflows/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

// extractResource extracts the resource name (function or workflow) from the URL path.
func extractResource(path string) string {
	if name := extractFunctionName(path); name != "" {
		return name
	}
	return extractWorkflowName(path)
}

// Middleware returns an HTTP middleware that enforces authorization.
func Middleware(authorizer *Authorizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := auth.GetIdentity(r.Context())
			if identity == nil {
				// No identity means auth middleware already passed (public path)
				next.ServeHTTP(w, r)
				return
			}

			perm := resolvePermission(r.Method, r.URL.Path)
			resource := extractResource(r.URL.Path)

			if err := authorizer.CheckWithContext(r.Context(), identity, perm, resource); err != nil {
				scope := store.TenantScopeFromContext(r.Context())
				logging.Op().Warn("authorization denied",
					"subject", identity.Subject,
					"permission", perm,
					"resource", resource,
					"path", r.URL.Path,
					"method", r.Method,
					"tenant_id", scope.TenantID,
					"namespace", scope.Namespace,
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "forbidden",
					"message": "insufficient permissions for this operation",
				})
				return
			}

			scope := store.TenantScopeFromContext(r.Context())
			logging.Op().Debug("authorization granted",
				"subject", identity.Subject,
				"permission", perm,
				"resource", resource,
				"path", r.URL.Path,
				"method", r.Method,
				"tenant_id", scope.TenantID,
				"namespace", scope.Namespace,
			)
			next.ServeHTTP(w, r)
		})
	}
}
