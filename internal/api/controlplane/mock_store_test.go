package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// Compile-time interface check.
var _ store.MetadataStore = (*mockMetadataStore)(nil)

type mockMetadataStore struct {
	closeFn                                 func() error
	pingFn                                  func(ctx context.Context) error
	saveFunctionFn                          func(ctx context.Context, fn *domain.Function) error
	getFunctionFn                           func(ctx context.Context, id string) (*domain.Function, error)
	getFunctionByNameFn                     func(ctx context.Context, name string) (*domain.Function, error)
	deleteFunctionFn                        func(ctx context.Context, id string) error
	listFunctionsFn                         func(ctx context.Context, limit, offset int) ([]*domain.Function, error)
	searchFunctionsFn                       func(ctx context.Context, query string, limit, offset int) ([]*domain.Function, error)
	updateFunctionFn                        func(ctx context.Context, name string, update *store.FunctionUpdate) (*domain.Function, error)
	listTenantsFn                           func(ctx context.Context, limit, offset int) ([]*store.TenantRecord, error)
	getTenantFn                             func(ctx context.Context, id string) (*store.TenantRecord, error)
	createTenantFn                          func(ctx context.Context, tenant *store.TenantRecord) (*store.TenantRecord, error)
	updateTenantFn                          func(ctx context.Context, id string, update *store.TenantUpdate) (*store.TenantRecord, error)
	deleteTenantFn                          func(ctx context.Context, id string) error
	setTenantPasswordHashFn                 func(ctx context.Context, id, passwordHash string) error
	listNamespacesFn                        func(ctx context.Context, tenantID string, limit, offset int) ([]*store.NamespaceRecord, error)
	getNamespaceFn                          func(ctx context.Context, tenantID, name string) (*store.NamespaceRecord, error)
	createNamespaceFn                       func(ctx context.Context, namespace *store.NamespaceRecord) (*store.NamespaceRecord, error)
	updateNamespaceFn                       func(ctx context.Context, tenantID, name string, update *store.NamespaceUpdate) (*store.NamespaceRecord, error)
	deleteNamespaceFn                       func(ctx context.Context, tenantID, name string) error
	listTenantQuotasFn                      func(ctx context.Context, tenantID string) ([]*store.TenantQuotaRecord, error)
	upsertTenantQuotaFn                     func(ctx context.Context, quota *store.TenantQuotaRecord) (*store.TenantQuotaRecord, error)
	deleteTenantQuotaFn                     func(ctx context.Context, tenantID, dimension string) error
	listTenantUsageFn                       func(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error)
	refreshTenantUsageFn                    func(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error)
	checkAndConsumeTenantQuotaFn            func(ctx context.Context, tenantID, dimension string, amount int64) (*store.TenantQuotaDecision, error)
	checkTenantAbsoluteQuotaFn              func(ctx context.Context, tenantID, dimension string, value int64) (*store.TenantQuotaDecision, error)
	getTenantFunctionCountFn                func(ctx context.Context, tenantID string) (int64, error)
	getTenantAsyncQueueDepthFn              func(ctx context.Context, tenantID string) (int64, error)
	publishVersionFn                        func(ctx context.Context, funcID string, version *domain.FunctionVersion) error
	getVersionFn                            func(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error)
	listVersionsFn                          func(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionVersion, error)
	deleteVersionFn                         func(ctx context.Context, funcID string, version int) error
	setAliasFn                              func(ctx context.Context, alias *domain.FunctionAlias) error
	getAliasFn                              func(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error)
	listAliasesFn                           func(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionAlias, error)
	deleteAliasFn                           func(ctx context.Context, funcID, aliasName string) error
	saveInvocationLogFn                     func(ctx context.Context, log *store.InvocationLog) error
	saveInvocationLogsFn                    func(ctx context.Context, logs []*store.InvocationLog) error
	listInvocationLogsFn                    func(ctx context.Context, functionID string, limit, offset int) ([]*store.InvocationLog, error)
	listAllInvocationLogsFn                 func(ctx context.Context, limit, offset int) ([]*store.InvocationLog, error)
	getInvocationLogFn                      func(ctx context.Context, requestID string) (*store.InvocationLog, error)
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
	acquireDueAsyncInvocationFn             func(ctx context.Context, workerID string, leaseDuration time.Duration) (*store.AsyncInvocation, error)
	acquireDueAsyncInvocationsFn            func(ctx context.Context, workerID string, leaseDuration time.Duration, batchSize int) ([]*store.AsyncInvocation, error)
	markAsyncInvocationSucceededFn          func(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	markAsyncInvocationForRetryFn           func(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	markAsyncInvocationDLQFn                func(ctx context.Context, id, lastError string) error
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
	enqueueAsyncInvocationWithIdempotencyFn func(ctx context.Context, inv *store.AsyncInvocation, idempotencyKey string, ttl time.Duration) (*store.AsyncInvocation, bool, error)
	createEventTopicFn                      func(ctx context.Context, topic *store.EventTopic) error
	getEventTopicFn                         func(ctx context.Context, id string) (*store.EventTopic, error)
	getEventTopicByNameFn                   func(ctx context.Context, name string) (*store.EventTopic, error)
	listEventTopicsFn                       func(ctx context.Context, limit, offset int) ([]*store.EventTopic, error)
	deleteEventTopicByNameFn                func(ctx context.Context, name string) error
	createEventSubscriptionFn               func(ctx context.Context, sub *store.EventSubscription) error
	getEventSubscriptionFn                  func(ctx context.Context, id string) (*store.EventSubscription, error)
	listEventSubscriptionsFn                func(ctx context.Context, topicID string, limit, offset int) ([]*store.EventSubscription, error)
	updateEventSubscriptionFn               func(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error)
	deleteEventSubscriptionFn               func(ctx context.Context, id string) error
	publishEventFn                          func(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, error)
	listEventMessagesFn                     func(ctx context.Context, topicID string, limit, offset int) ([]*store.EventMessage, error)
	publishEventFromOutboxFn                func(ctx context.Context, outboxID, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, bool, error)
	getEventDeliveryFn                      func(ctx context.Context, id string) (*store.EventDelivery, error)
	listEventDeliveriesFn                   func(ctx context.Context, subscriptionID string, limit, offset int, statuses []store.EventDeliveryStatus) ([]*store.EventDelivery, error)
	acquireDueEventDeliveryFn               func(ctx context.Context, workerID string, leaseDuration time.Duration) (*store.EventDelivery, error)
	markEventDeliverySucceededFn            func(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	markEventDeliveryForRetryFn             func(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	markEventDeliveryDLQFn                  func(ctx context.Context, id, lastError string) error
	requeueEventDeliveryFn                  func(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error)
	resolveEventReplaySequenceByTimeFn      func(ctx context.Context, subscriptionID string, from time.Time) (int64, error)
	setEventSubscriptionCursorFn            func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error)
	replayEventSubscriptionFn               func(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error)
	createEventOutboxFn                     func(ctx context.Context, outbox *store.EventOutbox) error
	getEventOutboxFn                        func(ctx context.Context, id string) (*store.EventOutbox, error)
	listEventOutboxFn                       func(ctx context.Context, topicID string, limit, offset int, statuses []store.EventOutboxStatus) ([]*store.EventOutbox, error)
	acquireDueEventOutboxFn                 func(ctx context.Context, workerID string, leaseDuration time.Duration) (*store.EventOutbox, error)
	markEventOutboxPublishedFn              func(ctx context.Context, id, messageID string) error
	markEventOutboxForRetryFn               func(ctx context.Context, id, lastError string, nextRunAt time.Time) error
	markEventOutboxFailedFn                 func(ctx context.Context, id, lastError string) error
	requeueEventOutboxFn                    func(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error)
	prepareEventInboxFn                     func(ctx context.Context, subscriptionID, messageID, deliveryID string) (*store.EventInboxRecord, bool, error)
	markEventInboxSucceededFn               func(ctx context.Context, subscriptionID, messageID, deliveryID, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error
	markEventInboxFailedFn                  func(ctx context.Context, subscriptionID, messageID, deliveryID, lastError string) error
	createNotificationFn                    func(ctx context.Context, n *store.NotificationRecord) error
	listNotificationsFn                     func(ctx context.Context, limit, offset int, status store.NotificationStatus) ([]*store.NotificationRecord, error)
	getUnreadNotificationCountFn            func(ctx context.Context) (int64, error)
	markNotificationReadFn                  func(ctx context.Context, id string) (*store.NotificationRecord, error)
	markAllNotificationsReadFn              func(ctx context.Context) (int64, error)
	saveRuntimeFn                           func(ctx context.Context, rt *store.RuntimeRecord) error
	getRuntimeFn                            func(ctx context.Context, id string) (*store.RuntimeRecord, error)
	listRuntimesFn                          func(ctx context.Context, limit, offset int) ([]*store.RuntimeRecord, error)
	deleteRuntimeFn                         func(ctx context.Context, id string) error
	getConfigFn                             func(ctx context.Context) (map[string]string, error)
	setConfigFn                             func(ctx context.Context, key, value string) error
	saveAPIKeyFn                            func(ctx context.Context, key *store.APIKeyRecord) error
	getAPIKeyByHashFn                       func(ctx context.Context, keyHash string) (*store.APIKeyRecord, error)
	getAPIKeyByNameFn                       func(ctx context.Context, name string) (*store.APIKeyRecord, error)
	listAPIKeysFn                           func(ctx context.Context, limit, offset int) ([]*store.APIKeyRecord, error)
	deleteAPIKeyFn                          func(ctx context.Context, name string) error
	saveSecretFn                            func(ctx context.Context, name, encryptedValue string) error
	getSecretFn                             func(ctx context.Context, name string) (string, error)
	deleteSecretFn                          func(ctx context.Context, name string) error
	listSecretsFn                           func(ctx context.Context) (map[string]string, error)
	secretExistsFn                          func(ctx context.Context, name string) (bool, error)
	checkRateLimitFn                        func(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error)
	saveFunctionCodeFn                      func(ctx context.Context, funcID, sourceCode, sourceHash string) error
	getFunctionCodeFn                       func(ctx context.Context, funcID string) (*domain.FunctionCode, error)
	updateFunctionCodeFn                    func(ctx context.Context, funcID, sourceCode, sourceHash string) error
	updateCompileResultFn                   func(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error
	deleteFunctionCodeFn                    func(ctx context.Context, funcID string) error
	saveFunctionFilesFn                     func(ctx context.Context, funcID string, files map[string][]byte) error
	getFunctionFilesFn                      func(ctx context.Context, funcID string) (map[string][]byte, error)
	listFunctionFilesFn                     func(ctx context.Context, funcID string) ([]store.FunctionFileInfo, error)
	deleteFunctionFilesFn                   func(ctx context.Context, funcID string) error
	hasFunctionFilesFn                      func(ctx context.Context, funcID string) (bool, error)
	saveGatewayRouteFn                      func(ctx context.Context, route *domain.GatewayRoute) error
	getGatewayRouteFn                       func(ctx context.Context, id string) (*domain.GatewayRoute, error)
	getRouteByDomainPathFn                  func(ctx context.Context, domain, path string) (*domain.GatewayRoute, error)
	listGatewayRoutesFn                     func(ctx context.Context, limit, offset int) ([]*domain.GatewayRoute, error)
	listRoutesByDomainFn                    func(ctx context.Context, domain string, limit, offset int) ([]*domain.GatewayRoute, error)
	deleteGatewayRouteFn                    func(ctx context.Context, id string) error
	updateGatewayRouteFn                    func(ctx context.Context, id string, route *domain.GatewayRoute) error
	saveLayerFn                             func(ctx context.Context, layer *domain.Layer) error
	getLayerFn                              func(ctx context.Context, id string) (*domain.Layer, error)
	getLayerByNameFn                        func(ctx context.Context, name string) (*domain.Layer, error)
	getLayerByContentHashFn                 func(ctx context.Context, hash string) (*domain.Layer, error)
	listLayersFn                            func(ctx context.Context, limit, offset int) ([]*domain.Layer, error)
	deleteLayerFn                           func(ctx context.Context, id string) error
	setFunctionLayersFn                     func(ctx context.Context, funcID string, layerIDs []string) error
	getFunctionLayersFn                     func(ctx context.Context, funcID string) ([]*domain.Layer, error)
	listFunctionsByLayerFn                  func(ctx context.Context, layerID string) ([]string, error)
	createTriggerFn                         func(ctx context.Context, trigger *store.TriggerRecord) error
	getTriggerFn                            func(ctx context.Context, id string) (*store.TriggerRecord, error)
	getTriggerByNameFn                      func(ctx context.Context, name string) (*store.TriggerRecord, error)
	listTriggersFn                          func(ctx context.Context, limit, offset int) ([]*store.TriggerRecord, error)
	updateTriggerFn                         func(ctx context.Context, id string, update *store.TriggerUpdate) (*store.TriggerRecord, error)
	deleteTriggerFn                         func(ctx context.Context, id string) error
	createVolumeFn                          func(ctx context.Context, vol *domain.Volume) error
	getVolumeFn                             func(ctx context.Context, id string) (*domain.Volume, error)
	getVolumeByNameFn                       func(ctx context.Context, name string) (*domain.Volume, error)
	listVolumesFn                           func(ctx context.Context) ([]*domain.Volume, error)
	updateVolumeFn                          func(ctx context.Context, id string, updates map[string]interface{}) error
	deleteVolumeFn                          func(ctx context.Context, id string) error
	getFunctionVolumesFn                    func(ctx context.Context, functionID string) ([]*domain.Volume, error)
	createRoleFn                            func(ctx context.Context, role *store.RoleRecord) (*store.RoleRecord, error)
	getRoleFn                               func(ctx context.Context, id string) (*store.RoleRecord, error)
	listRolesFn                             func(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleRecord, error)
	deleteRoleFn                            func(ctx context.Context, id string) error
	createPermissionFn                      func(ctx context.Context, perm *store.PermissionRecord) (*store.PermissionRecord, error)
	getPermissionFn                         func(ctx context.Context, id string) (*store.PermissionRecord, error)
	listPermissionsFn                       func(ctx context.Context, limit, offset int) ([]*store.PermissionRecord, error)
	deletePermissionFn                      func(ctx context.Context, id string) error
	assignPermissionToRoleFn                func(ctx context.Context, roleID, permissionID string) error
	revokePermissionFromRoleFn              func(ctx context.Context, roleID, permissionID string) error
	listRolePermissionsFn                   func(ctx context.Context, roleID string) ([]*store.PermissionRecord, error)
	createRoleAssignmentFn                  func(ctx context.Context, ra *store.RoleAssignmentRecord) (*store.RoleAssignmentRecord, error)
	getRoleAssignmentFn                     func(ctx context.Context, id string) (*store.RoleAssignmentRecord, error)
	listRoleAssignmentsFn                   func(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleAssignmentRecord, error)
	listRoleAssignmentsByPrincipalFn        func(ctx context.Context, tenantID string, principalType domain.PrincipalType, principalID string, limit, offset int) ([]*store.RoleAssignmentRecord, error)
	deleteRoleAssignmentFn                  func(ctx context.Context, id string) error
	resolveEffectivePermissionsFn           func(ctx context.Context, tenantID, subject string) ([]string, error)
	listTenantMenuPermissionsFn             func(ctx context.Context, tenantID string) ([]*store.MenuPermissionRecord, error)
	upsertTenantMenuPermissionFn            func(ctx context.Context, tenantID, menuKey string, enabled bool) (*store.MenuPermissionRecord, error)
	deleteTenantMenuPermissionFn            func(ctx context.Context, tenantID, menuKey string) error
	seedDefaultMenuPermissionsFn            func(ctx context.Context, tenantID string) error
	listTenantButtonPermissionsFn           func(ctx context.Context, tenantID string) ([]*store.ButtonPermissionRecord, error)
	upsertTenantButtonPermissionFn          func(ctx context.Context, tenantID, permissionKey string, enabled bool) (*store.ButtonPermissionRecord, error)
	deleteTenantButtonPermissionFn          func(ctx context.Context, tenantID, permissionKey string) error
	seedDefaultButtonPermissionsFn          func(ctx context.Context, tenantID string) error
	saveAPIDocShareFn                       func(ctx context.Context, share *store.APIDocShare) error
	getAPIDocShareByTokenFn                 func(ctx context.Context, token string) (*store.APIDocShare, error)
	listAPIDocSharesFn                      func(ctx context.Context, tenantID, namespace string, limit, offset int) ([]*store.APIDocShare, error)
	deleteAPIDocShareFn                     func(ctx context.Context, id string) error
	incrementAPIDocShareAccessFn            func(ctx context.Context, token string) error
	saveFunctionDocFn                       func(ctx context.Context, doc *store.FunctionDoc) error
	getFunctionDocFn                        func(ctx context.Context, functionName string) (*store.FunctionDoc, error)
	deleteFunctionDocFn                     func(ctx context.Context, functionName string) error
	saveWorkflowDocFn                       func(ctx context.Context, doc *store.WorkflowDoc) error
	getWorkflowDocFn                        func(ctx context.Context, workflowName string) (*store.WorkflowDoc, error)
	deleteWorkflowDocFn                     func(ctx context.Context, workflowName string) error
	saveTestSuiteFn                         func(ctx context.Context, ts *store.TestSuite) error
	getTestSuiteFn                          func(ctx context.Context, functionName string) (*store.TestSuite, error)
	deleteTestSuiteFn                       func(ctx context.Context, functionName string) error
	createDbResourceFn                      func(ctx context.Context, res *store.DbResourceRecord) (*store.DbResourceRecord, error)
	getDbResourceFn                         func(ctx context.Context, id string) (*store.DbResourceRecord, error)
	getDbResourceByNameFn                   func(ctx context.Context, name string) (*store.DbResourceRecord, error)
	listDbResourcesFn                       func(ctx context.Context, limit, offset int) ([]*store.DbResourceRecord, error)
	updateDbResourceFn                      func(ctx context.Context, id string, update *store.DbResourceUpdate) (*store.DbResourceRecord, error)
	deleteDbResourceFn                      func(ctx context.Context, id string) error
	createDbBindingFn                       func(ctx context.Context, binding *store.DbBindingRecord) (*store.DbBindingRecord, error)
	getDbBindingFn                          func(ctx context.Context, id string) (*store.DbBindingRecord, error)
	listDbBindingsFn                        func(ctx context.Context, dbResourceID string, limit, offset int) ([]*store.DbBindingRecord, error)
	listDbBindingsByFunctionFn              func(ctx context.Context, functionID string, limit, offset int) ([]*store.DbBindingRecord, error)
	updateDbBindingFn                       func(ctx context.Context, id string, update *store.DbBindingUpdate) (*store.DbBindingRecord, error)
	deleteDbBindingFn                       func(ctx context.Context, id string) error
	createCredentialPolicyFn                func(ctx context.Context, policy *store.CredentialPolicyRecord) (*store.CredentialPolicyRecord, error)
	getCredentialPolicyFn                   func(ctx context.Context, dbResourceID string) (*store.CredentialPolicyRecord, error)
	updateCredentialPolicyFn                func(ctx context.Context, dbResourceID string, update *store.CredentialPolicyUpdate) (*store.CredentialPolicyRecord, error)
	deleteCredentialPolicyFn                func(ctx context.Context, dbResourceID string) error
	saveDbRequestLogFn                      func(ctx context.Context, log *domain.DbRequestLog) error
	listDbRequestLogsFn                     func(ctx context.Context, dbResourceID string, limit, offset int) ([]*domain.DbRequestLog, error)
	upsertClusterNodeFn                     func(ctx context.Context, node *store.ClusterNodeRecord) error
	getClusterNodeFn                        func(ctx context.Context, id string) (*store.ClusterNodeRecord, error)
	listClusterNodesFn                      func(ctx context.Context, limit, offset int) ([]*store.ClusterNodeRecord, error)
	updateClusterNodeHeartbeatFn            func(ctx context.Context, id string, activeVMs, queueDepth int) error
	deleteClusterNodeFn                     func(ctx context.Context, id string) error
	listActiveClusterNodesFn                func(ctx context.Context) ([]*store.ClusterNodeRecord, error)
}

// --- Method implementations ---

func (m *mockMetadataStore) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func (m *mockMetadataStore) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
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
	return nil, nil
}

func (m *mockMetadataStore) GetFunctionByName(ctx context.Context, name string) (*domain.Function, error) {
	if m.getFunctionByNameFn != nil {
		return m.getFunctionByNameFn(ctx, name)
	}
	return nil, nil
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

func (m *mockMetadataStore) ListTenants(ctx context.Context, limit, offset int) ([]*store.TenantRecord, error) {
	if m.listTenantsFn != nil {
		return m.listTenantsFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetTenant(ctx context.Context, id string) (*store.TenantRecord, error) {
	if m.getTenantFn != nil {
		return m.getTenantFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) CreateTenant(ctx context.Context, tenant *store.TenantRecord) (*store.TenantRecord, error) {
	if m.createTenantFn != nil {
		return m.createTenantFn(ctx, tenant)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateTenant(ctx context.Context, id string, update *store.TenantUpdate) (*store.TenantRecord, error) {
	if m.updateTenantFn != nil {
		return m.updateTenantFn(ctx, id, update)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteTenant(ctx context.Context, id string) error {
	if m.deleteTenantFn != nil {
		return m.deleteTenantFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) SetTenantPasswordHash(ctx context.Context, id, passwordHash string) error {
	if m.setTenantPasswordHashFn != nil {
		return m.setTenantPasswordHashFn(ctx, id, passwordHash)
	}
	return nil
}

func (m *mockMetadataStore) ListNamespaces(ctx context.Context, tenantID string, limit, offset int) ([]*store.NamespaceRecord, error) {
	if m.listNamespacesFn != nil {
		return m.listNamespacesFn(ctx, tenantID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetNamespace(ctx context.Context, tenantID, name string) (*store.NamespaceRecord, error) {
	if m.getNamespaceFn != nil {
		return m.getNamespaceFn(ctx, tenantID, name)
	}
	return nil, nil
}

func (m *mockMetadataStore) CreateNamespace(ctx context.Context, namespace *store.NamespaceRecord) (*store.NamespaceRecord, error) {
	if m.createNamespaceFn != nil {
		return m.createNamespaceFn(ctx, namespace)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateNamespace(ctx context.Context, tenantID, name string, update *store.NamespaceUpdate) (*store.NamespaceRecord, error) {
	if m.updateNamespaceFn != nil {
		return m.updateNamespaceFn(ctx, tenantID, name, update)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteNamespace(ctx context.Context, tenantID, name string) error {
	if m.deleteNamespaceFn != nil {
		return m.deleteNamespaceFn(ctx, tenantID, name)
	}
	return nil
}

func (m *mockMetadataStore) ListTenantQuotas(ctx context.Context, tenantID string) ([]*store.TenantQuotaRecord, error) {
	if m.listTenantQuotasFn != nil {
		return m.listTenantQuotasFn(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpsertTenantQuota(ctx context.Context, quota *store.TenantQuotaRecord) (*store.TenantQuotaRecord, error) {
	if m.upsertTenantQuotaFn != nil {
		return m.upsertTenantQuotaFn(ctx, quota)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteTenantQuota(ctx context.Context, tenantID, dimension string) error {
	if m.deleteTenantQuotaFn != nil {
		return m.deleteTenantQuotaFn(ctx, tenantID, dimension)
	}
	return nil
}

func (m *mockMetadataStore) ListTenantUsage(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error) {
	if m.listTenantUsageFn != nil {
		return m.listTenantUsageFn(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockMetadataStore) RefreshTenantUsage(ctx context.Context, tenantID string) ([]*store.TenantUsageRecord, error) {
	if m.refreshTenantUsageFn != nil {
		return m.refreshTenantUsageFn(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockMetadataStore) CheckAndConsumeTenantQuota(ctx context.Context, tenantID, dimension string, amount int64) (*store.TenantQuotaDecision, error) {
	if m.checkAndConsumeTenantQuotaFn != nil {
		return m.checkAndConsumeTenantQuotaFn(ctx, tenantID, dimension, amount)
	}
	return nil, nil
}

func (m *mockMetadataStore) CheckTenantAbsoluteQuota(ctx context.Context, tenantID, dimension string, value int64) (*store.TenantQuotaDecision, error) {
	if m.checkTenantAbsoluteQuotaFn != nil {
		return m.checkTenantAbsoluteQuotaFn(ctx, tenantID, dimension, value)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetTenantFunctionCount(ctx context.Context, tenantID string) (int64, error) {
	if m.getTenantFunctionCountFn != nil {
		return m.getTenantFunctionCountFn(ctx, tenantID)
	}
	return 0, nil
}

func (m *mockMetadataStore) GetTenantAsyncQueueDepth(ctx context.Context, tenantID string) (int64, error) {
	if m.getTenantAsyncQueueDepthFn != nil {
		return m.getTenantAsyncQueueDepthFn(ctx, tenantID)
	}
	return 0, nil
}

func (m *mockMetadataStore) PublishVersion(ctx context.Context, funcID string, version *domain.FunctionVersion) error {
	if m.publishVersionFn != nil {
		return m.publishVersionFn(ctx, funcID, version)
	}
	return nil
}

func (m *mockMetadataStore) GetVersion(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error) {
	if m.getVersionFn != nil {
		return m.getVersionFn(ctx, funcID, version)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListVersions(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionVersion, error) {
	if m.listVersionsFn != nil {
		return m.listVersionsFn(ctx, funcID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteVersion(ctx context.Context, funcID string, version int) error {
	if m.deleteVersionFn != nil {
		return m.deleteVersionFn(ctx, funcID, version)
	}
	return nil
}

func (m *mockMetadataStore) SetAlias(ctx context.Context, alias *domain.FunctionAlias) error {
	if m.setAliasFn != nil {
		return m.setAliasFn(ctx, alias)
	}
	return nil
}

func (m *mockMetadataStore) GetAlias(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error) {
	if m.getAliasFn != nil {
		return m.getAliasFn(ctx, funcID, aliasName)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListAliases(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionAlias, error) {
	if m.listAliasesFn != nil {
		return m.listAliasesFn(ctx, funcID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteAlias(ctx context.Context, funcID, aliasName string) error {
	if m.deleteAliasFn != nil {
		return m.deleteAliasFn(ctx, funcID, aliasName)
	}
	return nil
}

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
	return nil, nil
}

func (m *mockMetadataStore) GetFunctionState(ctx context.Context, functionID, key string) (*store.FunctionStateEntry, error) {
	if m.getFunctionStateFn != nil {
		return m.getFunctionStateFn(ctx, functionID, key)
	}
	return nil, nil
}

func (m *mockMetadataStore) PutFunctionState(ctx context.Context, functionID, key string, value json.RawMessage, opts *store.FunctionStatePutOptions) (*store.FunctionStateEntry, error) {
	if m.putFunctionStateFn != nil {
		return m.putFunctionStateFn(ctx, functionID, key, value, opts)
	}
	return nil, nil
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
	return nil, nil
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

func (m *mockMetadataStore) AcquireDueAsyncInvocation(ctx context.Context, workerID string, leaseDuration time.Duration) (*store.AsyncInvocation, error) {
	if m.acquireDueAsyncInvocationFn != nil {
		return m.acquireDueAsyncInvocationFn(ctx, workerID, leaseDuration)
	}
	return nil, nil
}

func (m *mockMetadataStore) AcquireDueAsyncInvocations(ctx context.Context, workerID string, leaseDuration time.Duration, batchSize int) ([]*store.AsyncInvocation, error) {
	if m.acquireDueAsyncInvocationsFn != nil {
		return m.acquireDueAsyncInvocationsFn(ctx, workerID, leaseDuration, batchSize)
	}
	return nil, nil
}

func (m *mockMetadataStore) MarkAsyncInvocationSucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	if m.markAsyncInvocationSucceededFn != nil {
		return m.markAsyncInvocationSucceededFn(ctx, id, requestID, output, durationMS, coldStart)
	}
	return nil
}

func (m *mockMetadataStore) MarkAsyncInvocationForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	if m.markAsyncInvocationForRetryFn != nil {
		return m.markAsyncInvocationForRetryFn(ctx, id, lastError, nextRunAt)
	}
	return nil
}

func (m *mockMetadataStore) MarkAsyncInvocationDLQ(ctx context.Context, id, lastError string) error {
	if m.markAsyncInvocationDLQFn != nil {
		return m.markAsyncInvocationDLQFn(ctx, id, lastError)
	}
	return nil
}

func (m *mockMetadataStore) RequeueAsyncInvocation(ctx context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error) {
	if m.requeueAsyncInvocationFn != nil {
		return m.requeueAsyncInvocationFn(ctx, id, maxAttempts)
	}
	return nil, nil
}

func (m *mockMetadataStore) PauseAsyncInvocation(ctx context.Context, id string) (*store.AsyncInvocation, error) {
	if m.pauseAsyncInvocationFn != nil {
		return m.pauseAsyncInvocationFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ResumeAsyncInvocation(ctx context.Context, id string) (*store.AsyncInvocation, error) {
	if m.resumeAsyncInvocationFn != nil {
		return m.resumeAsyncInvocationFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteAsyncInvocation(ctx context.Context, id string) error {
	if m.deleteAsyncInvocationFn != nil {
		return m.deleteAsyncInvocationFn(ctx, id)
	}
	return nil
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
	return nil, nil
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

func (m *mockMetadataStore) EnqueueAsyncInvocationWithIdempotency(ctx context.Context, inv *store.AsyncInvocation, idempotencyKey string, ttl time.Duration) (*store.AsyncInvocation, bool, error) {
	if m.enqueueAsyncInvocationWithIdempotencyFn != nil {
		return m.enqueueAsyncInvocationWithIdempotencyFn(ctx, inv, idempotencyKey, ttl)
	}
	return nil, false, nil
}

func (m *mockMetadataStore) CreateEventTopic(ctx context.Context, topic *store.EventTopic) error {
	if m.createEventTopicFn != nil {
		return m.createEventTopicFn(ctx, topic)
	}
	return nil
}

func (m *mockMetadataStore) GetEventTopic(ctx context.Context, id string) (*store.EventTopic, error) {
	if m.getEventTopicFn != nil {
		return m.getEventTopicFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetEventTopicByName(ctx context.Context, name string) (*store.EventTopic, error) {
	if m.getEventTopicByNameFn != nil {
		return m.getEventTopicByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListEventTopics(ctx context.Context, limit, offset int) ([]*store.EventTopic, error) {
	if m.listEventTopicsFn != nil {
		return m.listEventTopicsFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteEventTopicByName(ctx context.Context, name string) error {
	if m.deleteEventTopicByNameFn != nil {
		return m.deleteEventTopicByNameFn(ctx, name)
	}
	return nil
}

func (m *mockMetadataStore) CreateEventSubscription(ctx context.Context, sub *store.EventSubscription) error {
	if m.createEventSubscriptionFn != nil {
		return m.createEventSubscriptionFn(ctx, sub)
	}
	return nil
}

func (m *mockMetadataStore) GetEventSubscription(ctx context.Context, id string) (*store.EventSubscription, error) {
	if m.getEventSubscriptionFn != nil {
		return m.getEventSubscriptionFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListEventSubscriptions(ctx context.Context, topicID string, limit, offset int) ([]*store.EventSubscription, error) {
	if m.listEventSubscriptionsFn != nil {
		return m.listEventSubscriptionsFn(ctx, topicID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateEventSubscription(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
	if m.updateEventSubscriptionFn != nil {
		return m.updateEventSubscriptionFn(ctx, id, update)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteEventSubscription(ctx context.Context, id string) error {
	if m.deleteEventSubscriptionFn != nil {
		return m.deleteEventSubscriptionFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) PublishEvent(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, error) {
	if m.publishEventFn != nil {
		return m.publishEventFn(ctx, topicID, orderingKey, payload, headers)
	}
	return nil, 0, nil
}

func (m *mockMetadataStore) ListEventMessages(ctx context.Context, topicID string, limit, offset int) ([]*store.EventMessage, error) {
	if m.listEventMessagesFn != nil {
		return m.listEventMessagesFn(ctx, topicID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) PublishEventFromOutbox(ctx context.Context, outboxID, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, bool, error) {
	if m.publishEventFromOutboxFn != nil {
		return m.publishEventFromOutboxFn(ctx, outboxID, topicID, orderingKey, payload, headers)
	}
	return nil, 0, false, nil
}

func (m *mockMetadataStore) GetEventDelivery(ctx context.Context, id string) (*store.EventDelivery, error) {
	if m.getEventDeliveryFn != nil {
		return m.getEventDeliveryFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListEventDeliveries(ctx context.Context, subscriptionID string, limit, offset int, statuses []store.EventDeliveryStatus) ([]*store.EventDelivery, error) {
	if m.listEventDeliveriesFn != nil {
		return m.listEventDeliveriesFn(ctx, subscriptionID, limit, offset, statuses)
	}
	return nil, nil
}

func (m *mockMetadataStore) AcquireDueEventDelivery(ctx context.Context, workerID string, leaseDuration time.Duration) (*store.EventDelivery, error) {
	if m.acquireDueEventDeliveryFn != nil {
		return m.acquireDueEventDeliveryFn(ctx, workerID, leaseDuration)
	}
	return nil, nil
}

func (m *mockMetadataStore) MarkEventDeliverySucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	if m.markEventDeliverySucceededFn != nil {
		return m.markEventDeliverySucceededFn(ctx, id, requestID, output, durationMS, coldStart)
	}
	return nil
}

func (m *mockMetadataStore) MarkEventDeliveryForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	if m.markEventDeliveryForRetryFn != nil {
		return m.markEventDeliveryForRetryFn(ctx, id, lastError, nextRunAt)
	}
	return nil
}

func (m *mockMetadataStore) MarkEventDeliveryDLQ(ctx context.Context, id, lastError string) error {
	if m.markEventDeliveryDLQFn != nil {
		return m.markEventDeliveryDLQFn(ctx, id, lastError)
	}
	return nil
}

func (m *mockMetadataStore) RequeueEventDelivery(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error) {
	if m.requeueEventDeliveryFn != nil {
		return m.requeueEventDeliveryFn(ctx, id, maxAttempts)
	}
	return nil, nil
}

func (m *mockMetadataStore) ResolveEventReplaySequenceByTime(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
	if m.resolveEventReplaySequenceByTimeFn != nil {
		return m.resolveEventReplaySequenceByTimeFn(ctx, subscriptionID, from)
	}
	return 0, nil
}

func (m *mockMetadataStore) SetEventSubscriptionCursor(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
	if m.setEventSubscriptionCursorFn != nil {
		return m.setEventSubscriptionCursorFn(ctx, subscriptionID, lastAckedSequence)
	}
	return nil, nil
}

func (m *mockMetadataStore) ReplayEventSubscription(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
	if m.replayEventSubscriptionFn != nil {
		return m.replayEventSubscriptionFn(ctx, subscriptionID, fromSequence, limit)
	}
	return 0, nil
}

func (m *mockMetadataStore) CreateEventOutbox(ctx context.Context, outbox *store.EventOutbox) error {
	if m.createEventOutboxFn != nil {
		return m.createEventOutboxFn(ctx, outbox)
	}
	return nil
}

func (m *mockMetadataStore) GetEventOutbox(ctx context.Context, id string) (*store.EventOutbox, error) {
	if m.getEventOutboxFn != nil {
		return m.getEventOutboxFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListEventOutbox(ctx context.Context, topicID string, limit, offset int, statuses []store.EventOutboxStatus) ([]*store.EventOutbox, error) {
	if m.listEventOutboxFn != nil {
		return m.listEventOutboxFn(ctx, topicID, limit, offset, statuses)
	}
	return nil, nil
}

func (m *mockMetadataStore) AcquireDueEventOutbox(ctx context.Context, workerID string, leaseDuration time.Duration) (*store.EventOutbox, error) {
	if m.acquireDueEventOutboxFn != nil {
		return m.acquireDueEventOutboxFn(ctx, workerID, leaseDuration)
	}
	return nil, nil
}

func (m *mockMetadataStore) MarkEventOutboxPublished(ctx context.Context, id, messageID string) error {
	if m.markEventOutboxPublishedFn != nil {
		return m.markEventOutboxPublishedFn(ctx, id, messageID)
	}
	return nil
}

func (m *mockMetadataStore) MarkEventOutboxForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	if m.markEventOutboxForRetryFn != nil {
		return m.markEventOutboxForRetryFn(ctx, id, lastError, nextRunAt)
	}
	return nil
}

func (m *mockMetadataStore) MarkEventOutboxFailed(ctx context.Context, id, lastError string) error {
	if m.markEventOutboxFailedFn != nil {
		return m.markEventOutboxFailedFn(ctx, id, lastError)
	}
	return nil
}

func (m *mockMetadataStore) RequeueEventOutbox(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error) {
	if m.requeueEventOutboxFn != nil {
		return m.requeueEventOutboxFn(ctx, id, maxAttempts)
	}
	return nil, nil
}

func (m *mockMetadataStore) PrepareEventInbox(ctx context.Context, subscriptionID, messageID, deliveryID string) (*store.EventInboxRecord, bool, error) {
	if m.prepareEventInboxFn != nil {
		return m.prepareEventInboxFn(ctx, subscriptionID, messageID, deliveryID)
	}
	return nil, false, nil
}

func (m *mockMetadataStore) MarkEventInboxSucceeded(ctx context.Context, subscriptionID, messageID, deliveryID, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	if m.markEventInboxSucceededFn != nil {
		return m.markEventInboxSucceededFn(ctx, subscriptionID, messageID, deliveryID, requestID, output, durationMS, coldStart)
	}
	return nil
}

func (m *mockMetadataStore) MarkEventInboxFailed(ctx context.Context, subscriptionID, messageID, deliveryID, lastError string) error {
	if m.markEventInboxFailedFn != nil {
		return m.markEventInboxFailedFn(ctx, subscriptionID, messageID, deliveryID, lastError)
	}
	return nil
}

func (m *mockMetadataStore) CreateNotification(ctx context.Context, n *store.NotificationRecord) error {
	if m.createNotificationFn != nil {
		return m.createNotificationFn(ctx, n)
	}
	return nil
}

func (m *mockMetadataStore) ListNotifications(ctx context.Context, limit, offset int, status store.NotificationStatus) ([]*store.NotificationRecord, error) {
	if m.listNotificationsFn != nil {
		return m.listNotificationsFn(ctx, limit, offset, status)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetUnreadNotificationCount(ctx context.Context) (int64, error) {
	if m.getUnreadNotificationCountFn != nil {
		return m.getUnreadNotificationCountFn(ctx)
	}
	return 0, nil
}

func (m *mockMetadataStore) MarkNotificationRead(ctx context.Context, id string) (*store.NotificationRecord, error) {
	if m.markNotificationReadFn != nil {
		return m.markNotificationReadFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) MarkAllNotificationsRead(ctx context.Context) (int64, error) {
	if m.markAllNotificationsReadFn != nil {
		return m.markAllNotificationsReadFn(ctx)
	}
	return 0, nil
}

func (m *mockMetadataStore) SaveRuntime(ctx context.Context, rt *store.RuntimeRecord) error {
	if m.saveRuntimeFn != nil {
		return m.saveRuntimeFn(ctx, rt)
	}
	return nil
}

func (m *mockMetadataStore) GetRuntime(ctx context.Context, id string) (*store.RuntimeRecord, error) {
	if m.getRuntimeFn != nil {
		return m.getRuntimeFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListRuntimes(ctx context.Context, limit, offset int) ([]*store.RuntimeRecord, error) {
	if m.listRuntimesFn != nil {
		return m.listRuntimesFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteRuntime(ctx context.Context, id string) error {
	if m.deleteRuntimeFn != nil {
		return m.deleteRuntimeFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) GetConfig(ctx context.Context) (map[string]string, error) {
	if m.getConfigFn != nil {
		return m.getConfigFn(ctx)
	}
	return nil, nil
}

func (m *mockMetadataStore) SetConfig(ctx context.Context, key, value string) error {
	if m.setConfigFn != nil {
		return m.setConfigFn(ctx, key, value)
	}
	return nil
}

func (m *mockMetadataStore) SaveAPIKey(ctx context.Context, key *store.APIKeyRecord) error {
	if m.saveAPIKeyFn != nil {
		return m.saveAPIKeyFn(ctx, key)
	}
	return nil
}

func (m *mockMetadataStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*store.APIKeyRecord, error) {
	if m.getAPIKeyByHashFn != nil {
		return m.getAPIKeyByHashFn(ctx, keyHash)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetAPIKeyByName(ctx context.Context, name string) (*store.APIKeyRecord, error) {
	if m.getAPIKeyByNameFn != nil {
		return m.getAPIKeyByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListAPIKeys(ctx context.Context, limit, offset int) ([]*store.APIKeyRecord, error) {
	if m.listAPIKeysFn != nil {
		return m.listAPIKeysFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteAPIKey(ctx context.Context, name string) error {
	if m.deleteAPIKeyFn != nil {
		return m.deleteAPIKeyFn(ctx, name)
	}
	return nil
}

func (m *mockMetadataStore) SaveSecret(ctx context.Context, name, encryptedValue string) error {
	if m.saveSecretFn != nil {
		return m.saveSecretFn(ctx, name, encryptedValue)
	}
	return nil
}

func (m *mockMetadataStore) GetSecret(ctx context.Context, name string) (string, error) {
	if m.getSecretFn != nil {
		return m.getSecretFn(ctx, name)
	}
	return "", nil
}

func (m *mockMetadataStore) DeleteSecret(ctx context.Context, name string) error {
	if m.deleteSecretFn != nil {
		return m.deleteSecretFn(ctx, name)
	}
	return nil
}

func (m *mockMetadataStore) ListSecrets(ctx context.Context) (map[string]string, error) {
	if m.listSecretsFn != nil {
		return m.listSecretsFn(ctx)
	}
	return nil, nil
}

func (m *mockMetadataStore) SecretExists(ctx context.Context, name string) (bool, error) {
	if m.secretExistsFn != nil {
		return m.secretExistsFn(ctx, name)
	}
	return false, nil
}

func (m *mockMetadataStore) CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error) {
	if m.checkRateLimitFn != nil {
		return m.checkRateLimitFn(ctx, key, maxTokens, refillRate, requested)
	}
	return true, maxTokens, nil
}

func (m *mockMetadataStore) SaveFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	if m.saveFunctionCodeFn != nil {
		return m.saveFunctionCodeFn(ctx, funcID, sourceCode, sourceHash)
	}
	return nil
}

func (m *mockMetadataStore) GetFunctionCode(ctx context.Context, funcID string) (*domain.FunctionCode, error) {
	if m.getFunctionCodeFn != nil {
		return m.getFunctionCodeFn(ctx, funcID)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	if m.updateFunctionCodeFn != nil {
		return m.updateFunctionCodeFn(ctx, funcID, sourceCode, sourceHash)
	}
	return nil
}

func (m *mockMetadataStore) UpdateCompileResult(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
	if m.updateCompileResultFn != nil {
		return m.updateCompileResultFn(ctx, funcID, binary, binaryHash, status, compileError)
	}
	return nil
}

func (m *mockMetadataStore) DeleteFunctionCode(ctx context.Context, funcID string) error {
	if m.deleteFunctionCodeFn != nil {
		return m.deleteFunctionCodeFn(ctx, funcID)
	}
	return nil
}

func (m *mockMetadataStore) SaveFunctionFiles(ctx context.Context, funcID string, files map[string][]byte) error {
	if m.saveFunctionFilesFn != nil {
		return m.saveFunctionFilesFn(ctx, funcID, files)
	}
	return nil
}

func (m *mockMetadataStore) GetFunctionFiles(ctx context.Context, funcID string) (map[string][]byte, error) {
	if m.getFunctionFilesFn != nil {
		return m.getFunctionFilesFn(ctx, funcID)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListFunctionFiles(ctx context.Context, funcID string) ([]store.FunctionFileInfo, error) {
	if m.listFunctionFilesFn != nil {
		return m.listFunctionFilesFn(ctx, funcID)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteFunctionFiles(ctx context.Context, funcID string) error {
	if m.deleteFunctionFilesFn != nil {
		return m.deleteFunctionFilesFn(ctx, funcID)
	}
	return nil
}

func (m *mockMetadataStore) HasFunctionFiles(ctx context.Context, funcID string) (bool, error) {
	if m.hasFunctionFilesFn != nil {
		return m.hasFunctionFilesFn(ctx, funcID)
	}
	return false, nil
}

func (m *mockMetadataStore) SaveGatewayRoute(ctx context.Context, route *domain.GatewayRoute) error {
	if m.saveGatewayRouteFn != nil {
		return m.saveGatewayRouteFn(ctx, route)
	}
	return nil
}

func (m *mockMetadataStore) GetGatewayRoute(ctx context.Context, id string) (*domain.GatewayRoute, error) {
	if m.getGatewayRouteFn != nil {
		return m.getGatewayRouteFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetRouteByDomainPath(ctx context.Context, domainName, path string) (*domain.GatewayRoute, error) {
	if m.getRouteByDomainPathFn != nil {
		return m.getRouteByDomainPathFn(ctx, domainName, path)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListGatewayRoutes(ctx context.Context, limit, offset int) ([]*domain.GatewayRoute, error) {
	if m.listGatewayRoutesFn != nil {
		return m.listGatewayRoutesFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListRoutesByDomain(ctx context.Context, domainName string, limit, offset int) ([]*domain.GatewayRoute, error) {
	if m.listRoutesByDomainFn != nil {
		return m.listRoutesByDomainFn(ctx, domainName, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteGatewayRoute(ctx context.Context, id string) error {
	if m.deleteGatewayRouteFn != nil {
		return m.deleteGatewayRouteFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) UpdateGatewayRoute(ctx context.Context, id string, route *domain.GatewayRoute) error {
	if m.updateGatewayRouteFn != nil {
		return m.updateGatewayRouteFn(ctx, id, route)
	}
	return nil
}

func (m *mockMetadataStore) SaveLayer(ctx context.Context, layer *domain.Layer) error {
	if m.saveLayerFn != nil {
		return m.saveLayerFn(ctx, layer)
	}
	return nil
}

func (m *mockMetadataStore) GetLayer(ctx context.Context, id string) (*domain.Layer, error) {
	if m.getLayerFn != nil {
		return m.getLayerFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetLayerByName(ctx context.Context, name string) (*domain.Layer, error) {
	if m.getLayerByNameFn != nil {
		return m.getLayerByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetLayerByContentHash(ctx context.Context, hash string) (*domain.Layer, error) {
	if m.getLayerByContentHashFn != nil {
		return m.getLayerByContentHashFn(ctx, hash)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListLayers(ctx context.Context, limit, offset int) ([]*domain.Layer, error) {
	if m.listLayersFn != nil {
		return m.listLayersFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteLayer(ctx context.Context, id string) error {
	if m.deleteLayerFn != nil {
		return m.deleteLayerFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) SetFunctionLayers(ctx context.Context, funcID string, layerIDs []string) error {
	if m.setFunctionLayersFn != nil {
		return m.setFunctionLayersFn(ctx, funcID, layerIDs)
	}
	return nil
}

func (m *mockMetadataStore) GetFunctionLayers(ctx context.Context, funcID string) ([]*domain.Layer, error) {
	if m.getFunctionLayersFn != nil {
		return m.getFunctionLayersFn(ctx, funcID)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListFunctionsByLayer(ctx context.Context, layerID string) ([]string, error) {
	if m.listFunctionsByLayerFn != nil {
		return m.listFunctionsByLayerFn(ctx, layerID)
	}
	return nil, nil
}

func (m *mockMetadataStore) CreateTrigger(ctx context.Context, trigger *store.TriggerRecord) error {
	if m.createTriggerFn != nil {
		return m.createTriggerFn(ctx, trigger)
	}
	return nil
}

func (m *mockMetadataStore) GetTrigger(ctx context.Context, id string) (*store.TriggerRecord, error) {
	if m.getTriggerFn != nil {
		return m.getTriggerFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetTriggerByName(ctx context.Context, name string) (*store.TriggerRecord, error) {
	if m.getTriggerByNameFn != nil {
		return m.getTriggerByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListTriggers(ctx context.Context, limit, offset int) ([]*store.TriggerRecord, error) {
	if m.listTriggersFn != nil {
		return m.listTriggersFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateTrigger(ctx context.Context, id string, update *store.TriggerUpdate) (*store.TriggerRecord, error) {
	if m.updateTriggerFn != nil {
		return m.updateTriggerFn(ctx, id, update)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteTrigger(ctx context.Context, id string) error {
	if m.deleteTriggerFn != nil {
		return m.deleteTriggerFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) CreateVolume(ctx context.Context, vol *domain.Volume) error {
	if m.createVolumeFn != nil {
		return m.createVolumeFn(ctx, vol)
	}
	return nil
}

func (m *mockMetadataStore) GetVolume(ctx context.Context, id string) (*domain.Volume, error) {
	if m.getVolumeFn != nil {
		return m.getVolumeFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetVolumeByName(ctx context.Context, name string) (*domain.Volume, error) {
	if m.getVolumeByNameFn != nil {
		return m.getVolumeByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListVolumes(ctx context.Context) ([]*domain.Volume, error) {
	if m.listVolumesFn != nil {
		return m.listVolumesFn(ctx)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateVolume(ctx context.Context, id string, updates map[string]interface{}) error {
	if m.updateVolumeFn != nil {
		return m.updateVolumeFn(ctx, id, updates)
	}
	return nil
}

func (m *mockMetadataStore) DeleteVolume(ctx context.Context, id string) error {
	if m.deleteVolumeFn != nil {
		return m.deleteVolumeFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) GetFunctionVolumes(ctx context.Context, functionID string) ([]*domain.Volume, error) {
	if m.getFunctionVolumesFn != nil {
		return m.getFunctionVolumesFn(ctx, functionID)
	}
	return nil, nil
}

func (m *mockMetadataStore) CreateRole(ctx context.Context, role *store.RoleRecord) (*store.RoleRecord, error) {
	if m.createRoleFn != nil {
		return m.createRoleFn(ctx, role)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetRole(ctx context.Context, id string) (*store.RoleRecord, error) {
	if m.getRoleFn != nil {
		return m.getRoleFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListRoles(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleRecord, error) {
	if m.listRolesFn != nil {
		return m.listRolesFn(ctx, tenantID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteRole(ctx context.Context, id string) error {
	if m.deleteRoleFn != nil {
		return m.deleteRoleFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) CreatePermission(ctx context.Context, perm *store.PermissionRecord) (*store.PermissionRecord, error) {
	if m.createPermissionFn != nil {
		return m.createPermissionFn(ctx, perm)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetPermission(ctx context.Context, id string) (*store.PermissionRecord, error) {
	if m.getPermissionFn != nil {
		return m.getPermissionFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListPermissions(ctx context.Context, limit, offset int) ([]*store.PermissionRecord, error) {
	if m.listPermissionsFn != nil {
		return m.listPermissionsFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeletePermission(ctx context.Context, id string) error {
	if m.deletePermissionFn != nil {
		return m.deletePermissionFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	if m.assignPermissionToRoleFn != nil {
		return m.assignPermissionToRoleFn(ctx, roleID, permissionID)
	}
	return nil
}

func (m *mockMetadataStore) RevokePermissionFromRole(ctx context.Context, roleID, permissionID string) error {
	if m.revokePermissionFromRoleFn != nil {
		return m.revokePermissionFromRoleFn(ctx, roleID, permissionID)
	}
	return nil
}

func (m *mockMetadataStore) ListRolePermissions(ctx context.Context, roleID string) ([]*store.PermissionRecord, error) {
	if m.listRolePermissionsFn != nil {
		return m.listRolePermissionsFn(ctx, roleID)
	}
	return nil, nil
}

func (m *mockMetadataStore) CreateRoleAssignment(ctx context.Context, ra *store.RoleAssignmentRecord) (*store.RoleAssignmentRecord, error) {
	if m.createRoleAssignmentFn != nil {
		return m.createRoleAssignmentFn(ctx, ra)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetRoleAssignment(ctx context.Context, id string) (*store.RoleAssignmentRecord, error) {
	if m.getRoleAssignmentFn != nil {
		return m.getRoleAssignmentFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListRoleAssignments(ctx context.Context, tenantID string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
	if m.listRoleAssignmentsFn != nil {
		return m.listRoleAssignmentsFn(ctx, tenantID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListRoleAssignmentsByPrincipal(ctx context.Context, tenantID string, principalType domain.PrincipalType, principalID string, limit, offset int) ([]*store.RoleAssignmentRecord, error) {
	if m.listRoleAssignmentsByPrincipalFn != nil {
		return m.listRoleAssignmentsByPrincipalFn(ctx, tenantID, principalType, principalID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteRoleAssignment(ctx context.Context, id string) error {
	if m.deleteRoleAssignmentFn != nil {
		return m.deleteRoleAssignmentFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) ResolveEffectivePermissions(ctx context.Context, tenantID, subject string) ([]string, error) {
	if m.resolveEffectivePermissionsFn != nil {
		return m.resolveEffectivePermissionsFn(ctx, tenantID, subject)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListTenantMenuPermissions(ctx context.Context, tenantID string) ([]*store.MenuPermissionRecord, error) {
	if m.listTenantMenuPermissionsFn != nil {
		return m.listTenantMenuPermissionsFn(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpsertTenantMenuPermission(ctx context.Context, tenantID, menuKey string, enabled bool) (*store.MenuPermissionRecord, error) {
	if m.upsertTenantMenuPermissionFn != nil {
		return m.upsertTenantMenuPermissionFn(ctx, tenantID, menuKey, enabled)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteTenantMenuPermission(ctx context.Context, tenantID, menuKey string) error {
	if m.deleteTenantMenuPermissionFn != nil {
		return m.deleteTenantMenuPermissionFn(ctx, tenantID, menuKey)
	}
	return nil
}

func (m *mockMetadataStore) SeedDefaultMenuPermissions(ctx context.Context, tenantID string) error {
	if m.seedDefaultMenuPermissionsFn != nil {
		return m.seedDefaultMenuPermissionsFn(ctx, tenantID)
	}
	return nil
}

func (m *mockMetadataStore) ListTenantButtonPermissions(ctx context.Context, tenantID string) ([]*store.ButtonPermissionRecord, error) {
	if m.listTenantButtonPermissionsFn != nil {
		return m.listTenantButtonPermissionsFn(ctx, tenantID)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpsertTenantButtonPermission(ctx context.Context, tenantID, permissionKey string, enabled bool) (*store.ButtonPermissionRecord, error) {
	if m.upsertTenantButtonPermissionFn != nil {
		return m.upsertTenantButtonPermissionFn(ctx, tenantID, permissionKey, enabled)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteTenantButtonPermission(ctx context.Context, tenantID, permissionKey string) error {
	if m.deleteTenantButtonPermissionFn != nil {
		return m.deleteTenantButtonPermissionFn(ctx, tenantID, permissionKey)
	}
	return nil
}

func (m *mockMetadataStore) SeedDefaultButtonPermissions(ctx context.Context, tenantID string) error {
	if m.seedDefaultButtonPermissionsFn != nil {
		return m.seedDefaultButtonPermissionsFn(ctx, tenantID)
	}
	return nil
}

func (m *mockMetadataStore) SaveAPIDocShare(ctx context.Context, share *store.APIDocShare) error {
	if m.saveAPIDocShareFn != nil {
		return m.saveAPIDocShareFn(ctx, share)
	}
	return nil
}

func (m *mockMetadataStore) GetAPIDocShareByToken(ctx context.Context, token string) (*store.APIDocShare, error) {
	if m.getAPIDocShareByTokenFn != nil {
		return m.getAPIDocShareByTokenFn(ctx, token)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListAPIDocShares(ctx context.Context, tenantID, namespace string, limit, offset int) ([]*store.APIDocShare, error) {
	if m.listAPIDocSharesFn != nil {
		return m.listAPIDocSharesFn(ctx, tenantID, namespace, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteAPIDocShare(ctx context.Context, id string) error {
	if m.deleteAPIDocShareFn != nil {
		return m.deleteAPIDocShareFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) IncrementAPIDocShareAccess(ctx context.Context, token string) error {
	if m.incrementAPIDocShareAccessFn != nil {
		return m.incrementAPIDocShareAccessFn(ctx, token)
	}
	return nil
}

func (m *mockMetadataStore) SaveFunctionDoc(ctx context.Context, doc *store.FunctionDoc) error {
	if m.saveFunctionDocFn != nil {
		return m.saveFunctionDocFn(ctx, doc)
	}
	return nil
}

func (m *mockMetadataStore) GetFunctionDoc(ctx context.Context, functionName string) (*store.FunctionDoc, error) {
	if m.getFunctionDocFn != nil {
		return m.getFunctionDocFn(ctx, functionName)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteFunctionDoc(ctx context.Context, functionName string) error {
	if m.deleteFunctionDocFn != nil {
		return m.deleteFunctionDocFn(ctx, functionName)
	}
	return nil
}

func (m *mockMetadataStore) SaveWorkflowDoc(ctx context.Context, doc *store.WorkflowDoc) error {
	if m.saveWorkflowDocFn != nil {
		return m.saveWorkflowDocFn(ctx, doc)
	}
	return nil
}

func (m *mockMetadataStore) GetWorkflowDoc(ctx context.Context, workflowName string) (*store.WorkflowDoc, error) {
	if m.getWorkflowDocFn != nil {
		return m.getWorkflowDocFn(ctx, workflowName)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteWorkflowDoc(ctx context.Context, workflowName string) error {
	if m.deleteWorkflowDocFn != nil {
		return m.deleteWorkflowDocFn(ctx, workflowName)
	}
	return nil
}

func (m *mockMetadataStore) SaveTestSuite(ctx context.Context, ts *store.TestSuite) error {
	if m.saveTestSuiteFn != nil {
		return m.saveTestSuiteFn(ctx, ts)
	}
	return nil
}

func (m *mockMetadataStore) GetTestSuite(ctx context.Context, functionName string) (*store.TestSuite, error) {
	if m.getTestSuiteFn != nil {
		return m.getTestSuiteFn(ctx, functionName)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteTestSuite(ctx context.Context, functionName string) error {
	if m.deleteTestSuiteFn != nil {
		return m.deleteTestSuiteFn(ctx, functionName)
	}
	return nil
}

func (m *mockMetadataStore) CreateDbResource(ctx context.Context, res *store.DbResourceRecord) (*store.DbResourceRecord, error) {
	if m.createDbResourceFn != nil {
		return m.createDbResourceFn(ctx, res)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetDbResource(ctx context.Context, id string) (*store.DbResourceRecord, error) {
	if m.getDbResourceFn != nil {
		return m.getDbResourceFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetDbResourceByName(ctx context.Context, name string) (*store.DbResourceRecord, error) {
	if m.getDbResourceByNameFn != nil {
		return m.getDbResourceByNameFn(ctx, name)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListDbResources(ctx context.Context, limit, offset int) ([]*store.DbResourceRecord, error) {
	if m.listDbResourcesFn != nil {
		return m.listDbResourcesFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateDbResource(ctx context.Context, id string, update *store.DbResourceUpdate) (*store.DbResourceRecord, error) {
	if m.updateDbResourceFn != nil {
		return m.updateDbResourceFn(ctx, id, update)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteDbResource(ctx context.Context, id string) error {
	if m.deleteDbResourceFn != nil {
		return m.deleteDbResourceFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) CreateDbBinding(ctx context.Context, binding *store.DbBindingRecord) (*store.DbBindingRecord, error) {
	if m.createDbBindingFn != nil {
		return m.createDbBindingFn(ctx, binding)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetDbBinding(ctx context.Context, id string) (*store.DbBindingRecord, error) {
	if m.getDbBindingFn != nil {
		return m.getDbBindingFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListDbBindings(ctx context.Context, dbResourceID string, limit, offset int) ([]*store.DbBindingRecord, error) {
	if m.listDbBindingsFn != nil {
		return m.listDbBindingsFn(ctx, dbResourceID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListDbBindingsByFunction(ctx context.Context, functionID string, limit, offset int) ([]*store.DbBindingRecord, error) {
	if m.listDbBindingsByFunctionFn != nil {
		return m.listDbBindingsByFunctionFn(ctx, functionID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateDbBinding(ctx context.Context, id string, update *store.DbBindingUpdate) (*store.DbBindingRecord, error) {
	if m.updateDbBindingFn != nil {
		return m.updateDbBindingFn(ctx, id, update)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteDbBinding(ctx context.Context, id string) error {
	if m.deleteDbBindingFn != nil {
		return m.deleteDbBindingFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) CreateCredentialPolicy(ctx context.Context, policy *store.CredentialPolicyRecord) (*store.CredentialPolicyRecord, error) {
	if m.createCredentialPolicyFn != nil {
		return m.createCredentialPolicyFn(ctx, policy)
	}
	return nil, nil
}

func (m *mockMetadataStore) GetCredentialPolicy(ctx context.Context, dbResourceID string) (*store.CredentialPolicyRecord, error) {
	if m.getCredentialPolicyFn != nil {
		return m.getCredentialPolicyFn(ctx, dbResourceID)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateCredentialPolicy(ctx context.Context, dbResourceID string, update *store.CredentialPolicyUpdate) (*store.CredentialPolicyRecord, error) {
	if m.updateCredentialPolicyFn != nil {
		return m.updateCredentialPolicyFn(ctx, dbResourceID, update)
	}
	return nil, nil
}

func (m *mockMetadataStore) DeleteCredentialPolicy(ctx context.Context, dbResourceID string) error {
	if m.deleteCredentialPolicyFn != nil {
		return m.deleteCredentialPolicyFn(ctx, dbResourceID)
	}
	return nil
}

func (m *mockMetadataStore) SaveDbRequestLog(ctx context.Context, log *domain.DbRequestLog) error {
	if m.saveDbRequestLogFn != nil {
		return m.saveDbRequestLogFn(ctx, log)
	}
	return nil
}

func (m *mockMetadataStore) ListDbRequestLogs(ctx context.Context, dbResourceID string, limit, offset int) ([]*domain.DbRequestLog, error) {
	if m.listDbRequestLogsFn != nil {
		return m.listDbRequestLogsFn(ctx, dbResourceID, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpsertClusterNode(ctx context.Context, node *store.ClusterNodeRecord) error {
	if m.upsertClusterNodeFn != nil {
		return m.upsertClusterNodeFn(ctx, node)
	}
	return nil
}

func (m *mockMetadataStore) GetClusterNode(ctx context.Context, id string) (*store.ClusterNodeRecord, error) {
	if m.getClusterNodeFn != nil {
		return m.getClusterNodeFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMetadataStore) ListClusterNodes(ctx context.Context, limit, offset int) ([]*store.ClusterNodeRecord, error) {
	if m.listClusterNodesFn != nil {
		return m.listClusterNodesFn(ctx, limit, offset)
	}
	return nil, nil
}

func (m *mockMetadataStore) UpdateClusterNodeHeartbeat(ctx context.Context, id string, activeVMs, queueDepth int) error {
	if m.updateClusterNodeHeartbeatFn != nil {
		return m.updateClusterNodeHeartbeatFn(ctx, id, activeVMs, queueDepth)
	}
	return nil
}

func (m *mockMetadataStore) DeleteClusterNode(ctx context.Context, id string) error {
	if m.deleteClusterNodeFn != nil {
		return m.deleteClusterNodeFn(ctx, id)
	}
	return nil
}

func (m *mockMetadataStore) ListActiveClusterNodes(ctx context.Context) ([]*store.ClusterNodeRecord, error) {
	if m.listActiveClusterNodesFn != nil {
		return m.listActiveClusterNodesFn(ctx)
	}
	return nil, nil
}

func (m *mockMetadataStore) RecordPoolMetrics(ctx context.Context, snap store.PoolMetricsSnapshot) error {
	return nil
}

func (m *mockMetadataStore) PrunePoolMetrics(ctx context.Context, retentionSeconds int) error {
	return nil
}

// --- Test helper ---

func setupTestHandler(t *testing.T, ms *mockMetadataStore) (*Handler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	h := &Handler{Store: s}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}
