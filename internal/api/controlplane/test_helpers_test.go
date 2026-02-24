package controlplane

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// newTestStore is a convenience wrapper; same as store.NewStore but shorter.
func newTestStore(ms *mockMetadataStore) *store.Store {
	return store.NewStore(ms)
}

// ---------- mock schedule store (embedded in mockMetadataStore) ----------

type mockScheduleStore struct {
	saveScheduleFn            func(ctx context.Context, s *store.Schedule) error
	listSchedulesByFunctionFn func(ctx context.Context, functionName string, limit, offset int) ([]*store.Schedule, error)
	listAllSchedulesFn        func(ctx context.Context, limit, offset int) ([]*store.Schedule, error)
	getScheduleFn             func(ctx context.Context, id string) (*store.Schedule, error)
	deleteScheduleFn          func(ctx context.Context, id string) error
	updateScheduleLastRunFn   func(ctx context.Context, id string, t time.Time) error
	updateScheduleEnabledFn   func(ctx context.Context, id string, enabled bool) error
	updateScheduleCronFn      func(ctx context.Context, id string, cronExpr string) error
}

func (m *mockScheduleStore) SaveSchedule(ctx context.Context, s *store.Schedule) error {
	if m.saveScheduleFn != nil {
		return m.saveScheduleFn(ctx, s)
	}
	return nil
}
func (m *mockScheduleStore) ListSchedulesByFunction(ctx context.Context, functionName string, limit, offset int) ([]*store.Schedule, error) {
	if m.listSchedulesByFunctionFn != nil {
		return m.listSchedulesByFunctionFn(ctx, functionName, limit, offset)
	}
	return nil, nil
}
func (m *mockScheduleStore) ListAllSchedules(ctx context.Context, limit, offset int) ([]*store.Schedule, error) {
	if m.listAllSchedulesFn != nil {
		return m.listAllSchedulesFn(ctx, limit, offset)
	}
	return nil, nil
}
func (m *mockScheduleStore) GetSchedule(ctx context.Context, id string) (*store.Schedule, error) {
	if m.getScheduleFn != nil {
		return m.getScheduleFn(ctx, id)
	}
	return nil, nil
}
func (m *mockScheduleStore) DeleteSchedule(ctx context.Context, id string) error {
	if m.deleteScheduleFn != nil {
		return m.deleteScheduleFn(ctx, id)
	}
	return nil
}
func (m *mockScheduleStore) UpdateScheduleLastRun(ctx context.Context, id string, t time.Time) error {
	if m.updateScheduleLastRunFn != nil {
		return m.updateScheduleLastRunFn(ctx, id, t)
	}
	return nil
}
func (m *mockScheduleStore) UpdateScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	if m.updateScheduleEnabledFn != nil {
		return m.updateScheduleEnabledFn(ctx, id, enabled)
	}
	return nil
}
func (m *mockScheduleStore) UpdateScheduleCron(ctx context.Context, id string, cronExpr string) error {
	if m.updateScheduleCronFn != nil {
		return m.updateScheduleCronFn(ctx, id, cronExpr)
	}
	return nil
}

// ---------- mock workflow store ----------

type mockWorkflowStore struct {
	createWorkflowFn             func(ctx context.Context, w *domain.Workflow) error
	getWorkflowFn                func(ctx context.Context, id string) (*domain.Workflow, error)
	getWorkflowByNameFn          func(ctx context.Context, name string) (*domain.Workflow, error)
	listWorkflowsFn              func(ctx context.Context, limit, offset int) ([]*domain.Workflow, error)
	deleteWorkflowFn             func(ctx context.Context, id string) error
	updateWorkflowVersionFn      func(ctx context.Context, id string, version int) error
	createWorkflowVersionFn      func(ctx context.Context, v *domain.WorkflowVersion) error
	getWorkflowVersionFn         func(ctx context.Context, id string) (*domain.WorkflowVersion, error)
	getWorkflowVersionByNumberFn func(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error)
	listWorkflowVersionsFn       func(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowVersion, error)
	createWorkflowNodesFn        func(ctx context.Context, nodes []domain.WorkflowNode) error
	createWorkflowEdgesFn        func(ctx context.Context, edges []domain.WorkflowEdge) error
	getWorkflowNodesFn           func(ctx context.Context, versionID string) ([]domain.WorkflowNode, error)
	getWorkflowNodeByIDFn        func(ctx context.Context, nodeID string) (*domain.WorkflowNode, error)
	getWorkflowEdgesFn           func(ctx context.Context, versionID string) ([]domain.WorkflowEdge, error)
	createRunFn                  func(ctx context.Context, run *domain.WorkflowRun) error
	getRunFn                     func(ctx context.Context, id string) (*domain.WorkflowRun, error)
	listRunsFn                   func(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowRun, error)
	updateRunStatusFn            func(ctx context.Context, id string, status domain.RunStatus, errMsg string, output json.RawMessage) error
	createRunNodesFn             func(ctx context.Context, nodes []domain.RunNode) error
	getRunNodesFn                func(ctx context.Context, runID string) ([]domain.RunNode, error)
	acquireReadyNodeFn           func(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (*domain.RunNode, error)
	updateRunNodeFn              func(ctx context.Context, node *domain.RunNode) error
	decrementDepsFn              func(ctx context.Context, runID string, nodeKeys []string) error
	createNodeAttemptFn          func(ctx context.Context, a *domain.NodeAttempt) error
	updateNodeAttemptFn          func(ctx context.Context, a *domain.NodeAttempt) error
	getNodeAttemptsFn            func(ctx context.Context, runNodeID string) ([]domain.NodeAttempt, error)
}

func (m *mockWorkflowStore) CreateWorkflow(ctx context.Context, w *domain.Workflow) error {
	if m.createWorkflowFn != nil {
		return m.createWorkflowFn(ctx, w)
	}
	return nil
}
func (m *mockWorkflowStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	if m.getWorkflowFn != nil {
		return m.getWorkflowFn(ctx, id)
	}
	return nil, nil
}
func (m *mockWorkflowStore) GetWorkflowByName(ctx context.Context, name string) (*domain.Workflow, error) {
	if m.getWorkflowByNameFn != nil {
		return m.getWorkflowByNameFn(ctx, name)
	}
	return nil, nil
}
func (m *mockWorkflowStore) ListWorkflows(ctx context.Context, limit, offset int) ([]*domain.Workflow, error) {
	if m.listWorkflowsFn != nil {
		return m.listWorkflowsFn(ctx, limit, offset)
	}
	return nil, nil
}
func (m *mockWorkflowStore) DeleteWorkflow(ctx context.Context, id string) error {
	if m.deleteWorkflowFn != nil {
		return m.deleteWorkflowFn(ctx, id)
	}
	return nil
}
func (m *mockWorkflowStore) UpdateWorkflowVersion(ctx context.Context, id string, version int) error {
	if m.updateWorkflowVersionFn != nil {
		return m.updateWorkflowVersionFn(ctx, id, version)
	}
	return nil
}
func (m *mockWorkflowStore) CreateWorkflowVersion(ctx context.Context, v *domain.WorkflowVersion) error {
	if m.createWorkflowVersionFn != nil {
		return m.createWorkflowVersionFn(ctx, v)
	}
	return nil
}
func (m *mockWorkflowStore) GetWorkflowVersion(ctx context.Context, id string) (*domain.WorkflowVersion, error) {
	if m.getWorkflowVersionFn != nil {
		return m.getWorkflowVersionFn(ctx, id)
	}
	return nil, nil
}
func (m *mockWorkflowStore) GetWorkflowVersionByNumber(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error) {
	if m.getWorkflowVersionByNumberFn != nil {
		return m.getWorkflowVersionByNumberFn(ctx, workflowID, version)
	}
	return nil, nil
}
func (m *mockWorkflowStore) ListWorkflowVersions(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowVersion, error) {
	if m.listWorkflowVersionsFn != nil {
		return m.listWorkflowVersionsFn(ctx, workflowID, limit, offset)
	}
	return nil, nil
}
func (m *mockWorkflowStore) CreateWorkflowNodes(ctx context.Context, nodes []domain.WorkflowNode) error {
	if m.createWorkflowNodesFn != nil {
		return m.createWorkflowNodesFn(ctx, nodes)
	}
	return nil
}
func (m *mockWorkflowStore) CreateWorkflowEdges(ctx context.Context, edges []domain.WorkflowEdge) error {
	if m.createWorkflowEdgesFn != nil {
		return m.createWorkflowEdgesFn(ctx, edges)
	}
	return nil
}
func (m *mockWorkflowStore) GetWorkflowNodes(ctx context.Context, versionID string) ([]domain.WorkflowNode, error) {
	if m.getWorkflowNodesFn != nil {
		return m.getWorkflowNodesFn(ctx, versionID)
	}
	return nil, nil
}
func (m *mockWorkflowStore) GetWorkflowNodeByID(ctx context.Context, nodeID string) (*domain.WorkflowNode, error) {
	if m.getWorkflowNodeByIDFn != nil {
		return m.getWorkflowNodeByIDFn(ctx, nodeID)
	}
	return nil, nil
}
func (m *mockWorkflowStore) GetWorkflowEdges(ctx context.Context, versionID string) ([]domain.WorkflowEdge, error) {
	if m.getWorkflowEdgesFn != nil {
		return m.getWorkflowEdgesFn(ctx, versionID)
	}
	return nil, nil
}
func (m *mockWorkflowStore) CreateRun(ctx context.Context, run *domain.WorkflowRun) error {
	if m.createRunFn != nil {
		return m.createRunFn(ctx, run)
	}
	return nil
}
func (m *mockWorkflowStore) GetRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	if m.getRunFn != nil {
		return m.getRunFn(ctx, id)
	}
	return nil, nil
}
func (m *mockWorkflowStore) ListRuns(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowRun, error) {
	if m.listRunsFn != nil {
		return m.listRunsFn(ctx, workflowID, limit, offset)
	}
	return nil, nil
}
func (m *mockWorkflowStore) UpdateRunStatus(ctx context.Context, id string, status domain.RunStatus, errMsg string, output json.RawMessage) error {
	if m.updateRunStatusFn != nil {
		return m.updateRunStatusFn(ctx, id, status, errMsg, output)
	}
	return nil
}
func (m *mockWorkflowStore) CreateRunNodes(ctx context.Context, nodes []domain.RunNode) error {
	if m.createRunNodesFn != nil {
		return m.createRunNodesFn(ctx, nodes)
	}
	return nil
}
func (m *mockWorkflowStore) GetRunNodes(ctx context.Context, runID string) ([]domain.RunNode, error) {
	if m.getRunNodesFn != nil {
		return m.getRunNodesFn(ctx, runID)
	}
	return nil, nil
}
func (m *mockWorkflowStore) AcquireReadyNode(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (*domain.RunNode, error) {
	if m.acquireReadyNodeFn != nil {
		return m.acquireReadyNodeFn(ctx, leaseOwner, leaseDuration)
	}
	return nil, nil
}
func (m *mockWorkflowStore) UpdateRunNode(ctx context.Context, node *domain.RunNode) error {
	if m.updateRunNodeFn != nil {
		return m.updateRunNodeFn(ctx, node)
	}
	return nil
}
func (m *mockWorkflowStore) DecrementDeps(ctx context.Context, runID string, nodeKeys []string) error {
	if m.decrementDepsFn != nil {
		return m.decrementDepsFn(ctx, runID, nodeKeys)
	}
	return nil
}
func (m *mockWorkflowStore) CreateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	if m.createNodeAttemptFn != nil {
		return m.createNodeAttemptFn(ctx, a)
	}
	return nil
}
func (m *mockWorkflowStore) UpdateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	if m.updateNodeAttemptFn != nil {
		return m.updateNodeAttemptFn(ctx, a)
	}
	return nil
}
func (m *mockWorkflowStore) GetNodeAttempts(ctx context.Context, runNodeID string) ([]domain.NodeAttempt, error) {
	if m.getNodeAttemptsFn != nil {
		return m.getNodeAttemptsFn(ctx, runNodeID)
	}
	return nil, nil
}

// ---------- compositeMetadataStore ----------
// compositeMetadataStore embeds mockMetadataStore plus optional ScheduleStore / WorkflowStore
// so that store.NewStore can discover them via type assertion.
type compositeMetadataStore struct {
	*mockMetadataStore
	sched *mockScheduleStore
	wf    *mockWorkflowStore
}

func (c *compositeMetadataStore) SaveSchedule(ctx context.Context, s *store.Schedule) error {
	return c.sched.SaveSchedule(ctx, s)
}
func (c *compositeMetadataStore) ListSchedulesByFunction(ctx context.Context, functionName string, limit, offset int) ([]*store.Schedule, error) {
	return c.sched.ListSchedulesByFunction(ctx, functionName, limit, offset)
}
func (c *compositeMetadataStore) ListAllSchedules(ctx context.Context, limit, offset int) ([]*store.Schedule, error) {
	return c.sched.ListAllSchedules(ctx, limit, offset)
}
func (c *compositeMetadataStore) GetSchedule(ctx context.Context, id string) (*store.Schedule, error) {
	return c.sched.GetSchedule(ctx, id)
}
func (c *compositeMetadataStore) DeleteSchedule(ctx context.Context, id string) error {
	return c.sched.DeleteSchedule(ctx, id)
}
func (c *compositeMetadataStore) UpdateScheduleLastRun(ctx context.Context, id string, t time.Time) error {
	return c.sched.UpdateScheduleLastRun(ctx, id, t)
}
func (c *compositeMetadataStore) UpdateScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	return c.sched.UpdateScheduleEnabled(ctx, id, enabled)
}
func (c *compositeMetadataStore) UpdateScheduleCron(ctx context.Context, id string, cronExpr string) error {
	return c.sched.UpdateScheduleCron(ctx, id, cronExpr)
}

// WorkflowStore delegation
func (c *compositeMetadataStore) CreateWorkflow(ctx context.Context, w *domain.Workflow) error {
	return c.wf.CreateWorkflow(ctx, w)
}
func (c *compositeMetadataStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	return c.wf.GetWorkflow(ctx, id)
}
func (c *compositeMetadataStore) GetWorkflowByName(ctx context.Context, name string) (*domain.Workflow, error) {
	return c.wf.GetWorkflowByName(ctx, name)
}
func (c *compositeMetadataStore) ListWorkflows(ctx context.Context, limit, offset int) ([]*domain.Workflow, error) {
	return c.wf.ListWorkflows(ctx, limit, offset)
}
func (c *compositeMetadataStore) DeleteWorkflow(ctx context.Context, id string) error {
	return c.wf.DeleteWorkflow(ctx, id)
}
func (c *compositeMetadataStore) UpdateWorkflowVersion(ctx context.Context, id string, version int) error {
	return c.wf.UpdateWorkflowVersion(ctx, id, version)
}
func (c *compositeMetadataStore) CreateWorkflowVersion(ctx context.Context, v *domain.WorkflowVersion) error {
	return c.wf.CreateWorkflowVersion(ctx, v)
}
func (c *compositeMetadataStore) GetWorkflowVersion(ctx context.Context, id string) (*domain.WorkflowVersion, error) {
	return c.wf.GetWorkflowVersion(ctx, id)
}
func (c *compositeMetadataStore) GetWorkflowVersionByNumber(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error) {
	return c.wf.GetWorkflowVersionByNumber(ctx, workflowID, version)
}
func (c *compositeMetadataStore) ListWorkflowVersions(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowVersion, error) {
	return c.wf.ListWorkflowVersions(ctx, workflowID, limit, offset)
}
func (c *compositeMetadataStore) CreateWorkflowNodes(ctx context.Context, nodes []domain.WorkflowNode) error {
	return c.wf.CreateWorkflowNodes(ctx, nodes)
}
func (c *compositeMetadataStore) CreateWorkflowEdges(ctx context.Context, edges []domain.WorkflowEdge) error {
	return c.wf.CreateWorkflowEdges(ctx, edges)
}
func (c *compositeMetadataStore) GetWorkflowNodes(ctx context.Context, versionID string) ([]domain.WorkflowNode, error) {
	return c.wf.GetWorkflowNodes(ctx, versionID)
}
func (c *compositeMetadataStore) GetWorkflowNodeByID(ctx context.Context, nodeID string) (*domain.WorkflowNode, error) {
	return c.wf.GetWorkflowNodeByID(ctx, nodeID)
}
func (c *compositeMetadataStore) GetWorkflowEdges(ctx context.Context, versionID string) ([]domain.WorkflowEdge, error) {
	return c.wf.GetWorkflowEdges(ctx, versionID)
}
func (c *compositeMetadataStore) CreateRun(ctx context.Context, run *domain.WorkflowRun) error {
	return c.wf.CreateRun(ctx, run)
}
func (c *compositeMetadataStore) GetRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	return c.wf.GetRun(ctx, id)
}
func (c *compositeMetadataStore) ListRuns(ctx context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowRun, error) {
	return c.wf.ListRuns(ctx, workflowID, limit, offset)
}
func (c *compositeMetadataStore) UpdateRunStatus(ctx context.Context, id string, status domain.RunStatus, errMsg string, output json.RawMessage) error {
	return c.wf.UpdateRunStatus(ctx, id, status, errMsg, output)
}
func (c *compositeMetadataStore) CreateRunNodes(ctx context.Context, nodes []domain.RunNode) error {
	return c.wf.CreateRunNodes(ctx, nodes)
}
func (c *compositeMetadataStore) GetRunNodes(ctx context.Context, runID string) ([]domain.RunNode, error) {
	return c.wf.GetRunNodes(ctx, runID)
}
func (c *compositeMetadataStore) AcquireReadyNode(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (*domain.RunNode, error) {
	return c.wf.AcquireReadyNode(ctx, leaseOwner, leaseDuration)
}
func (c *compositeMetadataStore) UpdateRunNode(ctx context.Context, node *domain.RunNode) error {
	return c.wf.UpdateRunNode(ctx, node)
}
func (c *compositeMetadataStore) DecrementDeps(ctx context.Context, runID string, nodeKeys []string) error {
	return c.wf.DecrementDeps(ctx, runID, nodeKeys)
}
func (c *compositeMetadataStore) CreateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	return c.wf.CreateNodeAttempt(ctx, a)
}
func (c *compositeMetadataStore) UpdateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	return c.wf.UpdateNodeAttempt(ctx, a)
}
func (c *compositeMetadataStore) GetNodeAttempts(ctx context.Context, runNodeID string) ([]domain.NodeAttempt, error) {
	return c.wf.GetNodeAttempts(ctx, runNodeID)
}

// newCompositeStore creates a store.Store from a mock that also implements
// ScheduleStore and WorkflowStore.
func newCompositeStore(ms *mockMetadataStore, ss *mockScheduleStore, ws *mockWorkflowStore) *store.Store {
	if ss == nil {
		ss = &mockScheduleStore{}
	}
	if ws == nil {
		ws = &mockWorkflowStore{}
	}
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	c := &compositeMetadataStore{mockMetadataStore: ms, sched: ss, wf: ws}
	return store.NewStore(c)
}

// ---------- mock APIKey store ----------

type mockAPIKeyStore struct {
	saveFn      func(ctx context.Context, key *auth.APIKey) error
	getByHashFn func(ctx context.Context, keyHash string) (*auth.APIKey, error)
	getByNameFn func(ctx context.Context, name string) (*auth.APIKey, error)
	listFn      func(ctx context.Context) ([]*auth.APIKey, error)
	deleteFn    func(ctx context.Context, name string) error
}

func (m *mockAPIKeyStore) SaveAPIKey(ctx context.Context, key *auth.APIKey) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, key)
	}
	return nil
}
func (m *mockAPIKeyStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*auth.APIKey, error) {
	if m.getByHashFn != nil {
		return m.getByHashFn(ctx, keyHash)
	}
	return nil, nil
}
func (m *mockAPIKeyStore) GetAPIKeyByName(ctx context.Context, name string) (*auth.APIKey, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(ctx, name)
	}
	return nil, nil
}
func (m *mockAPIKeyStore) ListAPIKeys(ctx context.Context) ([]*auth.APIKey, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}
func (m *mockAPIKeyStore) DeleteAPIKey(ctx context.Context, name string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, name)
	}
	return nil
}

// ---------- mock secrets backend ----------

type mockSecretsBackend struct {
	saveFn   func(ctx context.Context, name, encValue string) error
	getFn    func(ctx context.Context, name string) (string, error)
	deleteFn func(ctx context.Context, name string) error
	listFn   func(ctx context.Context) (map[string]string, error)
	existsFn func(ctx context.Context, name string) (bool, error)
}

func (m *mockSecretsBackend) SaveSecret(ctx context.Context, name, encValue string) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, name, encValue)
	}
	return nil
}
func (m *mockSecretsBackend) GetSecret(ctx context.Context, name string) (string, error) {
	if m.getFn != nil {
		return m.getFn(ctx, name)
	}
	return "", nil
}
func (m *mockSecretsBackend) DeleteSecret(ctx context.Context, name string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, name)
	}
	return nil
}
func (m *mockSecretsBackend) ListSecrets(ctx context.Context) (map[string]string, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return map[string]string{}, nil
}
func (m *mockSecretsBackend) SecretExists(ctx context.Context, name string) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, name)
	}
	return false, nil
}

// expectStatus is a test helper that checks response status code
func expectStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if w.Code != expected {
		t.Fatalf("expected %d, got %d: %s", expected, w.Code, w.Body.String())
	}
}
