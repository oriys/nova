package domain

import (
	"encoding/json"
	"time"
)

// BundleType represents the type of package
type BundleType string

const (
	BundleTypeFunction BundleType = "function_bundle" // Only contains functions
	BundleTypeWorkflow BundleType = "workflow_bundle" // Contains workflow + required functions
)

// AppVisibility controls who can see an app
type AppVisibility string

const (
	VisibilityPublic  AppVisibility = "public"  // Anyone can see and install
	VisibilityPrivate AppVisibility = "private" // Only owner/org members can see
)

// ReleaseStatus represents the publishing state of a release
type ReleaseStatus string

const (
	ReleaseStatusDraft     ReleaseStatus = "draft"     // Not yet published
	ReleaseStatusPublished ReleaseStatus = "published" // Available for installation
	ReleaseStatusYanked    ReleaseStatus = "yanked"    // Deprecated, cannot install new
)

// InstallationStatus represents the state of an installation
type InstallationStatus string

const (
	InstallStatusPending   InstallationStatus = "pending"   // Installation queued
	InstallStatusPlanning  InstallationStatus = "planning"  // Analyzing dependencies
	InstallStatusApplying  InstallationStatus = "applying"  // Creating resources
	InstallStatusValidate  InstallationStatus = "validating" // Verifying installation
	InstallStatusSucceeded InstallationStatus = "succeeded" // Installation complete
	InstallStatusFailed    InstallationStatus = "failed"    // Installation failed
	InstallStatusUpgrading InstallationStatus = "upgrading" // Upgrade in progress
	InstallStatusDeleting  InstallationStatus = "deleting"  // Uninstall in progress
)

// JobOperation represents the type of marketplace operation
type JobOperation string

const (
	JobOperationInstall   JobOperation = "install"
	JobOperationUpgrade   JobOperation = "upgrade"
	JobOperationUninstall JobOperation = "uninstall"
)

// ManagedMode defines how an installed resource is managed
type ManagedMode string

const (
	ManagedModeExclusive ManagedMode = "exclusive" // Only this installation owns it
	ManagedModeShared    ManagedMode = "shared"    // Shared with other installations
)

// App represents a marketplace application/package
type App struct {
	ID          string        `json:"id"`
	Slug        string        `json:"slug"`                   // URL-friendly identifier (e.g., "my-app")
	Owner       string        `json:"owner"`                  // User/org that published it
	Visibility  AppVisibility `json:"visibility"`             // public or private
	Title       string        `json:"title"`                  // Display name
	Summary     string        `json:"summary,omitempty"`      // Short description
	Description string        `json:"description,omitempty"`  // Long description (markdown)
	Tags        []string      `json:"tags,omitempty"`         // Search tags
	IconURL     string        `json:"icon_url,omitempty"`     // Icon image URL
	SourceURL   string        `json:"source_url,omitempty"`   // Source code repository
	HomepageURL string        `json:"homepage_url,omitempty"` // Documentation site
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// AppRelease represents a specific version of an app
type AppRelease struct {
	ID              string          `json:"id"`
	AppID           string          `json:"app_id"`
	Version         string          `json:"version"`                     // SemVer (e.g., "1.2.3")
	ManifestJSON    json.RawMessage `json:"manifest_json"`               // Bundle manifest
	ArtifactURI     string          `json:"artifact_uri"`                // S3/local path to bundle archive
	ArtifactDigest  string          `json:"artifact_digest"`             // SHA256 of artifact
	Signature       string          `json:"signature,omitempty"`         // Optional cosign signature
	Status          ReleaseStatus   `json:"status"`                      // draft, published, yanked
	Changelog       string          `json:"changelog,omitempty"`         // Release notes
	RequiresVersion string          `json:"requires_version,omitempty"`  // Minimum Nova version
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// BundleManifest represents the metadata file in a bundle
type BundleManifest struct {
	Name           string                 `json:"name" yaml:"name"`
	Version        string                 `json:"version" yaml:"version"` // SemVer
	Type           BundleType             `json:"type" yaml:"type"`
	Description    string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Functions      []FunctionSpec         `json:"functions" yaml:"functions"`                   // All functions in bundle
	Workflow       *WorkflowSpec          `json:"workflow,omitempty" yaml:"workflow,omitempty"` // For workflow_bundle only
	Parameters     []InstallParameter     `json:"parameters,omitempty" yaml:"parameters,omitempty"` // User-configurable values
	Dependencies   []BundleDependency     `json:"dependencies,omitempty" yaml:"dependencies,omitempty"` // External bundles
	RequiresNovaVersion string            `json:"requires_nova_version,omitempty" yaml:"requires_nova_version,omitempty"`
}

// FunctionSpec defines a function within a bundle
type FunctionSpec struct {
	Key         string            `json:"key" yaml:"key"`                     // Internal reference key (e.g., "validator", "processor")
	Name        string            `json:"name,omitempty" yaml:"name,omitempty"` // Optional default name (can be overridden at install)
	Runtime     Runtime           `json:"runtime" yaml:"runtime"`
	Handler     string            `json:"handler" yaml:"handler"`
	Files       []string          `json:"files" yaml:"files"`                 // Relative paths in bundle (e.g., "functions/validator/*")
	MemoryMB    int               `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
	TimeoutS    int               `json:"timeout_s,omitempty" yaml:"timeout_s,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty" yaml:"env_vars,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
}

// WorkflowSpec defines a workflow within a bundle
type WorkflowSpec struct {
	Name        string               `json:"name,omitempty" yaml:"name,omitempty"` // Optional default name
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Definition  WorkflowBundleDAG    `json:"definition" yaml:"definition"`
}

// WorkflowBundleDAG is the DAG definition that uses function_ref instead of function_name
type WorkflowBundleDAG struct {
	Nodes []BundleNodeDefinition `json:"nodes" yaml:"nodes"`
	Edges []EdgeDefinition       `json:"edges" yaml:"edges"` // Reuse from workflow.go
}

// BundleNodeDefinition is like NodeDefinition but uses FunctionRef
type BundleNodeDefinition struct {
	NodeKey      string          `json:"node_key" yaml:"node_key"`
	FunctionRef  string          `json:"function_ref" yaml:"function_ref"`   // References FunctionSpec.Key in bundle
	InputMapping json.RawMessage `json:"input_mapping,omitempty" yaml:"input_mapping,omitempty"`
	RetryPolicy  *RetryPolicy    `json:"retry_policy,omitempty" yaml:"retry_policy,omitempty"`
	TimeoutS     int             `json:"timeout_s,omitempty" yaml:"timeout_s,omitempty"`
}

// InstallParameter defines a user-configurable parameter
type InstallParameter struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Type        string `json:"type" yaml:"type"`                             // "string", "int", "bool", "secret"
	Default     string `json:"default,omitempty" yaml:"default,omitempty"`   // Default value
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"` // Must be provided at install
}

// BundleDependency represents a dependency on another bundle
type BundleDependency struct {
	Name    string `json:"name" yaml:"name"`       // App slug
	Version string `json:"version" yaml:"version"` // SemVer constraint (e.g., "^1.0.0")
}

// Installation represents an installed app in a tenant/namespace
type Installation struct {
	ID          string             `json:"id"`
	TenantID    string             `json:"tenant_id"`
	Namespace   string             `json:"namespace"`
	AppID       string             `json:"app_id"`
	ReleaseID   string             `json:"release_id"`
	InstallName string             `json:"install_name"`          // User-chosen name for this installation
	Status      InstallationStatus `json:"status"`
	ValuesJSON  json.RawMessage    `json:"values_json,omitempty"` // User-provided parameter values
	CreatedBy   string             `json:"created_by"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// InstallationResource tracks a resource created by an installation
type InstallationResource struct {
	ID             string      `json:"id"`
	InstallationID string      `json:"installation_id"`
	ResourceType   string      `json:"resource_type"`   // "function", "workflow"
	ResourceName   string      `json:"resource_name"`   // Full name in tenant/namespace
	ResourceID     string      `json:"resource_id"`     // UUID of the resource
	ContentDigest  string      `json:"content_digest"`  // SHA256 of resource content
	ManagedMode    ManagedMode `json:"managed_mode"`    // exclusive or shared
	CreatedAt      time.Time   `json:"created_at"`
}

// InstallJob tracks async install/upgrade/uninstall operations
type InstallJob struct {
	ID             string             `json:"id"`
	InstallationID string             `json:"installation_id"`
	Operation      JobOperation       `json:"operation"`           // install, upgrade, uninstall
	Status         InstallationStatus `json:"status"`              // pending, applying, succeeded, failed
	Step           string             `json:"step,omitempty"`      // Current step description
	Error          string             `json:"error,omitempty"`     // Error message if failed
	StartedAt      time.Time          `json:"started_at"`
	FinishedAt     *time.Time         `json:"finished_at,omitempty"`
}

// InstallPlan represents the result of a dry-run installation
type InstallPlan struct {
	Valid           bool                `json:"valid"`
	Conflicts       []ResourceConflict  `json:"conflicts,omitempty"`       // Resources that already exist
	ToCreate        []PlannedResource   `json:"to_create"`                 // Resources to be created
	QuotaCheck      QuotaCheckResult    `json:"quota_check"`
	MissingRuntimes []string            `json:"missing_runtimes,omitempty"` // Runtimes not available
	Errors          []string            `json:"errors,omitempty"`           // Validation errors
}

// ResourceConflict indicates a naming conflict
type ResourceConflict struct {
	ResourceType string `json:"resource_type"` // "function", "workflow"
	ResourceName string `json:"resource_name"`
	ExistingID   string `json:"existing_id"`
	Reason       string `json:"reason,omitempty"`
}

// PlannedResource represents a resource that will be created
type PlannedResource struct {
	ResourceType string `json:"resource_type"` // "function", "workflow"
	ResourceName string `json:"resource_name"`
	Runtime      string `json:"runtime,omitempty"`
	Description  string `json:"description,omitempty"`
}

// QuotaCheckResult summarizes quota impact
type QuotaCheckResult struct {
	OK             bool   `json:"ok"`
	FunctionsUsed  int    `json:"functions_used"`
	FunctionsLimit int    `json:"functions_limit"`
	Reason         string `json:"reason,omitempty"`
}

// InstallRequest represents an installation request
type InstallRequest struct {
	AppSlug      string                 `json:"app_slug"`
	Version      string                 `json:"version"`             // SemVer
	TenantID     string                 `json:"tenant_id"`
	Namespace    string                 `json:"namespace"`
	InstallName  string                 `json:"install_name"`        // Unique name for this installation
	NamePrefix   string                 `json:"name_prefix,omitempty"` // Optional prefix for generated resource names
	Values       map[string]interface{} `json:"values,omitempty"`    // User-provided parameter values
	DryRun       bool                   `json:"dry_run,omitempty"`   // If true, only return plan
}
