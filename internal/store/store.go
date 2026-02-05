package store

import (
	"context"
	"fmt"

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
	GetFunctionTimeSeries(ctx context.Context, functionID string, hours int) ([]TimeSeriesBucket, error)
	GetGlobalTimeSeries(ctx context.Context, hours int) ([]TimeSeriesBucket, error)

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
}

// Store wraps the MetadataStore (Postgres) for all persistence.
type Store struct {
	MetadataStore
}

func NewStore(meta MetadataStore) *Store {
	return &Store{
		MetadataStore: meta,
	}
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
