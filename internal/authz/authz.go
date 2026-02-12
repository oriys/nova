package authz

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// Authorizer checks whether an identity has the required permission
type Authorizer struct {
	defaultRole domain.Role
}

// New creates an Authorizer with the given default role
func New(defaultRole domain.Role) *Authorizer {
	if !domain.ValidRole(defaultRole) {
		defaultRole = domain.RoleViewer
	}
	return &Authorizer{defaultRole: defaultRole}
}

// Check verifies that identity holds perm, optionally scoped to resource.
// DENY policies are evaluated first. Returns nil if allowed, non-nil error otherwise.
func (a *Authorizer) Check(identity *auth.Identity, perm domain.Permission, resource string) error {
	if identity == nil {
		return errForbidden
	}

	policies := identity.Policies
	if len(policies) == 0 {
		// Apply default role
		policies = []domain.PolicyBinding{{Role: a.defaultRole}}
	}

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
			if matchScope(pb.Functions, resource) {
				return errForbidden
			}
		}
	}

	// Phase 2: Check ALLOW policies
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
			if matchScope(pb.Functions, resource) {
				return nil
			}
		}
	}
	return errForbidden
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
}

// resolvePermission determines the required permission for a request.
func resolvePermission(method, path string) domain.Permission {
	// Special case: invoke endpoint
	if method == "POST" && strings.Contains(path, "/invoke") {
		return domain.PermFunctionInvoke
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
			resource := extractFunctionName(r.URL.Path)

			if err := authorizer.Check(identity, perm, resource); err != nil {
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
