package gateway

import (
	"testing"

	"github.com/oriys/nova/internal/domain"
)

func TestPolicyAllowsRouteTarget(t *testing.T) {
	t.Run("empty policies allow when explicit scope not required", func(t *testing.T) {
		if !policyAllowsRouteTarget(nil, "hello", "", false) {
			t.Fatal("expected unrestricted API key to be allowed")
		}
	})

	t.Run("empty policies deny when explicit scope required", func(t *testing.T) {
		if policyAllowsRouteTarget(nil, "hello", "", true) {
			t.Fatal("expected unrestricted API key to be denied")
		}
	})

	t.Run("allow binding grants explicit access", func(t *testing.T) {
		policies := []domain.PolicyBinding{{
			Role:      domain.RoleInvoker,
			Functions: []string{"hello"},
		}}
		if !policyAllowsRouteTarget(policies, "hello", "", true) {
			t.Fatal("expected matching allow binding to pass")
		}
	})

	t.Run("deny binding overrides allow", func(t *testing.T) {
		policies := []domain.PolicyBinding{
			{
				Role:      domain.RoleInvoker,
				Functions: []string{"hello"},
			},
			{
				Role:      domain.RoleInvoker,
				Functions: []string{"hello"},
				Effect:    domain.EffectDeny,
			},
		}
		if policyAllowsRouteTarget(policies, "hello", "", true) {
			t.Fatal("expected deny binding to override allow")
		}
	})
}

func TestRouteRequiresExplicitAPIKeyScope(t *testing.T) {
	if !routeRequiresExplicitAPIKeyScope(&domain.GatewayRoute{
		AuthConfig: map[string]string{"require_explicit_scope": "true"},
	}) {
		t.Fatal("expected require_explicit_scope=true to enable strict mode")
	}

	if routeRequiresExplicitAPIKeyScope(&domain.GatewayRoute{
		AuthConfig: map[string]string{"require_explicit_scope": "false"},
	}) {
		t.Fatal("expected require_explicit_scope=false to disable strict mode")
	}
}
