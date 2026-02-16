package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// FunctionUpdate contains optional fields for updating a function.
type FunctionUpdate struct {
	Handler             *string                 `json:"handler,omitempty"`
	Code                *string                 `json:"code,omitempty"` // inline code update
	MemoryMB            *int                    `json:"memory_mb,omitempty"`
	TimeoutS            *int                    `json:"timeout_s,omitempty"`
	MinReplicas         *int                    `json:"min_replicas,omitempty"`
	MaxReplicas         *int                    `json:"max_replicas,omitempty"`
	InstanceConcurrency *int                    `json:"instance_concurrency,omitempty"`
	Mode                *domain.ExecutionMode   `json:"mode,omitempty"`
	Backend             *domain.BackendType     `json:"backend,omitempty"`
	Limits              *domain.ResourceLimits  `json:"limits,omitempty"`
	NetworkPolicy       *domain.NetworkPolicy   `json:"network_policy,omitempty"`
	RolloutPolicy       *domain.RolloutPolicy   `json:"rollout_policy,omitempty"`
	AutoScalePolicy     *domain.AutoScalePolicy `json:"auto_scale_policy,omitempty"`
	CapacityPolicy      *domain.CapacityPolicy  `json:"capacity_policy,omitempty"`
	SLOPolicy           *domain.SLOPolicy       `json:"slo_policy,omitempty"`
	EnvVars             map[string]string       `json:"env_vars,omitempty"`
	MergeEnvVars        bool                    `json:"merge_env_vars,omitempty"`
	Layers              []string                `json:"layers,omitempty"`  // layer IDs (max 6)
	Mounts              []domain.VolumeMount    `json:"mounts,omitempty"` // persistent volume mounts
}

// MetadataStore is the durable metadata store (functions, versions, aliases).
type MetadataStore interface {
	Close() error
	Ping(ctx context.Context) error

	SaveFunction(ctx context.Context, fn *domain.Function) error
	GetFunction(ctx context.Context, id string) (*domain.Function, error)
	GetFunctionByName(ctx context.Context, name string) (*domain.Function, error)
	DeleteFunction(ctx context.Context, id string) error
	ListFunctions(ctx context.Context, limit, offset int) ([]*domain.Function, error)
	SearchFunctions(ctx context.Context, query string, limit, offset int) ([]*domain.Function, error)
	UpdateFunction(ctx context.Context, name string, update *FunctionUpdate) (*domain.Function, error)

	// Tenancy (tenant / namespace)
	ListTenants(ctx context.Context, limit, offset int) ([]*TenantRecord, error)
	GetTenant(ctx context.Context, id string) (*TenantRecord, error)
	CreateTenant(ctx context.Context, tenant *TenantRecord) (*TenantRecord, error)
	UpdateTenant(ctx context.Context, id string, update *TenantUpdate) (*TenantRecord, error)
	DeleteTenant(ctx context.Context, id string) error

	ListNamespaces(ctx context.Context, tenantID string, limit, offset int) ([]*NamespaceRecord, error)
	GetNamespace(ctx context.Context, tenantID, name string) (*NamespaceRecord, error)
	CreateNamespace(ctx context.Context, namespace *NamespaceRecord) (*NamespaceRecord, error)
	UpdateNamespace(ctx context.Context, tenantID, name string, update *NamespaceUpdate) (*NamespaceRecord, error)
	DeleteNamespace(ctx context.Context, tenantID, name string) error

	// Tenant governance (quota + usage)
	ListTenantQuotas(ctx context.Context, tenantID string) ([]*TenantQuotaRecord, error)
	UpsertTenantQuota(ctx context.Context, quota *TenantQuotaRecord) (*TenantQuotaRecord, error)
	DeleteTenantQuota(ctx context.Context, tenantID, dimension string) error
	ListTenantUsage(ctx context.Context, tenantID string) ([]*TenantUsageRecord, error)
	RefreshTenantUsage(ctx context.Context, tenantID string) ([]*TenantUsageRecord, error)
	CheckAndConsumeTenantQuota(ctx context.Context, tenantID, dimension string, amount int64) (*TenantQuotaDecision, error)
	CheckTenantAbsoluteQuota(ctx context.Context, tenantID, dimension string, value int64) (*TenantQuotaDecision, error)
	GetTenantFunctionCount(ctx context.Context, tenantID string) (int64, error)
	GetTenantAsyncQueueDepth(ctx context.Context, tenantID string) (int64, error)

	PublishVersion(ctx context.Context, funcID string, version *domain.FunctionVersion) error
	GetVersion(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error)
	ListVersions(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionVersion, error)
	DeleteVersion(ctx context.Context, funcID string, version int) error

	SetAlias(ctx context.Context, alias *domain.FunctionAlias) error
	GetAlias(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error)
	ListAliases(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionAlias, error)
	DeleteAlias(ctx context.Context, funcID, aliasName string) error

	// Invocation logs
	SaveInvocationLog(ctx context.Context, log *InvocationLog) error
	SaveInvocationLogs(ctx context.Context, logs []*InvocationLog) error
	ListInvocationLogs(ctx context.Context, functionID string, limit, offset int) ([]*InvocationLog, error)
	ListAllInvocationLogs(ctx context.Context, limit, offset int) ([]*InvocationLog, error)
	GetInvocationLog(ctx context.Context, requestID string) (*InvocationLog, error)
	GetFunctionTimeSeries(ctx context.Context, functionID string, rangeSeconds, bucketSeconds int) ([]TimeSeriesBucket, error)
	GetGlobalTimeSeries(ctx context.Context, rangeSeconds, bucketSeconds int) ([]TimeSeriesBucket, error)
	GetFunctionDailyHeatmap(ctx context.Context, functionID string, weeks int) ([]DailyCount, error)
	GetGlobalDailyHeatmap(ctx context.Context, weeks int) ([]DailyCount, error)
	GetFunctionSLOSnapshot(ctx context.Context, functionID string, windowSeconds int) (*FunctionSLOSnapshot, error)

	// Async invocations (queue + retries + DLQ)
	EnqueueAsyncInvocation(ctx context.Context, inv *AsyncInvocation) error
	GetAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error)
	ListAsyncInvocations(ctx context.Context, limit, offset int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error)
	ListFunctionAsyncInvocations(ctx context.Context, functionID string, limit, offset int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error)
	AcquireDueAsyncInvocation(ctx context.Context, workerID string, leaseDuration time.Duration) (*AsyncInvocation, error)
	AcquireDueAsyncInvocations(ctx context.Context, workerID string, leaseDuration time.Duration, batchSize int) ([]*AsyncInvocation, error)
	MarkAsyncInvocationSucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	MarkAsyncInvocationForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	MarkAsyncInvocationDLQ(ctx context.Context, id, lastError string) error
	RequeueAsyncInvocation(ctx context.Context, id string, maxAttempts int) (*AsyncInvocation, error)
	PauseAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error)
	ResumeAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error)
	DeleteAsyncInvocation(ctx context.Context, id string) error
	PauseAsyncInvocationsByFunction(ctx context.Context, functionID string) (int, error)
	ResumeAsyncInvocationsByFunction(ctx context.Context, functionID string) (int, error)
	PauseAsyncInvocationsByWorkflow(ctx context.Context, workflowID string) (int, error)
	ResumeAsyncInvocationsByWorkflow(ctx context.Context, workflowID string) (int, error)
	ListWorkflowAsyncInvocations(ctx context.Context, workflowID string, limit, offset int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error)
	CountAsyncInvocations(ctx context.Context, statuses []AsyncInvocationStatus) (int64, error)
	CountFunctionAsyncInvocations(ctx context.Context, functionID string, statuses []AsyncInvocationStatus) (int64, error)
	CountWorkflowAsyncInvocations(ctx context.Context, workflowID string, statuses []AsyncInvocationStatus) (int64, error)
	GetAsyncInvocationSummary(ctx context.Context) (*AsyncInvocationSummary, error)
	SetGlobalAsyncPause(ctx context.Context, paused bool) error
	GetGlobalAsyncPause(ctx context.Context) (bool, error)
	EnqueueAsyncInvocationWithIdempotency(ctx context.Context, inv *AsyncInvocation, idempotencyKey string, ttl time.Duration) (*AsyncInvocation, bool, error)

	// Event bus (topics / subscriptions / deliveries)
	CreateEventTopic(ctx context.Context, topic *EventTopic) error
	GetEventTopic(ctx context.Context, id string) (*EventTopic, error)
	GetEventTopicByName(ctx context.Context, name string) (*EventTopic, error)
	ListEventTopics(ctx context.Context, limit, offset int) ([]*EventTopic, error)
	DeleteEventTopicByName(ctx context.Context, name string) error

	CreateEventSubscription(ctx context.Context, sub *EventSubscription) error
	GetEventSubscription(ctx context.Context, id string) (*EventSubscription, error)
	ListEventSubscriptions(ctx context.Context, topicID string, limit, offset int) ([]*EventSubscription, error)
	UpdateEventSubscription(ctx context.Context, id string, update *EventSubscriptionUpdate) (*EventSubscription, error)
	DeleteEventSubscription(ctx context.Context, id string) error

	PublishEvent(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*EventMessage, int, error)
	ListEventMessages(ctx context.Context, topicID string, limit, offset int) ([]*EventMessage, error)
	PublishEventFromOutbox(ctx context.Context, outboxID, topicID, orderingKey string, payload, headers json.RawMessage) (*EventMessage, int, bool, error)

	GetEventDelivery(ctx context.Context, id string) (*EventDelivery, error)
	ListEventDeliveries(ctx context.Context, subscriptionID string, limit, offset int, statuses []EventDeliveryStatus) ([]*EventDelivery, error)
	AcquireDueEventDelivery(ctx context.Context, workerID string, leaseDuration time.Duration) (*EventDelivery, error)
	MarkEventDeliverySucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	MarkEventDeliveryForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	MarkEventDeliveryDLQ(ctx context.Context, id, lastError string) error
	RequeueEventDelivery(ctx context.Context, id string, maxAttempts int) (*EventDelivery, error)
	ResolveEventReplaySequenceByTime(ctx context.Context, subscriptionID string, from time.Time) (int64, error)
	SetEventSubscriptionCursor(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*EventSubscription, error)
	ReplayEventSubscription(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error)

	CreateEventOutbox(ctx context.Context, outbox *EventOutbox) error
	GetEventOutbox(ctx context.Context, id string) (*EventOutbox, error)
	ListEventOutbox(ctx context.Context, topicID string, limit, offset int, statuses []EventOutboxStatus) ([]*EventOutbox, error)
	AcquireDueEventOutbox(ctx context.Context, workerID string, leaseDuration time.Duration) (*EventOutbox, error)
	MarkEventOutboxPublished(ctx context.Context, id, messageID string) error
	MarkEventOutboxForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	MarkEventOutboxFailed(ctx context.Context, id, lastError string) error
	RequeueEventOutbox(ctx context.Context, id string, maxAttempts int) (*EventOutbox, error)

	PrepareEventInbox(ctx context.Context, subscriptionID, messageID, deliveryID string) (*EventInboxRecord, bool, error)
	MarkEventInboxSucceeded(ctx context.Context, subscriptionID, messageID, deliveryID, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	MarkEventInboxFailed(ctx context.Context, subscriptionID, messageID, deliveryID, lastError string) error

	// UI notifications
	CreateNotification(ctx context.Context, n *NotificationRecord) error
	ListNotifications(ctx context.Context, limit, offset int, status NotificationStatus) ([]*NotificationRecord, error)
	GetUnreadNotificationCount(ctx context.Context) (int64, error)
	MarkNotificationRead(ctx context.Context, id string) (*NotificationRecord, error)
	MarkAllNotificationsRead(ctx context.Context) (int64, error)

	// Runtimes
	SaveRuntime(ctx context.Context, rt *RuntimeRecord) error
	GetRuntime(ctx context.Context, id string) (*RuntimeRecord, error)
	ListRuntimes(ctx context.Context, limit, offset int) ([]*RuntimeRecord, error)
	DeleteRuntime(ctx context.Context, id string) error

	// Config
	GetConfig(ctx context.Context) (map[string]string, error)
	SetConfig(ctx context.Context, key, value string) error

	// API Keys
	SaveAPIKey(ctx context.Context, key *APIKeyRecord) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error)
	GetAPIKeyByName(ctx context.Context, name string) (*APIKeyRecord, error)
	ListAPIKeys(ctx context.Context, limit, offset int) ([]*APIKeyRecord, error)
	DeleteAPIKey(ctx context.Context, name string) error

	// Secrets
	SaveSecret(ctx context.Context, name, encryptedValue string) error
	GetSecret(ctx context.Context, name string) (string, error)
	DeleteSecret(ctx context.Context, name string) error
	ListSecrets(ctx context.Context) (map[string]string, error)
	SecretExists(ctx context.Context, name string) (bool, error)

	// Rate limiting
	CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error)

	// Function code
	SaveFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error
	GetFunctionCode(ctx context.Context, funcID string) (*domain.FunctionCode, error)
	UpdateFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error
	UpdateCompileResult(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error
	DeleteFunctionCode(ctx context.Context, funcID string) error

	// Function files (multi-file support)
	SaveFunctionFiles(ctx context.Context, funcID string, files map[string][]byte) error
	GetFunctionFiles(ctx context.Context, funcID string) (map[string][]byte, error)
	ListFunctionFiles(ctx context.Context, funcID string) ([]FunctionFileInfo, error)
	DeleteFunctionFiles(ctx context.Context, funcID string) error
	HasFunctionFiles(ctx context.Context, funcID string) (bool, error)

	// Gateway routes
	SaveGatewayRoute(ctx context.Context, route *domain.GatewayRoute) error
	GetGatewayRoute(ctx context.Context, id string) (*domain.GatewayRoute, error)
	GetRouteByDomainPath(ctx context.Context, domain, path string) (*domain.GatewayRoute, error)
	ListGatewayRoutes(ctx context.Context, limit, offset int) ([]*domain.GatewayRoute, error)
	ListRoutesByDomain(ctx context.Context, domain string, limit, offset int) ([]*domain.GatewayRoute, error)
	DeleteGatewayRoute(ctx context.Context, id string) error
	UpdateGatewayRoute(ctx context.Context, id string, route *domain.GatewayRoute) error

	// Layers
	SaveLayer(ctx context.Context, layer *domain.Layer) error
	GetLayer(ctx context.Context, id string) (*domain.Layer, error)
	GetLayerByName(ctx context.Context, name string) (*domain.Layer, error)
	GetLayerByContentHash(ctx context.Context, hash string) (*domain.Layer, error)
	ListLayers(ctx context.Context, limit, offset int) ([]*domain.Layer, error)
	DeleteLayer(ctx context.Context, id string) error
	SetFunctionLayers(ctx context.Context, funcID string, layerIDs []string) error
	GetFunctionLayers(ctx context.Context, funcID string) ([]*domain.Layer, error)
	ListFunctionsByLayer(ctx context.Context, layerID string) ([]string, error)

	// Volumes
	CreateVolume(ctx context.Context, vol *domain.Volume) error
	GetVolume(ctx context.Context, id string) (*domain.Volume, error)
	GetVolumeByName(ctx context.Context, name string) (*domain.Volume, error)
	ListVolumes(ctx context.Context) ([]*domain.Volume, error)
	UpdateVolume(ctx context.Context, id string, updates map[string]interface{}) error
	DeleteVolume(ctx context.Context, id string) error
	GetFunctionVolumes(ctx context.Context, functionID string) ([]*domain.Volume, error)

	// RBAC: Roles
	CreateRole(ctx context.Context, role *RoleRecord) (*RoleRecord, error)
	GetRole(ctx context.Context, id string) (*RoleRecord, error)
	ListRoles(ctx context.Context, tenantID string, limit, offset int) ([]*RoleRecord, error)
	DeleteRole(ctx context.Context, id string) error

	// RBAC: Permissions
	CreatePermission(ctx context.Context, perm *PermissionRecord) (*PermissionRecord, error)
	GetPermission(ctx context.Context, id string) (*PermissionRecord, error)
	ListPermissions(ctx context.Context, limit, offset int) ([]*PermissionRecord, error)
	DeletePermission(ctx context.Context, id string) error

	// RBAC: Role ↔ Permission
	AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error
	RevokePermissionFromRole(ctx context.Context, roleID, permissionID string) error
	ListRolePermissions(ctx context.Context, roleID string) ([]*PermissionRecord, error)

	// RBAC: Role Assignments (scoped)
	CreateRoleAssignment(ctx context.Context, ra *RoleAssignmentRecord) (*RoleAssignmentRecord, error)
	GetRoleAssignment(ctx context.Context, id string) (*RoleAssignmentRecord, error)
	ListRoleAssignments(ctx context.Context, tenantID string, limit, offset int) ([]*RoleAssignmentRecord, error)
	ListRoleAssignmentsByPrincipal(ctx context.Context, tenantID string, principalType domain.PrincipalType, principalID string) ([]*RoleAssignmentRecord, error)
	DeleteRoleAssignment(ctx context.Context, id string) error

	// Tenant-level menu permissions
	ListTenantMenuPermissions(ctx context.Context, tenantID string) ([]*MenuPermissionRecord, error)
	UpsertTenantMenuPermission(ctx context.Context, tenantID, menuKey string, enabled bool) (*MenuPermissionRecord, error)
	DeleteTenantMenuPermission(ctx context.Context, tenantID, menuKey string) error
	SeedDefaultMenuPermissions(ctx context.Context, tenantID string) error

	// Tenant-level button permissions
	ListTenantButtonPermissions(ctx context.Context, tenantID string) ([]*ButtonPermissionRecord, error)
	UpsertTenantButtonPermission(ctx context.Context, tenantID, permissionKey string, enabled bool) (*ButtonPermissionRecord, error)
	DeleteTenantButtonPermission(ctx context.Context, tenantID, permissionKey string) error
	SeedDefaultButtonPermissions(ctx context.Context, tenantID string) error

	// API Doc Shares
	SaveAPIDocShare(ctx context.Context, share *APIDocShare) error
	GetAPIDocShareByToken(ctx context.Context, token string) (*APIDocShare, error)
	ListAPIDocShares(ctx context.Context, tenantID, namespace string, limit, offset int) ([]*APIDocShare, error)
	DeleteAPIDocShare(ctx context.Context, id string) error
	IncrementAPIDocShareAccess(ctx context.Context, token string) error

	// Function Docs (per-function persisted documentation)
	SaveFunctionDoc(ctx context.Context, doc *FunctionDoc) error
	GetFunctionDoc(ctx context.Context, functionName string) (*FunctionDoc, error)
	DeleteFunctionDoc(ctx context.Context, functionName string) error

	// Workflow Docs (per-workflow persisted documentation)
	SaveWorkflowDoc(ctx context.Context, doc *WorkflowDoc) error
	GetWorkflowDoc(ctx context.Context, workflowName string) (*WorkflowDoc, error)
	DeleteWorkflowDoc(ctx context.Context, workflowName string) error

	// Test Suites (per-function persisted test suites)
	SaveTestSuite(ctx context.Context, ts *TestSuite) error
	GetTestSuite(ctx context.Context, functionName string) (*TestSuite, error)
	DeleteTestSuite(ctx context.Context, functionName string) error

	// Database Access (DbResource / DbBinding / CredentialPolicy)
	CreateDbResource(ctx context.Context, res *DbResourceRecord) (*DbResourceRecord, error)
	GetDbResource(ctx context.Context, id string) (*DbResourceRecord, error)
	GetDbResourceByName(ctx context.Context, name string) (*DbResourceRecord, error)
	ListDbResources(ctx context.Context, limit, offset int) ([]*DbResourceRecord, error)
	UpdateDbResource(ctx context.Context, id string, update *DbResourceUpdate) (*DbResourceRecord, error)
	DeleteDbResource(ctx context.Context, id string) error

	CreateDbBinding(ctx context.Context, binding *DbBindingRecord) (*DbBindingRecord, error)
	GetDbBinding(ctx context.Context, id string) (*DbBindingRecord, error)
	ListDbBindings(ctx context.Context, dbResourceID string, limit, offset int) ([]*DbBindingRecord, error)
	ListDbBindingsByFunction(ctx context.Context, functionID string, limit, offset int) ([]*DbBindingRecord, error)
	UpdateDbBinding(ctx context.Context, id string, update *DbBindingUpdate) (*DbBindingRecord, error)
	DeleteDbBinding(ctx context.Context, id string) error

	CreateCredentialPolicy(ctx context.Context, policy *CredentialPolicyRecord) (*CredentialPolicyRecord, error)
	GetCredentialPolicy(ctx context.Context, dbResourceID string) (*CredentialPolicyRecord, error)
	UpdateCredentialPolicy(ctx context.Context, dbResourceID string, update *CredentialPolicyUpdate) (*CredentialPolicyRecord, error)
	DeleteCredentialPolicy(ctx context.Context, dbResourceID string) error

	// Database Access audit log
	SaveDbRequestLog(ctx context.Context, log *domain.DbRequestLog) error
	ListDbRequestLogs(ctx context.Context, dbResourceID string, limit, offset int) ([]*domain.DbRequestLog, error)
}

// Store wraps the MetadataStore, WorkflowStore, and ScheduleStore (Postgres) for all persistence.
type Store struct {
	MetadataStore
	WorkflowStore
	ScheduleStore
}

type metadataStoreUnwrapper interface {
	UnderlyingMetadataStore() MetadataStore
}

func unwrapMetadataStore(meta MetadataStore) MetadataStore {
	seen := map[MetadataStore]struct{}{}
	current := meta
	for current != nil {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}

		unwrapper, ok := current.(metadataStoreUnwrapper)
		if !ok {
			break
		}
		next := unwrapper.UnderlyingMetadataStore()
		if next == nil || next == current {
			break
		}
		current = next
	}
	return current
}

func NewStore(meta MetadataStore) *Store {
	s := &Store{
		MetadataStore: meta,
	}
	// Recover optional stores from the wrapped metadata store chain.
	workflowProvider := meta
	if ws, ok := workflowProvider.(WorkflowStore); ok {
		s.WorkflowStore = ws
	} else if unwrapped := unwrapMetadataStore(meta); unwrapped != nil {
		if ws, ok := unwrapped.(WorkflowStore); ok {
			s.WorkflowStore = ws
		}
	}
	scheduleProvider := meta
	if ss, ok := scheduleProvider.(ScheduleStore); ok {
		s.ScheduleStore = ss
	} else if unwrapped := unwrapMetadataStore(meta); unwrapped != nil {
		if ss, ok := unwrapped.(ScheduleStore); ok {
			s.ScheduleStore = ss
		}
	}
	return s
}

func (s *Store) PingPostgres(ctx context.Context) error {
	if s.MetadataStore == nil {
		return fmt.Errorf("postgres not configured")
	}
	return s.MetadataStore.Ping(ctx)
}

func (s *Store) Ping(ctx context.Context) error {
	return s.PingPostgres(ctx)
}

func (s *Store) Close() error {
	if s.MetadataStore != nil {
		return s.MetadataStore.Close()
	}
	return nil
}

// APIDocShare represents a shared API documentation link.
type APIDocShare struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenant_id"`
	Namespace    string          `json:"namespace"`
	FunctionName string          `json:"function_name"`
	Title        string          `json:"title"`
	Token        string          `json:"token"`
	DocContent   json.RawMessage `json:"doc_content"`
	CreatedBy    string          `json:"created_by"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	AccessCount  int64           `json:"access_count"`
	LastAccessAt *time.Time      `json:"last_access_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// FunctionDoc represents persisted API documentation for a function.
type FunctionDoc struct {
	FunctionName string          `json:"function_name"`
	DocContent   json.RawMessage `json:"doc_content"`
	UpdatedAt    time.Time       `json:"updated_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

// WorkflowDoc represents persisted API documentation for a workflow.
type WorkflowDoc struct {
	WorkflowName string          `json:"workflow_name"`
	DocContent   json.RawMessage `json:"doc_content"`
	UpdatedAt    time.Time       `json:"updated_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

// TestSuite represents a persisted test suite for a function.
type TestSuite struct {
	FunctionName string          `json:"function_name"`
	TestCases    json.RawMessage `json:"test_cases"`
	UpdatedAt    time.Time       `json:"updated_at"`
	CreatedAt    time.Time       `json:"created_at"`
}
