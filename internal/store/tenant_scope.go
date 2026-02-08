package store

import (
	"context"
	"regexp"
	"strings"
)

const (
	DefaultTenantID  = "default"
	DefaultNamespace = "default"
)

var tenantScopePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// TenantScope describes the tenant boundary for metadata operations.
type TenantScope struct {
	TenantID  string
	Namespace string
}

type tenantScopeContextKey struct{}

var tenantScopeKey = tenantScopeContextKey{}

// WithTenantScope attaches tenant scope to context. Invalid/empty values fallback to defaults.
func WithTenantScope(ctx context.Context, tenantID, namespace string) context.Context {
	tenantID, namespace = normalizeTenantScope(tenantID, namespace)
	return context.WithValue(ctx, tenantScopeKey, TenantScope{TenantID: tenantID, Namespace: namespace})
}

// TenantScopeFromContext returns scope from context or defaults.
func TenantScopeFromContext(ctx context.Context) TenantScope {
	if ctx != nil {
		if scope, ok := ctx.Value(tenantScopeKey).(TenantScope); ok {
			tenantID, namespace := normalizeTenantScope(scope.TenantID, scope.Namespace)
			return TenantScope{TenantID: tenantID, Namespace: namespace}
		}
	}
	return TenantScope{TenantID: DefaultTenantID, Namespace: DefaultNamespace}
}

func tenantScopeFromContext(ctx context.Context) TenantScope {
	return TenantScopeFromContext(ctx)
}

// IsValidTenantScopePart checks whether a tenant or namespace identifier is valid.
func IsValidTenantScopePart(value string) bool {
	return tenantScopePattern.MatchString(strings.TrimSpace(value))
}

func normalizeTenantScope(tenantID, namespace string) (string, string) {
	tenantID = strings.TrimSpace(tenantID)
	namespace = strings.TrimSpace(namespace)

	if tenantID == "" || !tenantScopePattern.MatchString(tenantID) {
		tenantID = DefaultTenantID
	}
	if namespace == "" || !tenantScopePattern.MatchString(namespace) {
		namespace = DefaultNamespace
	}
	return tenantID, namespace
}
