package authz

import (
	"testing"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/domain"
)

func TestCheck_WorkflowScopeBinding(t *testing.T) {
	az := New(domain.RoleViewer)

	tests := []struct {
		name     string
		identity *auth.Identity
		perm     domain.Permission
		resource string
		wantErr  bool
	}{
		{
			name: "invoker with empty workflows allows all workflows",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker},
				},
			},
			perm:     domain.PermWorkflowInvoke,
			resource: "my-workflow",
			wantErr:  false,
		},
		{
			name: "invoker with specific workflow binding allows matching",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker, Workflows: []string{"my-workflow"}},
				},
			},
			perm:     domain.PermWorkflowInvoke,
			resource: "my-workflow",
			wantErr:  false,
		},
		{
			name: "invoker with specific workflow binding denies non-matching",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker, Workflows: []string{"other-workflow"}},
				},
			},
			perm:     domain.PermWorkflowInvoke,
			resource: "my-workflow",
			wantErr:  true,
		},
		{
			name: "invoker with glob workflow pattern allows matching",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker, Workflows: []string{"staging-*"}},
				},
			},
			perm:     domain.PermWorkflowInvoke,
			resource: "staging-pipeline",
			wantErr:  false,
		},
		{
			name: "invoker with specific function binding does not restrict workflow",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker, Functions: []string{"my-func"}},
				},
			},
			perm:     domain.PermWorkflowInvoke,
			resource: "any-workflow",
			wantErr:  false,
		},
		{
			name: "invoker with function binding restricts function invoke",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker, Functions: []string{"allowed-func"}},
				},
			},
			perm:     domain.PermFunctionInvoke,
			resource: "other-func",
			wantErr:  true,
		},
		{
			name: "invoker with function binding allows matching function",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker, Functions: []string{"allowed-func"}},
				},
			},
			perm:     domain.PermFunctionInvoke,
			resource: "allowed-func",
			wantErr:  false,
		},
		{
			name: "deny workflow policy takes precedence",
			identity: &auth.Identity{
				Subject: "apikey:test",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleInvoker, Effect: domain.EffectDeny, Workflows: []string{"secret-wf"}},
					{Role: domain.RoleInvoker},
				},
			},
			perm:     domain.PermWorkflowInvoke,
			resource: "secret-wf",
			wantErr:  true,
		},
		{
			name: "admin bypasses all workflow checks",
			identity: &auth.Identity{
				Subject: "apikey:admin",
				Policies: []domain.PolicyBinding{
					{Role: domain.RoleAdmin},
				},
			},
			perm:     domain.PermWorkflowInvoke,
			resource: "any-workflow",
			wantErr:  false,
		},
		{
			name: "nil identity denied",
			identity: nil,
			perm:     domain.PermWorkflowInvoke,
			resource: "wf",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := az.Check(tt.identity, tt.perm, tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolvePermission_WorkflowInvoke(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   domain.Permission
	}{
		{"POST", "/workflows/my-wf/invoke-async", domain.PermWorkflowInvoke},
		{"POST", "/workflows/my-wf/runs", domain.PermWorkflowInvoke},
		{"POST", "/functions/my-fn/invoke", domain.PermFunctionInvoke},
		{"GET", "/workflows", domain.PermWorkflowManage},
		{"POST", "/workflows", domain.PermWorkflowManage},
		{"DELETE", "/workflows/my-wf", domain.PermWorkflowManage},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := resolvePermission(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("resolvePermission(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractWorkflowName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/workflows/my-wf/invoke-async", "my-wf"},
		{"/workflows/my-wf/runs", "my-wf"},
		{"/workflows/my-wf", "my-wf"},
		{"/functions/my-fn/invoke", ""},
		{"/workflows", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractWorkflowName(tt.path)
			if got != tt.want {
				t.Errorf("extractWorkflowName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractResource(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/functions/my-fn/invoke", "my-fn"},
		{"/workflows/my-wf/runs", "my-wf"},
		{"/metrics", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractResource(tt.path)
			if got != tt.want {
				t.Errorf("extractResource(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
