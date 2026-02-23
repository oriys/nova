package authz

import (
	"sort"

	"github.com/oriys/nova/internal/domain"
)

// RouteBinding describes an API route guarded by a permission.
type RouteBinding struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// PermissionBindingInfo maps a single permission code to its API routes
// and the UI button keys it controls.
type PermissionBindingInfo struct {
	Permission  string         `json:"permission"`
	Description string         `json:"description"`
	APIRoutes   []RouteBinding `json:"api_routes"`
	UIButtons   []string       `json:"ui_buttons"`
}

// buttonPermissionMap maps a permission code to the button keys it controls.
// Derived from store.AllButtonPermissionKeys – the button key IS the
// permission code, so the mapping is 1:1 for keys that appear in that list.
var buttonPermissionMap = map[string][]string{
	"function:create":  {"function:create"},
	"function:update":  {"function:update"},
	"function:delete":  {"function:delete"},
	"function:invoke":  {"function:invoke"},
	"runtime:write":    {"runtime:write"},
	"config:write":     {"config:write"},
	"secret:manage":    {"secret:manage"},
	"apikey:manage":    {"apikey:manage"},
	"workflow:manage":  {"workflow:manage"},
	"schedule:manage":  {"schedule:manage"},
	"gateway:manage":   {"gateway:manage"},
	"rbac:manage":      {"rbac:manage"},
}

// permissionDescriptions provides a human-readable label for each permission.
var permissionDescriptions = map[string]string{
	"function:create":  "Create functions",
	"function:read":    "View functions",
	"function:update":  "Update functions",
	"function:delete":  "Delete functions",
	"function:invoke":  "Invoke functions",
	"runtime:read":     "View runtimes",
	"runtime:write":    "Manage runtimes",
	"config:read":      "View platform configuration",
	"config:write":     "Modify platform configuration",
	"snapshot:read":    "View snapshots",
	"snapshot:write":   "Manage snapshots",
	"apikey:manage":    "Manage API keys",
	"secret:manage":    "Manage secrets",
	"workflow:manage":  "Manage workflows",
	"workflow:invoke":  "Invoke workflows",
	"schedule:manage":  "Manage schedules",
	"gateway:manage":   "Manage gateway routes",
	"log:read":         "View logs",
	"metrics:read":     "View metrics",
	"app:publish":      "Publish applications",
	"app:read":         "View applications",
	"app:install":      "Install applications",
	"app:manage":       "Manage applications",
	"rbac:manage":      "Manage RBAC roles and assignments",
}

// BuildPermissionBindings generates the full list of permission bindings by
// scanning the routeTable and buttonPermissionMap.
func BuildPermissionBindings() []PermissionBindingInfo {
	// Collect routes per permission from routeTable.
	routesByPerm := make(map[domain.Permission][]RouteBinding)
	for _, rp := range routeTable {
		routesByPerm[rp.permission] = append(routesByPerm[rp.permission], RouteBinding{
			Method: rp.method,
			Path:   rp.prefix,
		})
	}

	// Add special-case routes that resolvePermission handles outside routeTable.
	specialRoutes := []struct {
		perm  domain.Permission
		route RouteBinding
	}{
		{domain.PermFunctionInvoke, RouteBinding{"POST", "/functions/{name}/invoke"}},
		{domain.PermWorkflowInvoke, RouteBinding{"POST", "/workflows/{name}/invoke"}},
		{domain.PermWorkflowInvoke, RouteBinding{"POST", "/workflows/{name}/runs"}},
		{domain.PermSnapshotWrite, RouteBinding{"POST", "/functions/{name}/snapshot"}},
		{domain.PermSnapshotWrite, RouteBinding{"DELETE", "/functions/{name}/snapshot"}},
		{domain.PermSnapshotRead, RouteBinding{"GET", "/functions/{name}/snapshot"}},
		{domain.PermLogRead, RouteBinding{"GET", "/functions/{name}/logs"}},
		{domain.PermMetricsRead, RouteBinding{"GET", "/functions/{name}/metrics"}},
		{domain.PermConfigRead, RouteBinding{"GET", "/ai/config"}},
		{domain.PermConfigWrite, RouteBinding{"PUT", "/ai/config"}},
		{domain.PermConfigRead, RouteBinding{"GET", "/ai/prompts"}},
		{domain.PermConfigWrite, RouteBinding{"POST", "/ai/prompts"}},
	}
	for _, sr := range specialRoutes {
		routesByPerm[sr.perm] = append(routesByPerm[sr.perm], sr.route)
	}

	// Build combined set of all permission codes.
	allPerms := make(map[string]bool)
	for perm := range routesByPerm {
		allPerms[string(perm)] = true
	}
	for perm := range buttonPermissionMap {
		allPerms[perm] = true
	}
	for perm := range permissionDescriptions {
		allPerms[perm] = true
	}

	// Build sorted output.
	codes := make([]string, 0, len(allPerms))
	for c := range allPerms {
		codes = append(codes, c)
	}
	sort.Strings(codes)

	bindings := make([]PermissionBindingInfo, 0, len(codes))
	for _, code := range codes {
		info := PermissionBindingInfo{
			Permission:  code,
			Description: permissionDescriptions[code],
			APIRoutes:   routesByPerm[domain.Permission(code)],
			UIButtons:   buttonPermissionMap[code],
		}
		if info.APIRoutes == nil {
			info.APIRoutes = []RouteBinding{}
		}
		if info.UIButtons == nil {
			info.UIButtons = []string{}
		}
		bindings = append(bindings, info)
	}
	return bindings
}
