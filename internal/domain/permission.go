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
	PermScheduleManage  Permission = "schedule:manage"
	PermGatewayManage   Permission = "gateway:manage"
	PermLogRead         Permission = "log:read"
	PermMetricsRead     Permission = "metrics:read"
	PermAppPublish      Permission = "app:publish"
	PermAppRead         Permission = "app:read"
	PermAppInstall      Permission = "app:install"
	PermAppManage       Permission = "app:manage"
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

// PolicyBinding binds a role to an optional set of function scopes
type PolicyBinding struct {
	Role      Role     `json:"role"`
	Functions []string `json:"functions,omitempty"` // empty = all functions; supports glob patterns (e.g. "staging-*")
	Effect    Effect   `json:"effect,omitempty"`    // "allow" (default) or "deny"
}

// RolePermissions maps each role to its granted permissions
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermFunctionCreate, PermFunctionRead, PermFunctionUpdate, PermFunctionDelete, PermFunctionInvoke,
		PermRuntimeRead, PermRuntimeWrite,
		PermConfigRead, PermConfigWrite,
		PermSnapshotRead, PermSnapshotWrite,
		PermAPIKeyManage, PermSecretManage,
		PermWorkflowManage, PermScheduleManage, PermGatewayManage,
		PermLogRead, PermMetricsRead,
		PermAppPublish, PermAppRead, PermAppInstall, PermAppManage,
	},
	RoleOperator: {
		PermFunctionCreate, PermFunctionRead, PermFunctionUpdate, PermFunctionDelete, PermFunctionInvoke,
		PermRuntimeRead, PermRuntimeWrite,
		PermSnapshotRead, PermSnapshotWrite,
		PermWorkflowManage, PermScheduleManage,
		PermLogRead, PermMetricsRead,
		PermAppRead, PermAppInstall,
	},
	RoleInvoker: {
		PermFunctionRead, PermFunctionInvoke,
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
