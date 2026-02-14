// Package tenant defines the abstraction for tenant resource isolation and
// access control enforcement. This centralizes tenant boundary checks so that
// all planes (control, execution, scheduler, event bus, observability) apply
// consistent isolation rules regardless of the underlying storage or execution
// backend.
package tenant

import (
	"context"
	"errors"
)

// Standard sentinel errors for tenant isolation violations.
var (
	// ErrAccessDenied is returned when a principal lacks permission to
	// access a resource in the target tenant/namespace scope.
	ErrAccessDenied = errors.New("tenant: access denied")

	// ErrQuotaExceeded is returned when a tenant operation would exceed
	// the configured quota for the given dimension.
	ErrQuotaExceeded = errors.New("tenant: quota exceeded")

	// ErrNamespaceNotFound is returned when the target namespace does not
	// exist within the tenant.
	ErrNamespaceNotFound = errors.New("tenant: namespace not found")

	// ErrTenantNotFound is returned when the target tenant does not exist.
	ErrTenantNotFound = errors.New("tenant: tenant not found")

	// ErrTenantDisabled is returned when operations are attempted on a
	// disabled or suspended tenant.
	ErrTenantDisabled = errors.New("tenant: tenant disabled")
)

// Scope identifies the tenant and namespace context for an operation.
type Scope struct {
	TenantID  string
	Namespace string
}

// QuotaDimension identifies a resource type subject to quota enforcement.
type QuotaDimension string

const (
	QuotaInvocations    QuotaDimension = "invocations"
	QuotaFunctionsCount QuotaDimension = "functions_count"
	QuotaAsyncQueueDepth QuotaDimension = "async_queue_depth"
	QuotaEventPublishes QuotaDimension = "event_publishes"
	QuotaMemoryMB       QuotaDimension = "memory_mb"
	QuotaVCPUMilli      QuotaDimension = "vcpu_milli"
	QuotaDiskIOPS       QuotaDimension = "disk_iops"
)

// QuotaDecision describes whether a quota check passed or was denied.
type QuotaDecision struct {
	Allowed   bool   `json:"allowed"`
	Dimension string `json:"dimension"`
	Limit     int64  `json:"limit"`
	Used      int64  `json:"used"`
	Requested int64  `json:"requested"`
	Message   string `json:"message,omitempty"`
}

// ResourceDescriptor identifies a specific resource for access control checks.
type ResourceDescriptor struct {
	// Type is the resource category (e.g. "function", "topic", "secret").
	Type string
	// ID is the unique resource identifier within the scope.
	ID string
	// Action is the operation being performed (e.g. "read", "write", "invoke", "delete").
	Action string
}

// Isolator enforces tenant boundaries across all architectural planes.
// Implementations verify that operations respect tenant scope, namespace
// isolation, quota limits, and RBAC policies.
type Isolator interface {
	// ValidateAccess checks whether the current context has permission to
	// access the given resource within the specified scope. Returns
	// ErrAccessDenied if the principal lacks the required permission.
	ValidateAccess(ctx context.Context, scope Scope, resource ResourceDescriptor) error

	// EnforceQuota checks and optionally consumes quota for the given
	// dimension. Returns ErrQuotaExceeded if the operation would exceed
	// the tenant's configured limits.
	EnforceQuota(ctx context.Context, scope Scope, dimension QuotaDimension, amount int64) (*QuotaDecision, error)

	// ScopeContext attaches the tenant scope to the context so that
	// downstream components (store, queue, cache) automatically apply
	// tenant filtering.
	ScopeContext(ctx context.Context, scope Scope) context.Context

	// ExtractScope retrieves the tenant scope from the context.
	// Returns a default scope if none is present.
	ExtractScope(ctx context.Context) Scope

	// ValidateTenant checks that the tenant exists and is in an active
	// state. Returns ErrTenantNotFound or ErrTenantDisabled accordingly.
	ValidateTenant(ctx context.Context, tenantID string) error

	// ValidateNamespace checks that the namespace exists within the tenant.
	// Returns ErrNamespaceNotFound if it does not.
	ValidateNamespace(ctx context.Context, scope Scope) error
}
