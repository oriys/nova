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
	Handler             *string
	Code                *string // inline code update
	MemoryMB            *int
	TimeoutS            *int
	MinReplicas         *int
	MaxReplicas         *int
	InstanceConcurrency *int
	Mode                *domain.ExecutionMode
	Limits              *domain.ResourceLimits
	NetworkPolicy       *domain.NetworkPolicy
	AutoScalePolicy     *domain.AutoScalePolicy
	CapacityPolicy      *domain.CapacityPolicy
	EnvVars             map[string]string
	MergeEnvVars        bool
}

// MetadataStore is the durable metadata store (functions, versions, aliases).
type MetadataStore interface {
	Close() error
	Ping(ctx context.Context) error

	SaveFunction(ctx context.Context, fn *domain.Function) error
	GetFunction(ctx context.Context, id string) (*domain.Function, error)
	GetFunctionByName(ctx context.Context, name string) (*domain.Function, error)
	DeleteFunction(ctx context.Context, id string) error
	ListFunctions(ctx context.Context) ([]*domain.Function, error)
	UpdateFunction(ctx context.Context, name string, update *FunctionUpdate) (*domain.Function, error)

	PublishVersion(ctx context.Context, funcID string, version *domain.FunctionVersion) error
	GetVersion(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error)
	ListVersions(ctx context.Context, funcID string) ([]*domain.FunctionVersion, error)
	DeleteVersion(ctx context.Context, funcID string, version int) error

	SetAlias(ctx context.Context, alias *domain.FunctionAlias) error
	GetAlias(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error)
	ListAliases(ctx context.Context, funcID string) ([]*domain.FunctionAlias, error)
	DeleteAlias(ctx context.Context, funcID, aliasName string) error

	// Invocation logs
	SaveInvocationLog(ctx context.Context, log *InvocationLog) error
	SaveInvocationLogs(ctx context.Context, logs []*InvocationLog) error
	ListInvocationLogs(ctx context.Context, functionID string, limit int) ([]*InvocationLog, error)
	ListAllInvocationLogs(ctx context.Context, limit int) ([]*InvocationLog, error)
	GetInvocationLog(ctx context.Context, requestID string) (*InvocationLog, error)
	GetFunctionTimeSeries(ctx context.Context, functionID string, rangeSeconds, bucketSeconds int) ([]TimeSeriesBucket, error)
	GetGlobalTimeSeries(ctx context.Context, rangeSeconds, bucketSeconds int) ([]TimeSeriesBucket, error)
	GetFunctionDailyHeatmap(ctx context.Context, functionID string, weeks int) ([]DailyCount, error)
	GetGlobalDailyHeatmap(ctx context.Context, weeks int) ([]DailyCount, error)

	// Async invocations (queue + retries + DLQ)
	EnqueueAsyncInvocation(ctx context.Context, inv *AsyncInvocation) error
	GetAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error)
	ListAsyncInvocations(ctx context.Context, limit int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error)
	ListFunctionAsyncInvocations(ctx context.Context, functionID string, limit int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error)
	AcquireDueAsyncInvocation(ctx context.Context, workerID string, leaseDuration time.Duration) (*AsyncInvocation, error)
	MarkAsyncInvocationSucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	MarkAsyncInvocationForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	MarkAsyncInvocationDLQ(ctx context.Context, id, lastError string) error
	RequeueAsyncInvocation(ctx context.Context, id string, maxAttempts int) (*AsyncInvocation, error)
	EnqueueAsyncInvocationWithIdempotency(ctx context.Context, inv *AsyncInvocation, idempotencyKey string, ttl time.Duration) (*AsyncInvocation, bool, error)

	// Event bus (topics / subscriptions / deliveries)
	CreateEventTopic(ctx context.Context, topic *EventTopic) error
	GetEventTopic(ctx context.Context, id string) (*EventTopic, error)
	GetEventTopicByName(ctx context.Context, name string) (*EventTopic, error)
	ListEventTopics(ctx context.Context, limit int) ([]*EventTopic, error)
	DeleteEventTopicByName(ctx context.Context, name string) error

	CreateEventSubscription(ctx context.Context, sub *EventSubscription) error
	GetEventSubscription(ctx context.Context, id string) (*EventSubscription, error)
	ListEventSubscriptions(ctx context.Context, topicID string) ([]*EventSubscription, error)
	UpdateEventSubscription(ctx context.Context, id string, update *EventSubscriptionUpdate) (*EventSubscription, error)
	DeleteEventSubscription(ctx context.Context, id string) error

	PublishEvent(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*EventMessage, int, error)
	ListEventMessages(ctx context.Context, topicID string, limit int) ([]*EventMessage, error)

	GetEventDelivery(ctx context.Context, id string) (*EventDelivery, error)
	ListEventDeliveries(ctx context.Context, subscriptionID string, limit int, statuses []EventDeliveryStatus) ([]*EventDelivery, error)
	AcquireDueEventDelivery(ctx context.Context, workerID string, leaseDuration time.Duration) (*EventDelivery, error)
	MarkEventDeliverySucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	MarkEventDeliveryForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	MarkEventDeliveryDLQ(ctx context.Context, id, lastError string) error
	RequeueEventDelivery(ctx context.Context, id string, maxAttempts int) (*EventDelivery, error)
	ReplayEventSubscription(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error)

	// Runtimes
	SaveRuntime(ctx context.Context, rt *RuntimeRecord) error
	GetRuntime(ctx context.Context, id string) (*RuntimeRecord, error)
	ListRuntimes(ctx context.Context) ([]*RuntimeRecord, error)
	DeleteRuntime(ctx context.Context, id string) error

	// Config
	GetConfig(ctx context.Context) (map[string]string, error)
	SetConfig(ctx context.Context, key, value string) error

	// API Keys
	SaveAPIKey(ctx context.Context, key *APIKeyRecord) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error)
	GetAPIKeyByName(ctx context.Context, name string) (*APIKeyRecord, error)
	ListAPIKeys(ctx context.Context) ([]*APIKeyRecord, error)
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
	ListGatewayRoutes(ctx context.Context) ([]*domain.GatewayRoute, error)
	ListRoutesByDomain(ctx context.Context, domain string) ([]*domain.GatewayRoute, error)
	DeleteGatewayRoute(ctx context.Context, id string) error
	UpdateGatewayRoute(ctx context.Context, id string, route *domain.GatewayRoute) error

	// Layers
	SaveLayer(ctx context.Context, layer *domain.Layer) error
	GetLayer(ctx context.Context, id string) (*domain.Layer, error)
	GetLayerByName(ctx context.Context, name string) (*domain.Layer, error)
	GetLayerByContentHash(ctx context.Context, hash string) (*domain.Layer, error)
	ListLayers(ctx context.Context) ([]*domain.Layer, error)
	DeleteLayer(ctx context.Context, id string) error
	SetFunctionLayers(ctx context.Context, funcID string, layerIDs []string) error
	GetFunctionLayers(ctx context.Context, funcID string) ([]*domain.Layer, error)
	ListFunctionsByLayer(ctx context.Context, layerID string) ([]string, error)
}

// Store wraps the MetadataStore, WorkflowStore, and ScheduleStore (Postgres) for all persistence.
type Store struct {
	MetadataStore
	WorkflowStore
	ScheduleStore
}

func NewStore(meta MetadataStore) *Store {
	s := &Store{
		MetadataStore: meta,
	}
	// PostgresStore implements both interfaces
	if ws, ok := meta.(WorkflowStore); ok {
		s.WorkflowStore = ws
	}
	if ss, ok := meta.(ScheduleStore); ok {
		s.ScheduleStore = ss
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
