package dataplane

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// mockMetadataStore implements store.MetadataStore with configurable function
// fields. Methods not explicitly set return zero-value defaults.
type mockMetadataStore struct {
	pingFn                                  func(ctx context.Context) error
	closeFn                                 func() error
	getFunctionByNameFn                     func(ctx context.Context, name string) (*domain.Function, error)
	getFunctionFn                           func(ctx context.Context, id string) (*domain.Function, error)
	listFunctionsFn                         func(ctx context.Context, limit, offset int) ([]*domain.Function, error)
	saveFunctionFn                          func(ctx context.Context, fn *domain.Function) error
	deleteFunctionFn                        func(ctx context.Context, id string) error
	updateFunctionFn                        func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error)
	searchFunctionsFn                       func(ctx context.Context, query string, limit, offset int) ([]*domain.Function, error)
	listInvocationLogsFn                    func(ctx context.Context, functionID string, limit, offset int) ([]*store.InvocationLog, error)
	listAllInvocationLogsFn                 func(ctx context.Context, limit, offset int) ([]*store.InvocationLog, error)
	getInvocationLogFn                      func(ctx context.Context, requestID string) (*store.InvocationLog, error)
	saveInvocationLogFn                     func(ctx context.Context, log *store.InvocationLog) error
	saveInvocationLogsFn                    func(ctx context.Context, logs []*store.InvocationLog) error
	getFunctionTimeSeriesFn                 func(ctx context.Context, functionID string, rangeSeconds, bucketSeconds int) ([]store.TimeSeriesBucket, error)
	getGlobalTimeSeriesFn                   func(ctx context.Context, rangeSeconds, bucketSeconds int) ([]store.TimeSeriesBucket, error)
	getFunctionDailyHeatmapFn               func(ctx context.Context, functionID string, weeks int) ([]store.DailyCount, error)
	getGlobalDailyHeatmapFn                 func(ctx context.Context, weeks int) ([]store.DailyCount, error)
	getFunctionSLOSnapshotFn                func(ctx context.Context, functionID string, windowSeconds int) (*store.FunctionSLOSnapshot, error)
	getFunctionStateFn                      func(ctx context.Context, functionID, key string) (*store.FunctionStateEntry, error)
	putFunctionStateFn                      func(ctx context.Context, functionID, key string, value json.RawMessage, opts *store.FunctionStatePutOptions) (*store.FunctionStateEntry, error)
	deleteFunctionStateFn                   func(ctx context.Context, functionID, key string) error
	listFunctionStatesFn                    func(ctx context.Context, functionID string, opts *store.FunctionStateListOptions) ([]*store.FunctionStateEntry, error)
	countFunctionStatesFn                   func(ctx context.Context, functionID, prefix string) (int64, error)
	enqueueAsyncInvocationFn                func(ctx context.Context, inv *store.AsyncInvocation) error
	getAsyncInvocationFn                    func(ctx context.Context, id string) (*store.AsyncInvocation, error)
	listAsyncInvocationsFn                  func(ctx context.Context, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error)
	listFunctionAsyncInvocationsFn          func(ctx context.Context, functionID string, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error)
	requeueAsyncInvocationFn                func(ctx context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error)
	pauseAsyncInvocationFn                  func(ctx context.Context, id string) (*store.AsyncInvocation, error)
	resumeAsyncInvocationFn                 func(ctx context.Context, id string) (*store.AsyncInvocation, error)
	deleteAsyncInvocationFn                 func(ctx context.Context, id string) error
	pauseAsyncInvocationsByFunctionFn       func(ctx context.Context, functionID string) (int, error)
	resumeAsyncInvocationsByFunctionFn      func(ctx context.Context, functionID string) (int, error)
	pauseAsyncInvocationsByWorkflowFn       func(ctx context.Context, workflowID string) (int, error)
	resumeAsyncInvocationsByWorkflowFn      func(ctx context.Context, workflowID string) (int, error)
	listWorkflowAsyncInvocationsFn          func(ctx context.Context, workflowID string, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error)
	countAsyncInvocationsFn                 func(ctx context.Context, statuses []store.AsyncInvocationStatus) (int64, error)
	countFunctionAsyncInvocationsFn         func(ctx context.Context, functionID string, statuses []store.AsyncInvocationStatus) (int64, error)
	countWorkflowAsyncInvocationsFn         func(ctx context.Context, workflowID string, statuses []store.AsyncInvocationStatus) (int64, error)
	getAsyncInvocationSummaryFn             func(ctx context.Context) (*store.AsyncInvocationSummary, error)
	setGlobalAsyncPauseFn                   func(ctx context.Context, paused bool) error
	getGlobalAsyncPauseFn                   func(ctx context.Context) (bool, error)
	enqueueAsyncInvocationWithIdempotencyFn func(ctx context.Context, inv *store.AsyncInvocation, key string, ttl time.Duration) (*store.AsyncInvocation, bool, error)
	checkAndConsumeTenantQuotaFn            func(ctx context.Context, tenantID, dimension string, amount int64) (*store.TenantQuotaDecision, error)
	checkTenantAbsoluteQuotaFn              func(ctx context.Context, tenantID, dimension string, value int64) (*store.TenantQuotaDecision, error)
	getTenantAsyncQueueDepthFn              func(ctx context.Context, tenantID string) (int64, error)
	getFunctionCodeFn                       func(ctx context.Context, funcID string) (*domain.FunctionCode, error)

	// pagination extension
	countInvocationLogsFn                      func(ctx context.Context, functionID string) (int64, error)
	countAllInvocationLogsFn                   func(ctx context.Context) (int64, error)
	listAllInvocationLogsFilteredFn            func(ctx context.Context, limit, offset int, search, functionName string, success *bool) ([]*store.InvocationLog, error)
	countAllInvocationLogsFilteredFn           func(ctx context.Context, search, functionName string, success *bool) (int64, error)
	getAllInvocationLogsSummaryFn               func(ctx context.Context) (*store.InvocationLogSummary, error)
	getAllInvocationLogsSummaryFilteredFn       func(ctx context.Context, search, functionName string, success *bool) (*store.InvocationLogSummary, error)
}

// Ensure mockMetadataStore satisfies MetadataStore + pagination interfaces.
var _ store.MetadataStore = (*mockMetadataStore)(nil)
var _ invocationPaginationStore = (*mockMetadataStore)(nil)
var _ asyncInvocationPaginationStore = (*mockMetadataStore)(nil)

func (m *mockMetadataStore) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return nil
}
func (m *mockMetadataStore) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func (m *mockMetadataStore) SaveFunction(ctx context.Context, fn *domain.Function) error {
	if m.saveFunctionFn != nil {
		return m.saveFunctionFn(ctx, fn)
	}
	return nil
}
func (m *mockMetadataStore) GetFunction(ctx context.Context, id string) (*domain.Function, error) {
	if m.getFunctionFn != nil {
		return m.getFunctionFn(ctx, id)
	}
	return nil, store.ErrAsyncInvocationNotFound
}
func (m *mockMetadataStore) GetFunctionByName(ctx context.Context, name string) (*domain.Function, error) {
	if m.getFunctionByNameFn != nil {
		return m.getFunctionByNameFn(ctx, name)
	}
	return nil, store.ErrAsyncInvocationNotFound
}
func (m *mockMetadataStore) DeleteFunction(ctx context.Context, id string) error {
	if m.deleteFunctionFn != nil {
		return m.deleteFunctionFn(ctx, id)
	}
	return nil
}
func (m *mockMetadataStore) ListFunctions(ctx context.Context, limit, offset int) ([]*domain.Function, error) {
	if m.listFunctionsFn != nil {
		return m.listFunctionsFn(ctx, limit, offset)
	}
	return nil, nil
}
func (m *mockMetadataStore) SearchFunctions(ctx context.Context, query string, limit, offset int) ([]*domain.Function, error) {
	if m.searchFunctionsFn != nil {
		return m.searchFunctionsFn(ctx, query, limit, offset)
	}
	return nil, nil
}
func (m *mockMetadataStore) UpdateFunction(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error) {
	if m.updateFunctionFn != nil {
		return m.updateFunctionFn(ctx, name, update)
	}
	return nil, nil
}

// Tenancy stubs
func (m *mockMetadataStore) ListTenants(ctx context.Context, limit, offset int) ([]*store.TenantRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetTenant(ctx context.Context, id string) (*store.TenantRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) CreateTenant(ctx context.Context, t *store.TenantRecord) (*store.TenantRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateTenant(ctx context.Context, id string, u *store.TenantUpdate) (*store.TenantRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteTenant(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) SetTenantPasswordHash(ctx context.Context, id, hash string) error {
	return nil
}
func (m *mockMetadataStore) ListNamespaces(ctx context.Context, tenantID string, limit, offset int) ([]*store.NamespaceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetNamespace(ctx context.Context, tenantID, name string) (*store.NamespaceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) CreateNamespace(ctx context.Context, ns *store.NamespaceRecord) (*store.NamespaceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateNamespace(ctx context.Context, tenantID, name string, u *store.NamespaceUpdate) (*store.NamespaceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteNamespace(ctx context.Context, tenantID, name string) error {
	return nil
}

// Tenant governance stubs
func (m *mockMetadataStore) ListTenantQuotas(ctx context.Context, tenantID string) ([]*store.TenantQuotaRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpsertTenantQuota(ctx context.Context, q *store.TenantQuotaRecord) (*store.TenantQuotaRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteTenantQuota(ctx context.Context, tenantID, dim string) error {
	return nil
}
func (m *mockMetadataStore) ListTenantUsage(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) RefreshTenantUsage(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) CheckAndConsumeTenantQuota(ctx context.Context, tenantID, dimension string, amount int64) (*store.TenantQuotaDecision, error) {
	if m.checkAndConsumeTenantQuotaFn != nil {
		return m.checkAndConsumeTenantQuotaFn(ctx, tenantID, dimension, amount)
	}
	return &store.TenantQuotaDecision{Allowed: true}, nil
}
func (m *mockMetadataStore) CheckTenantAbsoluteQuota(ctx context.Context, tenantID, dimension string, value int64) (*store.TenantQuotaDecision, error) {
	if m.checkTenantAbsoluteQuotaFn != nil {
		return m.checkTenantAbsoluteQuotaFn(ctx, tenantID, dimension, value)
	}
	return &store.TenantQuotaDecision{Allowed: true}, nil
}
func (m *mockMetadataStore) GetTenantFunctionCount(ctx context.Context, tenantID string) (int64, error) {
	return 0, nil
}
func (m *mockMetadataStore) GetTenantAsyncQueueDepth(ctx context.Context, tenantID string) (int64, error) {
	if m.getTenantAsyncQueueDepthFn != nil {
		return m.getTenantAsyncQueueDepthFn(ctx, tenantID)
	}
	return 0, nil
}

// Versions
func (m *mockMetadataStore) PublishVersion(ctx context.Context, funcID string, v *domain.FunctionVersion) error {
	return nil
}
func (m *mockMetadataStore) GetVersion(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListVersions(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionVersion, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteVersion(ctx context.Context, funcID string, version int) error {
	return nil
}

// Aliases
func (m *mockMetadataStore) SetAlias(ctx context.Context, a *domain.FunctionAlias) error { return nil }
func (m *mockMetadataStore) GetAlias(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListAliases(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionAlias, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteAlias(ctx context.Context, funcID, aliasName string) error {
	return nil
}

// Invocation logs
func (m *mockMetadataStore) SaveInvocationLog(ctx context.Context, log *store.InvocationLog) error {
	if m.saveInvocationLogFn != nil {
		return m.saveInvocationLogFn(ctx, log)
	}
	return nil
}
func (m *mockMetadataStore) SaveInvocationLogs(ctx context.Context, logs []*store.InvocationLog) error {
	if m.saveInvocationLogsFn != nil {
		return m.saveInvocationLogsFn(ctx, logs)
	}
	return nil
}
func (m *mockMetadataStore) ListInvocationLogs(ctx context.Context, functionID string, limit, offset int) ([]*store.InvocationLog, error) {
	if m.listInvocationLogsFn != nil {
		return m.listInvocationLogsFn(ctx, functionID, limit, offset)
	}
	return nil, nil
}
func (m *mockMetadataStore) ListAllInvocationLogs(ctx context.Context, limit, offset int) ([]*store.InvocationLog, error) {
	if m.listAllInvocationLogsFn != nil {
		return m.listAllInvocationLogsFn(ctx, limit, offset)
	}
	return nil, nil
}
func (m *mockMetadataStore) GetInvocationLog(ctx context.Context, requestID string) (*store.InvocationLog, error) {
	if m.getInvocationLogFn != nil {
		return m.getInvocationLogFn(ctx, requestID)
	}
	return nil, nil
}
func (m *mockMetadataStore) GetFunctionTimeSeries(ctx context.Context, functionID string, rangeSeconds, bucketSeconds int) ([]store.TimeSeriesBucket, error) {
	if m.getFunctionTimeSeriesFn != nil {
		return m.getFunctionTimeSeriesFn(ctx, functionID, rangeSeconds, bucketSeconds)
	}
	return nil, nil
}
func (m *mockMetadataStore) GetGlobalTimeSeries(ctx context.Context, rangeSeconds, bucketSeconds int) ([]store.TimeSeriesBucket, error) {
	if m.getGlobalTimeSeriesFn != nil {
		return m.getGlobalTimeSeriesFn(ctx, rangeSeconds, bucketSeconds)
	}
	return nil, nil
}
func (m *mockMetadataStore) GetFunctionDailyHeatmap(ctx context.Context, functionID string, weeks int) ([]store.DailyCount, error) {
	if m.getFunctionDailyHeatmapFn != nil {
		return m.getFunctionDailyHeatmapFn(ctx, functionID, weeks)
	}
	return nil, nil
}
func (m *mockMetadataStore) GetGlobalDailyHeatmap(ctx context.Context, weeks int) ([]store.DailyCount, error) {
	if m.getGlobalDailyHeatmapFn != nil {
		return m.getGlobalDailyHeatmapFn(ctx, weeks)
	}
	return nil, nil
}
func (m *mockMetadataStore) GetFunctionSLOSnapshot(ctx context.Context, functionID string, windowSeconds int) (*store.FunctionSLOSnapshot, error) {
	if m.getFunctionSLOSnapshotFn != nil {
		return m.getFunctionSLOSnapshotFn(ctx, functionID, windowSeconds)
	}
	return &store.FunctionSLOSnapshot{}, nil
}

// Function state
func (m *mockMetadataStore) GetFunctionState(ctx context.Context, functionID, key string) (*store.FunctionStateEntry, error) {
	if m.getFunctionStateFn != nil {
		return m.getFunctionStateFn(ctx, functionID, key)
	}
	return nil, store.ErrFunctionStateNotFound
}
func (m *mockMetadataStore) PutFunctionState(ctx context.Context, functionID, key string, value json.RawMessage, opts *store.FunctionStatePutOptions) (*store.FunctionStateEntry, error) {
	if m.putFunctionStateFn != nil {
		return m.putFunctionStateFn(ctx, functionID, key, value, opts)
	}
	return &store.FunctionStateEntry{FunctionID: functionID, Key: key, Value: value, Version: 1}, nil
}
func (m *mockMetadataStore) DeleteFunctionState(ctx context.Context, functionID, key string) error {
	if m.deleteFunctionStateFn != nil {
		return m.deleteFunctionStateFn(ctx, functionID, key)
	}
	return nil
}
func (m *mockMetadataStore) ListFunctionStates(ctx context.Context, functionID string, opts *store.FunctionStateListOptions) ([]*store.FunctionStateEntry, error) {
	if m.listFunctionStatesFn != nil {
		return m.listFunctionStatesFn(ctx, functionID, opts)
	}
	return nil, nil
}
func (m *mockMetadataStore) CountFunctionStates(ctx context.Context, functionID, prefix string) (int64, error) {
	if m.countFunctionStatesFn != nil {
		return m.countFunctionStatesFn(ctx, functionID, prefix)
	}
	return 0, nil
}

// Async invocations
func (m *mockMetadataStore) EnqueueAsyncInvocation(ctx context.Context, inv *store.AsyncInvocation) error {
	if m.enqueueAsyncInvocationFn != nil {
		return m.enqueueAsyncInvocationFn(ctx, inv)
	}
	return nil
}
func (m *mockMetadataStore) GetAsyncInvocation(ctx context.Context, id string) (*store.AsyncInvocation, error) {
	if m.getAsyncInvocationFn != nil {
		return m.getAsyncInvocationFn(ctx, id)
	}
	return nil, store.ErrAsyncInvocationNotFound
}
func (m *mockMetadataStore) ListAsyncInvocations(ctx context.Context, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
	if m.listAsyncInvocationsFn != nil {
		return m.listAsyncInvocationsFn(ctx, limit, offset, statuses)
	}
	return nil, nil
}
func (m *mockMetadataStore) ListFunctionAsyncInvocations(ctx context.Context, functionID string, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
	if m.listFunctionAsyncInvocationsFn != nil {
		return m.listFunctionAsyncInvocationsFn(ctx, functionID, limit, offset, statuses)
	}
	return nil, nil
}
func (m *mockMetadataStore) AcquireDueAsyncInvocation(ctx context.Context, workerID string, lease time.Duration) (*store.AsyncInvocation, error) {
	return nil, nil
}
func (m *mockMetadataStore) AcquireDueAsyncInvocations(ctx context.Context, workerID string, lease time.Duration, batch int) ([]*store.AsyncInvocation, error) {
	return nil, nil
}
func (m *mockMetadataStore) MarkAsyncInvocationSucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	return nil
}
func (m *mockMetadataStore) MarkAsyncInvocationForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	return nil
}
func (m *mockMetadataStore) MarkAsyncInvocationDLQ(ctx context.Context, id, lastError string) error {
	return nil
}
func (m *mockMetadataStore) RequeueAsyncInvocation(ctx context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error) {
	if m.requeueAsyncInvocationFn != nil {
		return m.requeueAsyncInvocationFn(ctx, id, maxAttempts)
	}
	return nil, store.ErrAsyncInvocationNotFound
}
func (m *mockMetadataStore) PauseAsyncInvocation(ctx context.Context, id string) (*store.AsyncInvocation, error) {
	if m.pauseAsyncInvocationFn != nil {
		return m.pauseAsyncInvocationFn(ctx, id)
	}
	return nil, store.ErrAsyncInvocationNotFound
}
func (m *mockMetadataStore) ResumeAsyncInvocation(ctx context.Context, id string) (*store.AsyncInvocation, error) {
	if m.resumeAsyncInvocationFn != nil {
		return m.resumeAsyncInvocationFn(ctx, id)
	}
	return nil, store.ErrAsyncInvocationNotFound
}
func (m *mockMetadataStore) DeleteAsyncInvocation(ctx context.Context, id string) error {
	if m.deleteAsyncInvocationFn != nil {
		return m.deleteAsyncInvocationFn(ctx, id)
	}
	return store.ErrAsyncInvocationNotFound
}
func (m *mockMetadataStore) PauseAsyncInvocationsByFunction(ctx context.Context, functionID string) (int, error) {
	if m.pauseAsyncInvocationsByFunctionFn != nil {
		return m.pauseAsyncInvocationsByFunctionFn(ctx, functionID)
	}
	return 0, nil
}
func (m *mockMetadataStore) ResumeAsyncInvocationsByFunction(ctx context.Context, functionID string) (int, error) {
	if m.resumeAsyncInvocationsByFunctionFn != nil {
		return m.resumeAsyncInvocationsByFunctionFn(ctx, functionID)
	}
	return 0, nil
}
func (m *mockMetadataStore) PauseAsyncInvocationsByWorkflow(ctx context.Context, workflowID string) (int, error) {
	if m.pauseAsyncInvocationsByWorkflowFn != nil {
		return m.pauseAsyncInvocationsByWorkflowFn(ctx, workflowID)
	}
	return 0, nil
}
func (m *mockMetadataStore) ResumeAsyncInvocationsByWorkflow(ctx context.Context, workflowID string) (int, error) {
	if m.resumeAsyncInvocationsByWorkflowFn != nil {
		return m.resumeAsyncInvocationsByWorkflowFn(ctx, workflowID)
	}
	return 0, nil
}
func (m *mockMetadataStore) ListWorkflowAsyncInvocations(ctx context.Context, workflowID string, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
	if m.listWorkflowAsyncInvocationsFn != nil {
		return m.listWorkflowAsyncInvocationsFn(ctx, workflowID, limit, offset, statuses)
	}
	return nil, nil
}
func (m *mockMetadataStore) CountAsyncInvocations(ctx context.Context, statuses []store.AsyncInvocationStatus) (int64, error) {
	if m.countAsyncInvocationsFn != nil {
		return m.countAsyncInvocationsFn(ctx, statuses)
	}
	return 0, nil
}
func (m *mockMetadataStore) CountFunctionAsyncInvocations(ctx context.Context, functionID string, statuses []store.AsyncInvocationStatus) (int64, error) {
	if m.countFunctionAsyncInvocationsFn != nil {
		return m.countFunctionAsyncInvocationsFn(ctx, functionID, statuses)
	}
	return 0, nil
}
func (m *mockMetadataStore) CountWorkflowAsyncInvocations(ctx context.Context, workflowID string, statuses []store.AsyncInvocationStatus) (int64, error) {
	if m.countWorkflowAsyncInvocationsFn != nil {
		return m.countWorkflowAsyncInvocationsFn(ctx, workflowID, statuses)
	}
	return 0, nil
}
func (m *mockMetadataStore) GetAsyncInvocationSummary(ctx context.Context) (*store.AsyncInvocationSummary, error) {
	if m.getAsyncInvocationSummaryFn != nil {
		return m.getAsyncInvocationSummaryFn(ctx)
	}
	return &store.AsyncInvocationSummary{}, nil
}
func (m *mockMetadataStore) SetGlobalAsyncPause(ctx context.Context, paused bool) error {
	if m.setGlobalAsyncPauseFn != nil {
		return m.setGlobalAsyncPauseFn(ctx, paused)
	}
	return nil
}
func (m *mockMetadataStore) GetGlobalAsyncPause(ctx context.Context) (bool, error) {
	if m.getGlobalAsyncPauseFn != nil {
		return m.getGlobalAsyncPauseFn(ctx)
	}
	return false, nil
}
func (m *mockMetadataStore) EnqueueAsyncInvocationWithIdempotency(ctx context.Context, inv *store.AsyncInvocation, key string, ttl time.Duration) (*store.AsyncInvocation, bool, error) {
	if m.enqueueAsyncInvocationWithIdempotencyFn != nil {
		return m.enqueueAsyncInvocationWithIdempotencyFn(ctx, inv, key, ttl)
	}
	return inv, false, nil
}

// Event bus stubs (not used by dataplane handlers)
func (m *mockMetadataStore) CreateEventTopic(ctx context.Context, t *store.EventTopic) error {
	return nil
}
func (m *mockMetadataStore) GetEventTopic(ctx context.Context, id string) (*store.EventTopic, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetEventTopicByName(ctx context.Context, name string) (*store.EventTopic, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListEventTopics(ctx context.Context, limit, offset int) ([]*store.EventTopic, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteEventTopicByName(ctx context.Context, name string) error {
	return nil
}
func (m *mockMetadataStore) CreateEventSubscription(ctx context.Context, sub *store.EventSubscription) error {
	return nil
}
func (m *mockMetadataStore) GetEventSubscription(ctx context.Context, id string) (*store.EventSubscription, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListEventSubscriptions(ctx context.Context, topicID string, limit, offset int) ([]*store.EventSubscription, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateEventSubscription(ctx context.Context, id string, u *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteEventSubscription(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) PublishEvent(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, error) {
	return nil, 0, nil
}
func (m *mockMetadataStore) ListEventMessages(ctx context.Context, topicID string, limit, offset int) ([]*store.EventMessage, error) {
	return nil, nil
}
func (m *mockMetadataStore) PublishEventFromOutbox(ctx context.Context, outboxID, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, bool, error) {
	return nil, 0, false, nil
}
func (m *mockMetadataStore) GetEventDelivery(ctx context.Context, id string) (*store.EventDelivery, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListEventDeliveries(ctx context.Context, subscriptionID string, limit, offset int, statuses []store.EventDeliveryStatus) ([]*store.EventDelivery, error) {
	return nil, nil
}
func (m *mockMetadataStore) AcquireDueEventDelivery(ctx context.Context, workerID string, lease time.Duration) (*store.EventDelivery, error) {
	return nil, nil
}
func (m *mockMetadataStore) MarkEventDeliverySucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	return nil
}
func (m *mockMetadataStore) MarkEventDeliveryForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	return nil
}
func (m *mockMetadataStore) MarkEventDeliveryDLQ(ctx context.Context, id, lastError string) error {
	return nil
}
func (m *mockMetadataStore) RequeueEventDelivery(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error) {
	return nil, nil
}
func (m *mockMetadataStore) ResolveEventReplaySequenceByTime(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
	return 0, nil
}
func (m *mockMetadataStore) SetEventSubscriptionCursor(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
	return nil, nil
}
func (m *mockMetadataStore) ReplayEventSubscription(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
	return 0, nil
}
func (m *mockMetadataStore) CreateEventOutbox(ctx context.Context, outbox *store.EventOutbox) error {
	return nil
}
func (m *mockMetadataStore) GetEventOutbox(ctx context.Context, id string) (*store.EventOutbox, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListEventOutbox(ctx context.Context, topicID string, limit, offset int, statuses []store.EventOutboxStatus) ([]*store.EventOutbox, error) {
	return nil, nil
}
func (m *mockMetadataStore) AcquireDueEventOutbox(ctx context.Context, workerID string, lease time.Duration) (*store.EventOutbox, error) {
	return nil, nil
}
func (m *mockMetadataStore) MarkEventOutboxPublished(ctx context.Context, id, messageID string) error {
	return nil
}
func (m *mockMetadataStore) MarkEventOutboxForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	return nil
}
func (m *mockMetadataStore) MarkEventOutboxFailed(ctx context.Context, id, lastError string) error {
	return nil
}
func (m *mockMetadataStore) RequeueEventOutbox(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error) {
	return nil, nil
}
func (m *mockMetadataStore) PrepareEventInbox(ctx context.Context, subscriptionID, messageID, deliveryID string) (*store.EventInboxRecord, bool, error) {
	return nil, false, nil
}
func (m *mockMetadataStore) MarkEventInboxSucceeded(ctx context.Context, subscriptionID, messageID, deliveryID, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	return nil
}
func (m *mockMetadataStore) MarkEventInboxFailed(ctx context.Context, subscriptionID, messageID, deliveryID, lastError string) error {
	return nil
}

// Notifications
func (m *mockMetadataStore) CreateNotification(ctx context.Context, n *store.NotificationRecord) error {
	return nil
}
func (m *mockMetadataStore) ListNotifications(ctx context.Context, limit, offset int, status store.NotificationStatus) ([]*store.NotificationRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetUnreadNotificationCount(ctx context.Context) (int64, error) {
	return 0, nil
}
func (m *mockMetadataStore) MarkNotificationRead(ctx context.Context, id string) (*store.NotificationRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) MarkAllNotificationsRead(ctx context.Context) (int64, error) {
	return 0, nil
}

// Runtimes
func (m *mockMetadataStore) SaveRuntime(ctx context.Context, rt *store.RuntimeRecord) error {
	return nil
}
func (m *mockMetadataStore) GetRuntime(ctx context.Context, id string) (*store.RuntimeRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListRuntimes(ctx context.Context, limit, offset int) ([]*store.RuntimeRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteRuntime(ctx context.Context, id string) error { return nil }

// Config
func (m *mockMetadataStore) GetConfig(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (m *mockMetadataStore) SetConfig(ctx context.Context, key, value string) error { return nil }

// API Keys
func (m *mockMetadataStore) SaveAPIKey(ctx context.Context, key *store.APIKeyRecord) error {
	return nil
}
func (m *mockMetadataStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*store.APIKeyRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetAPIKeyByName(ctx context.Context, name string) (*store.APIKeyRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListAPIKeys(ctx context.Context, limit, offset int) ([]*store.APIKeyRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteAPIKey(ctx context.Context, name string) error { return nil }

// Secrets
func (m *mockMetadataStore) SaveSecret(ctx context.Context, name, encryptedValue string) error {
	return nil
}
func (m *mockMetadataStore) GetSecret(ctx context.Context, name string) (string, error) {
	return "", nil
}
func (m *mockMetadataStore) DeleteSecret(ctx context.Context, name string) error { return nil }
func (m *mockMetadataStore) ListSecrets(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (m *mockMetadataStore) SecretExists(ctx context.Context, name string) (bool, error) {
	return false, nil
}

// Rate limiting
func (m *mockMetadataStore) CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error) {
	return true, maxTokens, nil
}

// Function code
func (m *mockMetadataStore) SaveFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	return nil
}
func (m *mockMetadataStore) GetFunctionCode(ctx context.Context, funcID string) (*domain.FunctionCode, error) {
	if m.getFunctionCodeFn != nil {
		return m.getFunctionCodeFn(ctx, funcID)
	}
	return &domain.FunctionCode{SourceCode: "code"}, nil
}
func (m *mockMetadataStore) UpdateFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	return nil
}
func (m *mockMetadataStore) UpdateCompileResult(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
	return nil
}
func (m *mockMetadataStore) DeleteFunctionCode(ctx context.Context, funcID string) error { return nil }

// Function files
func (m *mockMetadataStore) SaveFunctionFiles(ctx context.Context, funcID string, files map[string][]byte) error {
	return nil
}
func (m *mockMetadataStore) GetFunctionFiles(ctx context.Context, funcID string) (map[string][]byte, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListFunctionFiles(ctx context.Context, funcID string) ([]store.FunctionFileInfo, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteFunctionFiles(ctx context.Context, funcID string) error { return nil }
func (m *mockMetadataStore) HasFunctionFiles(ctx context.Context, funcID string) (bool, error) {
	return false, nil
}

// Gateway routes
func (m *mockMetadataStore) SaveGatewayRoute(ctx context.Context, route *domain.GatewayRoute) error {
	return nil
}
func (m *mockMetadataStore) GetGatewayRoute(ctx context.Context, id string) (*domain.GatewayRoute, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetRouteByDomainPath(ctx context.Context, d, path string) (*domain.GatewayRoute, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListGatewayRoutes(ctx context.Context, limit, offset int) ([]*domain.GatewayRoute, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListRoutesByDomain(ctx context.Context, d string, limit, offset int) ([]*domain.GatewayRoute, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteGatewayRoute(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) UpdateGatewayRoute(ctx context.Context, id string, route *domain.GatewayRoute) error {
	return nil
}

// Layers
func (m *mockMetadataStore) SaveLayer(ctx context.Context, layer *domain.Layer) error { return nil }
func (m *mockMetadataStore) GetLayer(ctx context.Context, id string) (*domain.Layer, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetLayerByName(ctx context.Context, name string) (*domain.Layer, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetLayerByContentHash(ctx context.Context, hash string) (*domain.Layer, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListLayers(ctx context.Context, limit, offset int) ([]*domain.Layer, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteLayer(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) SetFunctionLayers(ctx context.Context, funcID string, layerIDs []string) error {
	return nil
}
func (m *mockMetadataStore) GetFunctionLayers(ctx context.Context, funcID string) ([]*domain.Layer, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListFunctionsByLayer(ctx context.Context, layerID string) ([]string, error) {
	return nil, nil
}

// Triggers
func (m *mockMetadataStore) CreateTrigger(ctx context.Context, t *store.TriggerRecord) error {
	return nil
}
func (m *mockMetadataStore) GetTrigger(ctx context.Context, id string) (*store.TriggerRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetTriggerByName(ctx context.Context, name string) (*store.TriggerRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListTriggers(ctx context.Context, limit, offset int) ([]*store.TriggerRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateTrigger(ctx context.Context, id string, u *store.TriggerUpdate) (*store.TriggerRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteTrigger(ctx context.Context, id string) error { return nil }

// Volumes
func (m *mockMetadataStore) CreateVolume(ctx context.Context, vol *domain.Volume) error { return nil }
func (m *mockMetadataStore) GetVolume(ctx context.Context, id string) (*domain.Volume, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetVolumeByName(ctx context.Context, name string) (*domain.Volume, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListVolumes(ctx context.Context) ([]*domain.Volume, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateVolume(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *mockMetadataStore) DeleteVolume(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) GetFunctionVolumes(ctx context.Context, functionID string) ([]*domain.Volume, error) {
	return nil, nil
}

// RBAC: Roles
func (m *mockMetadataStore) CreateRole(ctx context.Context, role *store.RoleRecord) (*store.RoleRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetRole(ctx context.Context, id string) (*store.RoleRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListRoles(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteRole(ctx context.Context, id string) error { return nil }

// RBAC: Permissions
func (m *mockMetadataStore) CreatePermission(ctx context.Context, perm *store.PermissionRecord) (*store.PermissionRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetPermission(ctx context.Context, id string) (*store.PermissionRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListPermissions(ctx context.Context, limit, offset int) ([]*store.PermissionRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeletePermission(ctx context.Context, id string) error { return nil }

// RBAC: Role ↔ Permission
func (m *mockMetadataStore) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	return nil
}
func (m *mockMetadataStore) RevokePermissionFromRole(ctx context.Context, roleID, permissionID string) error {
	return nil
}
func (m *mockMetadataStore) ListRolePermissions(ctx context.Context, roleID string) ([]*store.PermissionRecord, error) {
	return nil, nil
}

// RBAC: Role Assignments
func (m *mockMetadataStore) CreateRoleAssignment(ctx context.Context, ra *store.RoleAssignmentRecord) (*store.RoleAssignmentRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetRoleAssignment(ctx context.Context, id string) (*store.RoleAssignmentRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListRoleAssignments(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListRoleAssignmentsByPrincipal(ctx context.Context, tenantID string, principalType domain.PrincipalType, principalID string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteRoleAssignment(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) ResolveEffectivePermissions(ctx context.Context, tenantID, subject string) ([]string, error) {
	return nil, nil
}

// Menu permissions
func (m *mockMetadataStore) ListTenantMenuPermissions(ctx context.Context, tenantID string) ([]*store.MenuPermissionRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpsertTenantMenuPermission(ctx context.Context, tenantID, menuKey string, enabled bool) (*store.MenuPermissionRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteTenantMenuPermission(ctx context.Context, tenantID, menuKey string) error {
	return nil
}
func (m *mockMetadataStore) SeedDefaultMenuPermissions(ctx context.Context, tenantID string) error {
	return nil
}

// Button permissions
func (m *mockMetadataStore) ListTenantButtonPermissions(ctx context.Context, tenantID string) ([]*store.ButtonPermissionRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpsertTenantButtonPermission(ctx context.Context, tenantID, permissionKey string, enabled bool) (*store.ButtonPermissionRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteTenantButtonPermission(ctx context.Context, tenantID, permissionKey string) error {
	return nil
}
func (m *mockMetadataStore) SeedDefaultButtonPermissions(ctx context.Context, tenantID string) error {
	return nil
}

// API Doc Shares
func (m *mockMetadataStore) SaveAPIDocShare(ctx context.Context, share *store.APIDocShare) error {
	return nil
}
func (m *mockMetadataStore) GetAPIDocShareByToken(ctx context.Context, token string) (*store.APIDocShare, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListAPIDocShares(ctx context.Context, tenantID, namespace string, limit, offset int) ([]*store.APIDocShare, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteAPIDocShare(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) IncrementAPIDocShareAccess(ctx context.Context, token string) error {
	return nil
}

// Function Docs
func (m *mockMetadataStore) SaveFunctionDoc(ctx context.Context, doc *store.FunctionDoc) error {
	return nil
}
func (m *mockMetadataStore) GetFunctionDoc(ctx context.Context, functionName string) (*store.FunctionDoc, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteFunctionDoc(ctx context.Context, functionName string) error {
	return nil
}

// Workflow Docs
func (m *mockMetadataStore) SaveWorkflowDoc(ctx context.Context, doc *store.WorkflowDoc) error {
	return nil
}
func (m *mockMetadataStore) GetWorkflowDoc(ctx context.Context, workflowName string) (*store.WorkflowDoc, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteWorkflowDoc(ctx context.Context, workflowName string) error {
	return nil
}

// Test Suites
func (m *mockMetadataStore) SaveTestSuite(ctx context.Context, ts *store.TestSuite) error {
	return nil
}
func (m *mockMetadataStore) GetTestSuite(ctx context.Context, functionName string) (*store.TestSuite, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteTestSuite(ctx context.Context, functionName string) error {
	return nil
}

// Database Access
func (m *mockMetadataStore) CreateDbResource(ctx context.Context, res *store.DbResourceRecord) (*store.DbResourceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetDbResource(ctx context.Context, id string) (*store.DbResourceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetDbResourceByName(ctx context.Context, name string) (*store.DbResourceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListDbResources(ctx context.Context, limit, offset int) ([]*store.DbResourceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateDbResource(ctx context.Context, id string, update *store.DbResourceUpdate) (*store.DbResourceRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteDbResource(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) CreateDbBinding(ctx context.Context, binding *store.DbBindingRecord) (*store.DbBindingRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetDbBinding(ctx context.Context, id string) (*store.DbBindingRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListDbBindings(ctx context.Context, dbResourceID string, limit, offset int) ([]*store.DbBindingRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListDbBindingsByFunction(ctx context.Context, functionID string, limit, offset int) ([]*store.DbBindingRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateDbBinding(ctx context.Context, id string, update *store.DbBindingUpdate) (*store.DbBindingRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteDbBinding(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) CreateCredentialPolicy(ctx context.Context, policy *store.CredentialPolicyRecord) (*store.CredentialPolicyRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) GetCredentialPolicy(ctx context.Context, dbResourceID string) (*store.CredentialPolicyRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateCredentialPolicy(ctx context.Context, dbResourceID string, update *store.CredentialPolicyUpdate) (*store.CredentialPolicyRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) DeleteCredentialPolicy(ctx context.Context, dbResourceID string) error {
	return nil
}
func (m *mockMetadataStore) SaveDbRequestLog(ctx context.Context, log *domain.DbRequestLog) error {
	return nil
}
func (m *mockMetadataStore) ListDbRequestLogs(ctx context.Context, dbResourceID string, limit, offset int) ([]*domain.DbRequestLog, error) {
	return nil, nil
}

// Cluster nodes
func (m *mockMetadataStore) UpsertClusterNode(ctx context.Context, node *store.ClusterNodeRecord) error {
	return nil
}
func (m *mockMetadataStore) GetClusterNode(ctx context.Context, id string) (*store.ClusterNodeRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) ListClusterNodes(ctx context.Context, limit, offset int) ([]*store.ClusterNodeRecord, error) {
	return nil, nil
}
func (m *mockMetadataStore) UpdateClusterNodeHeartbeat(ctx context.Context, id string, activeVMs, queueDepth int) error {
	return nil
}
func (m *mockMetadataStore) DeleteClusterNode(ctx context.Context, id string) error { return nil }
func (m *mockMetadataStore) ListActiveClusterNodes(ctx context.Context) ([]*store.ClusterNodeRecord, error) {
	return nil, nil
}

// invocationPaginationStore methods
func (m *mockMetadataStore) CountInvocationLogs(ctx context.Context, functionID string) (int64, error) {
	if m.countInvocationLogsFn != nil {
		return m.countInvocationLogsFn(ctx, functionID)
	}
	return 0, nil
}
func (m *mockMetadataStore) CountAllInvocationLogs(ctx context.Context) (int64, error) {
	if m.countAllInvocationLogsFn != nil {
		return m.countAllInvocationLogsFn(ctx)
	}
	return 0, nil
}
func (m *mockMetadataStore) ListAllInvocationLogsFiltered(ctx context.Context, limit, offset int, search, functionName string, success *bool) ([]*store.InvocationLog, error) {
	if m.listAllInvocationLogsFilteredFn != nil {
		return m.listAllInvocationLogsFilteredFn(ctx, limit, offset, search, functionName, success)
	}
	return nil, nil
}
func (m *mockMetadataStore) CountAllInvocationLogsFiltered(ctx context.Context, search, functionName string, success *bool) (int64, error) {
	if m.countAllInvocationLogsFilteredFn != nil {
		return m.countAllInvocationLogsFilteredFn(ctx, search, functionName, success)
	}
	return 0, nil
}
func (m *mockMetadataStore) GetAllInvocationLogsSummary(ctx context.Context) (*store.InvocationLogSummary, error) {
	if m.getAllInvocationLogsSummaryFn != nil {
		return m.getAllInvocationLogsSummaryFn(ctx)
	}
	return &store.InvocationLogSummary{}, nil
}
func (m *mockMetadataStore) GetAllInvocationLogsSummaryFiltered(ctx context.Context, search, functionName string, success *bool) (*store.InvocationLogSummary, error) {
	if m.getAllInvocationLogsSummaryFilteredFn != nil {
		return m.getAllInvocationLogsSummaryFilteredFn(ctx, search, functionName, success)
	}
	return &store.InvocationLogSummary{}, nil
}

func (m *mockMetadataStore) RecordPoolMetrics(ctx context.Context, snap store.PoolMetricsSnapshot) error {
	return nil
}

func (m *mockMetadataStore) PrunePoolMetrics(ctx context.Context, retentionSeconds int) error {
	return nil
}

// mockWorkflowStore implements store.WorkflowStore for the mockMetadataStore to
// also satisfy the GetWorkflowByName requirement in ListWorkflowAsyncInvocations.
type mockWorkflowStore struct {
	getWorkflowByNameFn func(ctx context.Context, name string) (*domain.Workflow, error)
}

var _ store.WorkflowStore = (*mockWorkflowStore)(nil)

func (w *mockWorkflowStore) CreateWorkflow(ctx context.Context, wf *domain.Workflow) error {
	return nil
}
func (w *mockWorkflowStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	return nil, nil
}
func (w *mockWorkflowStore) GetWorkflowByName(ctx context.Context, name string) (*domain.Workflow, error) {
	if w.getWorkflowByNameFn != nil {
		return w.getWorkflowByNameFn(ctx, name)
	}
	return nil, fmt.Errorf("workflow not found")
}
func (w *mockWorkflowStore) ListWorkflows(ctx context.Context, limit, offset int) ([]*domain.Workflow, error) {
	return nil, nil
}
func (w *mockWorkflowStore) DeleteWorkflow(ctx context.Context, id string) error { return nil }
func (w *mockWorkflowStore) UpdateWorkflowVersion(ctx context.Context, id string, version int) error {
	return nil
}
func (w *mockWorkflowStore) CreateWorkflowVersion(ctx context.Context, v *domain.WorkflowVersion) error {
	return nil
}
func (w *mockWorkflowStore) GetWorkflowVersion(ctx context.Context, id string) (*domain.WorkflowVersion, error) {
	return nil, nil
}
func (w *mockWorkflowStore) GetWorkflowVersionByNumber(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error) {
	return nil, nil
}
func (w *mockWorkflowStore) ListWorkflowVersions(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowVersion, error) {
	return nil, nil
}
func (w *mockWorkflowStore) CreateWorkflowNodes(ctx context.Context, nodes []domain.WorkflowNode) error {
	return nil
}
func (w *mockWorkflowStore) CreateWorkflowEdges(ctx context.Context, edges []domain.WorkflowEdge) error {
	return nil
}
func (w *mockWorkflowStore) GetWorkflowNodes(ctx context.Context, versionID string) ([]domain.WorkflowNode, error) {
	return nil, nil
}
func (w *mockWorkflowStore) GetWorkflowNodeByID(ctx context.Context, nodeID string) (*domain.WorkflowNode, error) {
	return nil, nil
}
func (w *mockWorkflowStore) GetWorkflowEdges(ctx context.Context, versionID string) ([]domain.WorkflowEdge, error) {
	return nil, nil
}
func (w *mockWorkflowStore) CreateRun(ctx context.Context, run *domain.WorkflowRun) error { return nil }
func (w *mockWorkflowStore) GetRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	return nil, nil
}
func (w *mockWorkflowStore) ListRuns(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowRun, error) {
	return nil, nil
}
func (w *mockWorkflowStore) UpdateRunStatus(ctx context.Context, id string, status domain.RunStatus, errMsg string, output json.RawMessage) error {
	return nil
}
func (w *mockWorkflowStore) CreateRunNodes(ctx context.Context, nodes []domain.RunNode) error {
	return nil
}
func (w *mockWorkflowStore) GetRunNodes(ctx context.Context, runID string) ([]domain.RunNode, error) {
	return nil, nil
}
func (w *mockWorkflowStore) AcquireReadyNode(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (*domain.RunNode, error) {
	return nil, nil
}
func (w *mockWorkflowStore) UpdateRunNode(ctx context.Context, node *domain.RunNode) error {
	return nil
}
func (w *mockWorkflowStore) DecrementDeps(ctx context.Context, runID string, nodeKeys []string) error {
	return nil
}
func (w *mockWorkflowStore) CreateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	return nil
}
func (w *mockWorkflowStore) UpdateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	return nil
}
func (w *mockWorkflowStore) GetNodeAttempts(ctx context.Context, runNodeID string) ([]domain.NodeAttempt, error) {
	return nil, nil
}
