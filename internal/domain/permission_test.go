package domain

import "testing"

func TestValidRole(t *testing.T) {
	tests := []struct {
		role Role
		want bool
	}{
		{RoleAdmin, true},
		{RoleOperator, true},
		{RoleInvoker, true},
		{RoleViewer, true},
		{Role("superadmin"), false},
		{Role(""), false},
	}
	for _, tt := range tests {
		if got := ValidRole(tt.role); got != tt.want {
			t.Errorf("ValidRole(%q) = %v, want %v", tt.role, got, tt.want)
		}
	}
}

func TestRolePermissions_AdminHasAllPermissions(t *testing.T) {
	allPerms := []Permission{
		PermFunctionCreate, PermFunctionRead, PermFunctionUpdate, PermFunctionDelete, PermFunctionInvoke,
		PermRuntimeRead, PermRuntimeWrite,
		PermConfigRead, PermConfigWrite,
		PermSnapshotRead, PermSnapshotWrite,
		PermAPIKeyManage, PermSecretManage,
		PermWorkflowManage, PermWorkflowInvoke, PermScheduleManage, PermGatewayManage,
		PermLogRead, PermMetricsRead,
		PermAppPublish, PermAppRead, PermAppInstall, PermAppManage,
		PermRBACManage,
	}

	adminPerms := RolePermissions[RoleAdmin]
	permSet := make(map[Permission]bool, len(adminPerms))
	for _, p := range adminPerms {
		permSet[p] = true
	}

	for _, p := range allPerms {
		if !permSet[p] {
			t.Errorf("admin role missing permission %q", p)
		}
	}
}

func TestRolePermissions_ViewerCannotWrite(t *testing.T) {
	writePerms := []Permission{
		PermFunctionCreate, PermFunctionUpdate, PermFunctionDelete, PermFunctionInvoke,
		PermRuntimeWrite, PermConfigWrite, PermSnapshotWrite,
		PermAPIKeyManage, PermSecretManage,
		PermWorkflowManage, PermWorkflowInvoke, PermScheduleManage, PermGatewayManage,
		PermRBACManage,
	}

	viewerPerms := RolePermissions[RoleViewer]
	permSet := make(map[Permission]bool, len(viewerPerms))
	for _, p := range viewerPerms {
		permSet[p] = true
	}

	for _, p := range writePerms {
		if permSet[p] {
			t.Errorf("viewer role should not have write permission %q", p)
		}
	}
}

func TestRolePermissions_InvokerSubsetOfOperator(t *testing.T) {
	operatorPerms := make(map[Permission]bool)
	for _, p := range RolePermissions[RoleOperator] {
		operatorPerms[p] = true
	}
	for _, p := range RolePermissions[RoleInvoker] {
		if !operatorPerms[p] {
			t.Errorf("invoker permission %q not present in operator role", p)
		}
	}
}

func TestValidScopeType(t *testing.T) {
	tests := []struct {
		scope ScopeType
		want  bool
	}{
		{ScopeTenant, true},
		{ScopeResourceType, true},
		{ScopeResource, true},
		{ScopeType("global"), false},
		{ScopeType(""), false},
	}
	for _, tt := range tests {
		if got := ValidScopeType(tt.scope); got != tt.want {
			t.Errorf("ValidScopeType(%q) = %v, want %v", tt.scope, got, tt.want)
		}
	}
}

func TestValidPrincipalType(t *testing.T) {
	tests := []struct {
		pt   PrincipalType
		want bool
	}{
		{PrincipalUser, true},
		{PrincipalGroup, true},
		{PrincipalServiceAccount, true},
		{PrincipalType("anonymous"), false},
		{PrincipalType(""), false},
	}
	for _, tt := range tests {
		if got := ValidPrincipalType(tt.pt); got != tt.want {
			t.Errorf("ValidPrincipalType(%q) = %v, want %v", tt.pt, got, tt.want)
		}
	}
}
