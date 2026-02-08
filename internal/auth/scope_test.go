package auth

import "testing"

func TestExtractTenantScopesFromClaimsAllowedScopes(t *testing.T) {
	claims := map[string]any{
		"allowed_scopes": []any{
			map[string]any{"tenant_id": "team-a", "namespace": "prod"},
			"team-b/*",
			"team-a/prod",
		},
	}

	scopes := extractTenantScopesFromClaims(claims)
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(scopes))
	}

	if scopes[0].TenantID != "team-a" || scopes[0].Namespace != "prod" {
		t.Fatalf("unexpected scope[0]: %+v", scopes[0])
	}
	if scopes[1].TenantID != "team-b" || scopes[1].Namespace != "*" {
		t.Fatalf("unexpected scope[1]: %+v", scopes[1])
	}
}

func TestExtractTenantScopesFromClaimsAllowedTenantsNamespaces(t *testing.T) {
	claims := map[string]any{
		"allowed_tenants":    []any{"team-a", "team-b"},
		"allowed_namespaces": []any{"dev", "prod"},
	}

	scopes := extractTenantScopesFromClaims(claims)
	if len(scopes) != 4 {
		t.Fatalf("expected 4 scopes, got %d", len(scopes))
	}

	expected := map[string]struct{}{
		"team-a/dev":  {},
		"team-a/prod": {},
		"team-b/dev":  {},
		"team-b/prod": {},
	}

	for _, scope := range scopes {
		key := scope.TenantID + "/" + scope.Namespace
		if _, ok := expected[key]; !ok {
			t.Fatalf("unexpected scope: %s", key)
		}
	}
}

func TestExtractTenantScopesFromClaimsFallbackTenantOnly(t *testing.T) {
	claims := map[string]any{
		"tenant_id": "team-a",
	}

	scopes := extractTenantScopesFromClaims(claims)
	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(scopes))
	}
	if scopes[0].TenantID != "team-a" || scopes[0].Namespace != "*" {
		t.Fatalf("unexpected scope: %+v", scopes[0])
	}
}

func TestIdentityAllowsScope(t *testing.T) {
	identity := &Identity{
		Subject: "user:test",
		AllowedScopes: []TenantScope{
			{TenantID: "team-a", Namespace: "*"},
			{TenantID: "team-b", Namespace: "prod"},
		},
	}

	if !identity.AllowsScope("team-a", "dev") {
		t.Fatalf("expected team-a/dev to be allowed")
	}
	if !identity.AllowsScope("team-b", "prod") {
		t.Fatalf("expected team-b/prod to be allowed")
	}
	if identity.AllowsScope("team-b", "dev") {
		t.Fatalf("expected team-b/dev to be denied")
	}
	if identity.AllowsScope("team-c", "prod") {
		t.Fatalf("expected team-c/prod to be denied")
	}
}
