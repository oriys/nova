package domain

// Permission represents a specific action that can be performed
type Permission string

const (
	PermFunctionCreate  Permission = "function:create"
	PermFunctionRead    Permission = "function:read"
	PermFunctionUpdate  Permission = "function:update"
	PermFunctionDelete  Permission = "function:delete"
	PermFunctionInvoke  Permission = "function:invoke"
	PermRuntimeRead     Permission = "runtime:read"
	PermRuntimeWrite    Permission = "runtime:write"
	PermConfigRead      Permission = "config:read"
	PermConfigWrite     Permission = "config:write"
	PermSnapshotRead    Permission = "snapshot:read"
	PermSnapshotWrite   Permission = "snapshot:write"
	PermAPIKeyManage    Permission = "apikey:manage"
	PermSecretManage    Permission = "secret:manage"
	PermWorkflowManage  Permission = "workflow:manage"
	PermWorkflowInvoke  Permission = "workflow:invoke"
	PermScheduleManage  Permission = "schedule:manage"
	PermGatewayManage   Permission = "gateway:manage"
	PermLogRead         Permission = "log:read"
	PermMetricsRead     Permission = "metrics:read"
	PermAppPublish      Permission = "app:publish"
	PermAppRead         Permission = "app:read"
	PermAppInstall      Permission = "app:install"
	PermAppManage       Permission = "app:manage"
	PermRBACManage      Permission = "rbac:manage"
)

// Role represents a named set of permissions
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleInvoker  Role = "invoker"
	RoleViewer   Role = "viewer"
)

// Effect determines whether a policy binding allows or denies access
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// PolicyBinding binds a role to an optional set of function and workflow scopes
type PolicyBinding struct {
	Role      Role     `json:"role"`
	Functions []string `json:"functions,omitempty"`  // empty = all functions; supports glob patterns (e.g. "staging-*")
	Workflows []string `json:"workflows,omitempty"`  // empty = all workflows; supports glob patterns
	Effect    Effect   `json:"effect,omitempty"`     // "allow" (default) or "deny"
}

// RolePermissions maps each role to its granted permissions
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermFunctionCreate, PermFunctionRead, PermFunctionUpdate, PermFunctionDelete, PermFunctionInvoke,
		PermRuntimeRead, PermRuntimeWrite,
		PermConfigRead, PermConfigWrite,
		PermSnapshotRead, PermSnapshotWrite,
		PermAPIKeyManage, PermSecretManage,
		PermWorkflowManage, PermWorkflowInvoke, PermScheduleManage, PermGatewayManage,
		PermLogRead, PermMetricsRead,
		PermAppPublish, PermAppRead, PermAppInstall, PermAppManage,
		PermRBACManage,
	},
	RoleOperator: {
		PermFunctionCreate, PermFunctionRead, PermFunctionUpdate, PermFunctionDelete, PermFunctionInvoke,
		PermRuntimeRead, PermRuntimeWrite,
		PermSnapshotRead, PermSnapshotWrite,
		PermWorkflowManage, PermWorkflowInvoke, PermScheduleManage,
		PermLogRead, PermMetricsRead,
		PermAppRead, PermAppInstall,
	},
	RoleInvoker: {
		PermFunctionRead, PermFunctionInvoke,
		PermWorkflowInvoke,
		PermLogRead,
		PermAppRead,
	},
	RoleViewer: {
		PermFunctionRead,
		PermRuntimeRead,
		PermLogRead, PermMetricsRead,
		PermAppRead,
	},
}

// ValidRole returns true if the role is a known predefined role
func ValidRole(r Role) bool {
	_, ok := RolePermissions[r]
	return ok
}

// ─── RBAC Scope & Principal Types ───────────────────────────────────────────

// ScopeType defines the level at which a role assignment applies.
type ScopeType string

const (
	ScopeTenant       ScopeType = "tenant"
	ScopeResourceType ScopeType = "resource_type"
	ScopeResource     ScopeType = "resource"
)

// ValidScopeType returns true if the scope type is known.
func ValidScopeType(s ScopeType) bool {
	switch s {
	case ScopeTenant, ScopeResourceType, ScopeResource:
		return true
	}
	return false
}

// PrincipalType identifies the kind of entity receiving a role assignment.
type PrincipalType string

const (
	PrincipalUser           PrincipalType = "user"
	PrincipalGroup          PrincipalType = "group"
	PrincipalServiceAccount PrincipalType = "service_account"
)

// ValidPrincipalType returns true if the principal type is known.
func ValidPrincipalType(p PrincipalType) bool {
	switch p {
	case PrincipalUser, PrincipalGroup, PrincipalServiceAccount:
		return true
	}
	return false
}
