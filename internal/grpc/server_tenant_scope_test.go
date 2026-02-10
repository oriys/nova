package grpc

import (
	"context"
	"testing"

	"github.com/oriys/nova/internal/store"
	"google.golang.org/grpc/metadata"
)

func TestApplyTenantScopeFromMetadataDefaultsWithoutMetadata(t *testing.T) {
	ctx := applyTenantScopeFromMetadata(context.Background())
	scope := store.TenantScopeFromContext(ctx)

	if scope.TenantID != store.DefaultTenantID || scope.Namespace != store.DefaultNamespace {
		t.Fatalf("expected default scope, got %+v", scope)
	}
}

func TestApplyTenantScopeFromMetadataUsesIncomingScope(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-nova-tenant", "team-a",
		"x-nova-namespace", "prod",
	))
	scope := store.TenantScopeFromContext(applyTenantScopeFromMetadata(ctx))

	if scope.TenantID != "team-a" || scope.Namespace != "prod" {
		t.Fatalf("expected tenant scope team-a/prod, got %+v", scope)
	}
}

func TestApplyTenantScopeFromMetadataPreservesExistingNamespaceWhenMissing(t *testing.T) {
	baseCtx := store.WithTenantScope(context.Background(), "team-a", "dev")
	ctx := metadata.NewIncomingContext(baseCtx, metadata.Pairs("x-nova-tenant", "team-b"))
	scope := store.TenantScopeFromContext(applyTenantScopeFromMetadata(ctx))

	if scope.TenantID != "team-b" || scope.Namespace != "dev" {
		t.Fatalf("expected tenant scope team-b/dev, got %+v", scope)
	}
}
