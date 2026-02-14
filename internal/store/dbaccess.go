package store

import (
	"time"

	"github.com/oriys/nova/internal/domain"
)

// DbResourceRecord is the persistence representation of a database resource.
type DbResourceRecord struct {
	ID            string                  `json:"id"`
	TenantID      string                  `json:"tenant_id,omitempty"`
	Name          string                  `json:"name"`
	Type          domain.DbResourceType   `json:"type"`
	Endpoint      string                  `json:"endpoint"`
	Port          int                     `json:"port,omitempty"`
	DatabaseName  string                  `json:"database_name,omitempty"`
	Region        string                  `json:"region,omitempty"`
	TenantMode    domain.TenantMode       `json:"tenant_mode"`
	NetworkPolicy string                  `json:"network_policy,omitempty"`
	Capabilities  *domain.DbCapabilities  `json:"capabilities,omitempty"`
	CreatedAt     time.Time               `json:"created_at"`
	UpdatedAt     time.Time               `json:"updated_at"`
}

// DbResourceUpdate contains optional fields for updating a database resource.
type DbResourceUpdate struct {
	Name          *string                 `json:"name,omitempty"`
	Endpoint      *string                 `json:"endpoint,omitempty"`
	Port          *int                    `json:"port,omitempty"`
	DatabaseName  *string                 `json:"database_name,omitempty"`
	Region        *string                 `json:"region,omitempty"`
	TenantMode    *domain.TenantMode      `json:"tenant_mode,omitempty"`
	NetworkPolicy *string                 `json:"network_policy,omitempty"`
	Capabilities  *domain.DbCapabilities  `json:"capabilities,omitempty"`
}

// DbBindingRecord is the persistence representation of a database binding.
type DbBindingRecord struct {
	ID              string                  `json:"id"`
	TenantID        string                  `json:"tenant_id,omitempty"`
	FunctionID      string                  `json:"function_id"`
	VersionSelector string                  `json:"version_selector,omitempty"`
	DbResourceID    string                  `json:"db_resource_id"`
	Permissions     []domain.DbPermission   `json:"permissions"`
	Quota           *domain.DbBindingQuota  `json:"quota,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

// DbBindingUpdate contains optional fields for updating a database binding.
type DbBindingUpdate struct {
	VersionSelector *string                 `json:"version_selector,omitempty"`
	Permissions     []domain.DbPermission   `json:"permissions,omitempty"`
	Quota           *domain.DbBindingQuota  `json:"quota,omitempty"`
}

// CredentialPolicyRecord is the persistence representation of a credential policy.
type CredentialPolicyRecord struct {
	ID             string                     `json:"id"`
	DbResourceID   string                     `json:"db_resource_id"`
	AuthMode       domain.CredentialAuthMode   `json:"auth_mode"`
	RotationDays   int                         `json:"rotation_days,omitempty"`
	StaticUsername string                       `json:"static_username,omitempty"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
}

// CredentialPolicyUpdate contains optional fields for updating a credential policy.
type CredentialPolicyUpdate struct {
	AuthMode       *domain.CredentialAuthMode `json:"auth_mode,omitempty"`
	RotationDays   *int                       `json:"rotation_days,omitempty"`
	StaticUsername *string                     `json:"static_username,omitempty"`
}
