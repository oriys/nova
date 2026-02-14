package domain

import "testing"

func TestIsValidDbResourceType(t *testing.T) {
	tests := []struct {
		t    DbResourceType
		want bool
	}{
		{DbResourcePostgres, true},
		{DbResourceMySQL, true},
		{DbResourceRedis, true},
		{DbResourceDynamo, true},
		{DbResourceHTTP, true},
		{DbResourceType("mssql"), false},
		{DbResourceType(""), false},
	}
	for _, tt := range tests {
		if got := IsValidDbResourceType(tt.t); got != tt.want {
			t.Errorf("IsValidDbResourceType(%q) = %v, want %v", tt.t, got, tt.want)
		}
	}
}

func TestIsValidTenantMode(t *testing.T) {
	tests := []struct {
		m    TenantMode
		want bool
	}{
		{TenantModeDBPerTenant, true},
		{TenantModeSchemaPerTenant, true},
		{TenantModeSharedRLS, true},
		{TenantMode("other"), false},
		{TenantMode(""), false},
	}
	for _, tt := range tests {
		if got := IsValidTenantMode(tt.m); got != tt.want {
			t.Errorf("IsValidTenantMode(%q) = %v, want %v", tt.m, got, tt.want)
		}
	}
}

func TestIsValidDbPermission(t *testing.T) {
	tests := []struct {
		p    DbPermission
		want bool
	}{
		{DbPermRead, true},
		{DbPermWrite, true},
		{DbPermAdmin, true},
		{DbPermission("superadmin"), false},
		{DbPermission(""), false},
	}
	for _, tt := range tests {
		if got := IsValidDbPermission(tt.p); got != tt.want {
			t.Errorf("IsValidDbPermission(%q) = %v, want %v", tt.p, got, tt.want)
		}
	}
}

func TestIsValidCredentialAuthMode(t *testing.T) {
	tests := []struct {
		m    CredentialAuthMode
		want bool
	}{
		{CredentialAuthStatic, true},
		{CredentialAuthIAM, true},
		{CredentialAuthTokenExchange, true},
		{CredentialAuthMode("oauth"), false},
		{CredentialAuthMode(""), false},
	}
	for _, tt := range tests {
		if got := IsValidCredentialAuthMode(tt.m); got != tt.want {
			t.Errorf("IsValidCredentialAuthMode(%q) = %v, want %v", tt.m, got, tt.want)
		}
	}
}
