package domain

import "time"

// DbResourceType defines the type of database resource.
type DbResourceType string

const (
	DbResourcePostgres DbResourceType = "postgres"
	DbResourceMySQL    DbResourceType = "mysql"
	DbResourceRedis    DbResourceType = "redis"
	DbResourceDynamo   DbResourceType = "dynamo"
	DbResourceHTTP     DbResourceType = "http"
)

// IsValidDbResourceType returns true if the type is recognized.
func IsValidDbResourceType(t DbResourceType) bool {
	switch t {
	case DbResourcePostgres, DbResourceMySQL, DbResourceRedis, DbResourceDynamo, DbResourceHTTP:
		return true
	}
	return false
}

// TenantMode defines the tenant isolation strategy for a database resource.
type TenantMode string

const (
	TenantModeDBPerTenant     TenantMode = "db_per_tenant"
	TenantModeSchemaPerTenant TenantMode = "schema_per_tenant"
	TenantModeSharedRLS       TenantMode = "shared_rls"
)

// IsValidTenantMode returns true if the mode is recognized.
func IsValidTenantMode(m TenantMode) bool {
	switch m {
	case TenantModeDBPerTenant, TenantModeSchemaPerTenant, TenantModeSharedRLS:
		return true
	}
	return false
}

// DbCapabilities describes optional features supported by a database resource.
type DbCapabilities struct {
	SupportsRLS     bool `json:"supports_rls,omitempty"`
	SupportsIAMAuth bool `json:"supports_iam_auth,omitempty"`
}

// DbResource represents a registered database resource in the control plane.
type DbResource struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id,omitempty"`
	Name         string         `json:"name"`
	Type         DbResourceType `json:"type"`
	Endpoint     string         `json:"endpoint"`
	Port         int            `json:"port,omitempty"`
	DatabaseName string         `json:"database_name,omitempty"`
	Region       string         `json:"region,omitempty"`
	TenantMode   TenantMode     `json:"tenant_mode"`

	NetworkPolicy string          `json:"network_policy,omitempty"` // "vpc", "public", "private_link"
	Capabilities  *DbCapabilities `json:"capabilities,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DbPermission enumerates the allowed access levels for a binding.
type DbPermission string

const (
	DbPermRead  DbPermission = "read"
	DbPermWrite DbPermission = "write"
	DbPermAdmin DbPermission = "admin"
)

// IsValidDbPermission returns true if the permission is recognized.
func IsValidDbPermission(p DbPermission) bool {
	switch p {
	case DbPermRead, DbPermWrite, DbPermAdmin:
		return true
	}
	return false
}

// DbBindingQuota defines rate/concurrency limits for a binding.
type DbBindingQuota struct {
	MaxQPS            int `json:"max_qps,omitempty"`
	MaxSessions       int `json:"max_sessions,omitempty"`
	MaxTxConcurrency  int `json:"max_tx_concurrency,omitempty"`
	MaxOpenConnsToDb  int `json:"max_open_conns_to_db,omitempty"`
}

// DbBinding associates a function (or version selector) with a database resource
// and specifies the permissions and quotas for that association.
type DbBinding struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id,omitempty"`
	FunctionID      string          `json:"function_id"`
	VersionSelector string          `json:"version_selector,omitempty"` // e.g. "*", ">=2", "3"
	DbResourceID    string          `json:"db_resource_id"`
	Permissions     []DbPermission  `json:"permissions"`
	Quota           *DbBindingQuota `json:"quota,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CredentialAuthMode defines how credentials are obtained.
type CredentialAuthMode string

const (
	CredentialAuthStatic        CredentialAuthMode = "static"
	CredentialAuthIAM           CredentialAuthMode = "iam"
	CredentialAuthTokenExchange CredentialAuthMode = "token_exchange"
)

// IsValidCredentialAuthMode returns true if the mode is recognized.
func IsValidCredentialAuthMode(m CredentialAuthMode) bool {
	switch m {
	case CredentialAuthStatic, CredentialAuthIAM, CredentialAuthTokenExchange:
		return true
	}
	return false
}

// CredentialPolicy describes how database credentials are managed for a resource.
type CredentialPolicy struct {
	ID             string             `json:"id"`
	DbResourceID   string             `json:"db_resource_id"`
	AuthMode       CredentialAuthMode `json:"auth_mode"`
	RotationDays   int                `json:"rotation_days,omitempty"` // 0 = no auto-rotation
	StaticUsername string             `json:"static_username,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DbRequestLog records an auditable database access event.
type DbRequestLog struct {
	ID           string `json:"id"`
	RequestID    string `json:"request_id"`
	FunctionID   string `json:"function_id"`
	FunctionName string `json:"function_name,omitempty"`
	Version      int    `json:"version,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
	DbResourceID string `json:"db_resource_id"`

	StatementHash string `json:"statement_hash,omitempty"`
	Tables        string `json:"tables,omitempty"` // comma-separated table names
	RowsReturned  int64  `json:"rows_returned,omitempty"`
	RowsAffected  int64  `json:"rows_affected,omitempty"`

	LatencyMs int64  `json:"latency_ms"`
	ErrorCode string `json:"error_code,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
