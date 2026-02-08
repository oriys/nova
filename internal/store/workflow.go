package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

// WorkflowStore defines all workflow-related persistence operations.
type WorkflowStore interface {
	// Workflows
	CreateWorkflow(ctx context.Context, w *domain.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error)
	GetWorkflowByName(ctx context.Context, name string) (*domain.Workflow, error)
	ListWorkflows(ctx context.Context) ([]*domain.Workflow, error)
	DeleteWorkflow(ctx context.Context, id string) error
	UpdateWorkflowVersion(ctx context.Context, id string, version int) error

	// Versions
	CreateWorkflowVersion(ctx context.Context, v *domain.WorkflowVersion) error
	GetWorkflowVersion(ctx context.Context, id string) (*domain.WorkflowVersion, error)
	GetWorkflowVersionByNumber(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error)
	ListWorkflowVersions(ctx context.Context, workflowID string) ([]*domain.WorkflowVersion, error)

	// Nodes & Edges (bulk create)
	CreateWorkflowNodes(ctx context.Context, nodes []domain.WorkflowNode) error
	CreateWorkflowEdges(ctx context.Context, edges []domain.WorkflowEdge) error
	GetWorkflowNodes(ctx context.Context, versionID string) ([]domain.WorkflowNode, error)
	GetWorkflowNodeByID(ctx context.Context, nodeID string) (*domain.WorkflowNode, error)
	GetWorkflowEdges(ctx context.Context, versionID string) ([]domain.WorkflowEdge, error)

	// Runs
	CreateRun(ctx context.Context, run *domain.WorkflowRun) error
	GetRun(ctx context.Context, id string) (*domain.WorkflowRun, error)
	ListRuns(ctx context.Context, workflowID string) ([]*domain.WorkflowRun, error)
	UpdateRunStatus(ctx context.Context, id string, status domain.RunStatus, errMsg string, output json.RawMessage) error

	// Run Nodes
	CreateRunNodes(ctx context.Context, nodes []domain.RunNode) error
	GetRunNodes(ctx context.Context, runID string) ([]domain.RunNode, error)
	AcquireReadyNode(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (*domain.RunNode, error)
	UpdateRunNode(ctx context.Context, node *domain.RunNode) error
	DecrementDeps(ctx context.Context, runID string, nodeKeys []string) error

	// Attempts
	CreateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error
	UpdateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error
	GetNodeAttempts(ctx context.Context, runNodeID string) ([]domain.NodeAttempt, error)
}

// --- PostgresStore implements WorkflowStore ---

func (s *PostgresStore) CreateWorkflow(ctx context.Context, w *domain.Workflow) error {
	scope := tenantScopeFromContext(ctx)
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now
	if w.Status == "" {
		w.Status = domain.WorkflowStatusActive
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO dag_workflows (id, tenant_id, namespace, name, description, status, current_version, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		w.ID, scope.TenantID, scope.Namespace, w.Name, w.Description, w.Status, w.CurrentVersion, w.CreatedAt, w.UpdatedAt)
	return err
}

func (s *PostgresStore) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	scope := tenantScopeFromContext(ctx)
	w := &domain.Workflow{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description, status, current_version, created_at, updated_at
		 FROM dag_workflows WHERE id = $1 AND tenant_id = $2 AND namespace = $3`, id, scope.TenantID, scope.Namespace).
		Scan(&w.ID, &w.Name, &w.Description, &w.Status, &w.CurrentVersion, &w.CreatedAt, &w.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	return w, err
}

func (s *PostgresStore) GetWorkflowByName(ctx context.Context, name string) (*domain.Workflow, error) {
	scope := tenantScopeFromContext(ctx)
	w := &domain.Workflow{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description, status, current_version, created_at, updated_at
		 FROM dag_workflows
		 WHERE name = $1 AND tenant_id = $2 AND namespace = $3`,
		name, scope.TenantID, scope.Namespace).
		Scan(&w.ID, &w.Name, &w.Description, &w.Status, &w.CurrentVersion, &w.CreatedAt, &w.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", name)
	}
	return w, err
}

func (s *PostgresStore) ListWorkflows(ctx context.Context) ([]*domain.Workflow, error) {
	scope := tenantScopeFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, status, current_version, created_at, updated_at
		 FROM dag_workflows
		 WHERE status != 'deleted' AND tenant_id = $1 AND namespace = $2
		 ORDER BY created_at DESC`,
		scope.TenantID, scope.Namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.Workflow
	for rows.Next() {
		w := &domain.Workflow{}
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &w.Status, &w.CurrentVersion, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *PostgresStore) DeleteWorkflow(ctx context.Context, id string) error {
	scope := tenantScopeFromContext(ctx)
	ct, err := s.pool.Exec(ctx,
		`UPDATE dag_workflows
		 SET status = 'deleted', updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2 AND namespace = $3`,
		id, scope.TenantID, scope.Namespace)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return err
}

func (s *PostgresStore) UpdateWorkflowVersion(ctx context.Context, id string, version int) error {
	scope := tenantScopeFromContext(ctx)
	ct, err := s.pool.Exec(ctx,
		`UPDATE dag_workflows
		 SET current_version = $2, updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $3 AND namespace = $4`,
		id, version, scope.TenantID, scope.Namespace)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

// --- Versions ---

func (s *PostgresStore) CreateWorkflowVersion(ctx context.Context, v *domain.WorkflowVersion) error {
	if v.ID == "" {
		v.ID = uuid.New().String()
	}
	v.CreatedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO dag_workflow_versions (id, workflow_id, version, definition, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		v.ID, v.WorkflowID, v.Version, v.Definition, v.CreatedAt)
	return err
}

func (s *PostgresStore) GetWorkflowVersion(ctx context.Context, id string) (*domain.WorkflowVersion, error) {
	v := &domain.WorkflowVersion{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, workflow_id, version, definition, created_at
		 FROM dag_workflow_versions WHERE id = $1`, id).
		Scan(&v.ID, &v.WorkflowID, &v.Version, &v.Definition, &v.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workflow version not found: %s", id)
	}
	return v, err
}

func (s *PostgresStore) GetWorkflowVersionByNumber(ctx context.Context, workflowID string, version int) (*domain.WorkflowVersion, error) {
	v := &domain.WorkflowVersion{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, workflow_id, version, definition, created_at
		 FROM dag_workflow_versions WHERE workflow_id = $1 AND version = $2`, workflowID, version).
		Scan(&v.ID, &v.WorkflowID, &v.Version, &v.Definition, &v.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workflow version not found: %d", version)
	}
	return v, err
}

func (s *PostgresStore) ListWorkflowVersions(ctx context.Context, workflowID string) ([]*domain.WorkflowVersion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workflow_id, version, definition, created_at
		 FROM dag_workflow_versions WHERE workflow_id = $1 ORDER BY version DESC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.WorkflowVersion
	for rows.Next() {
		v := &domain.WorkflowVersion{}
		if err := rows.Scan(&v.ID, &v.WorkflowID, &v.Version, &v.Definition, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- Nodes & Edges ---

func (s *PostgresStore) CreateWorkflowNodes(ctx context.Context, nodes []domain.WorkflowNode) error {
	for i := range nodes {
		if nodes[i].ID == "" {
			nodes[i].ID = uuid.New().String()
		}
		retryJSON, _ := json.Marshal(nodes[i].RetryPolicy)
		_, err := s.pool.Exec(ctx,
			`INSERT INTO dag_workflow_nodes (id, version_id, node_key, function_name, input_mapping, retry_policy, timeout_s, position)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			nodes[i].ID, nodes[i].VersionID, nodes[i].NodeKey, nodes[i].FunctionName,
			nodes[i].InputMapping, retryJSON, nodes[i].TimeoutS, nodes[i].Position)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) CreateWorkflowEdges(ctx context.Context, edges []domain.WorkflowEdge) error {
	for i := range edges {
		if edges[i].ID == "" {
			edges[i].ID = uuid.New().String()
		}
		_, err := s.pool.Exec(ctx,
			`INSERT INTO dag_workflow_edges (id, version_id, from_node_id, to_node_id)
			 VALUES ($1, $2, $3, $4)`,
			edges[i].ID, edges[i].VersionID, edges[i].FromNodeID, edges[i].ToNodeID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) GetWorkflowNodes(ctx context.Context, versionID string) ([]domain.WorkflowNode, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, version_id, node_key, function_name, input_mapping, retry_policy, timeout_s, position
		 FROM dag_workflow_nodes WHERE version_id = $1 ORDER BY position`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.WorkflowNode
	for rows.Next() {
		n := domain.WorkflowNode{}
		var retryJSON []byte
		if err := rows.Scan(&n.ID, &n.VersionID, &n.NodeKey, &n.FunctionName,
			&n.InputMapping, &retryJSON, &n.TimeoutS, &n.Position); err != nil {
			return nil, err
		}
		if len(retryJSON) > 0 && string(retryJSON) != "null" {
			n.RetryPolicy = &domain.RetryPolicy{}
			json.Unmarshal(retryJSON, n.RetryPolicy)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetWorkflowNodeByID(ctx context.Context, nodeID string) (*domain.WorkflowNode, error) {
	n := &domain.WorkflowNode{}
	var retryJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, version_id, node_key, function_name, input_mapping, retry_policy, timeout_s, position
		 FROM dag_workflow_nodes WHERE id = $1`, nodeID).
		Scan(&n.ID, &n.VersionID, &n.NodeKey, &n.FunctionName, &n.InputMapping, &retryJSON, &n.TimeoutS, &n.Position)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workflow node not found: %s", nodeID)
	}
	if err != nil {
		return nil, err
	}
	if len(retryJSON) > 0 && string(retryJSON) != "null" {
		n.RetryPolicy = &domain.RetryPolicy{}
		json.Unmarshal(retryJSON, n.RetryPolicy)
	}
	return n, nil
}

func (s *PostgresStore) GetWorkflowEdges(ctx context.Context, versionID string) ([]domain.WorkflowEdge, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, version_id, from_node_id, to_node_id
		 FROM dag_workflow_edges WHERE version_id = $1`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.WorkflowEdge
	for rows.Next() {
		e := domain.WorkflowEdge{}
		if err := rows.Scan(&e.ID, &e.VersionID, &e.FromNodeID, &e.ToNodeID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- Runs ---

func (s *PostgresStore) CreateRun(ctx context.Context, run *domain.WorkflowRun) error {
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	run.StartedAt = now
	if run.Status == "" {
		run.Status = domain.RunStatusRunning
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO dag_runs (id, workflow_id, version_id, status, trigger_type, input, started_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		run.ID, run.WorkflowID, run.VersionID, run.Status, run.TriggerType, run.Input, run.StartedAt, run.CreatedAt)
	return err
}

func (s *PostgresStore) GetRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	scope := tenantScopeFromContext(ctx)
	r := &domain.WorkflowRun{}
	err := s.pool.QueryRow(ctx,
		`SELECT r.id, r.workflow_id, w.name, r.version_id, v.version, r.status, r.trigger_type,
		        r.input, r.output, COALESCE(r.error_message, ''), r.started_at, r.finished_at, r.created_at
		 FROM dag_runs r
		 JOIN dag_workflows w ON w.id = r.workflow_id
		 JOIN dag_workflow_versions v ON v.id = r.version_id
		 WHERE r.id = $1 AND w.tenant_id = $2 AND w.namespace = $3`,
		id, scope.TenantID, scope.Namespace).
		Scan(&r.ID, &r.WorkflowID, &r.WorkflowName, &r.VersionID, &r.Version, &r.Status, &r.TriggerType,
			&r.Input, &r.Output, &r.ErrorMessage, &r.StartedAt, &r.FinishedAt, &r.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("run not found: %s", id)
	}
	return r, err
}

func (s *PostgresStore) ListRuns(ctx context.Context, workflowID string) ([]*domain.WorkflowRun, error) {
	scope := tenantScopeFromContext(ctx)
	rows, err := s.pool.Query(ctx,
		`SELECT r.id, r.workflow_id, w.name, r.version_id, v.version, r.status, r.trigger_type,
		        r.input, r.output, COALESCE(r.error_message, ''), r.started_at, r.finished_at, r.created_at
		 FROM dag_runs r
		 JOIN dag_workflows w ON w.id = r.workflow_id
		 JOIN dag_workflow_versions v ON v.id = r.version_id
		 WHERE r.workflow_id = $1 AND w.tenant_id = $2 AND w.namespace = $3
		 ORDER BY r.created_at DESC LIMIT 100`,
		workflowID, scope.TenantID, scope.Namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.WorkflowRun
	for rows.Next() {
		r := &domain.WorkflowRun{}
		if err := rows.Scan(&r.ID, &r.WorkflowID, &r.WorkflowName, &r.VersionID, &r.Version, &r.Status, &r.TriggerType,
			&r.Input, &r.Output, &r.ErrorMessage, &r.StartedAt, &r.FinishedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateRunStatus(ctx context.Context, id string, status domain.RunStatus, errMsg string, output json.RawMessage) error {
	var finishedAt *time.Time
	if status == domain.RunStatusSucceeded || status == domain.RunStatusFailed || status == domain.RunStatusCancelled {
		now := time.Now().UTC()
		finishedAt = &now
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE dag_runs SET status = $2, error_message = $3, output = $4, finished_at = $5
		 WHERE id = $1 AND status = 'running'`,
		id, status, errMsg, output, finishedAt)
	return err
}

// --- Run Nodes ---

func (s *PostgresStore) CreateRunNodes(ctx context.Context, nodes []domain.RunNode) error {
	for i := range nodes {
		if nodes[i].ID == "" {
			nodes[i].ID = uuid.New().String()
		}
		nodes[i].CreatedAt = time.Now().UTC()
		_, err := s.pool.Exec(ctx,
			`INSERT INTO dag_run_nodes (id, run_id, node_id, node_key, function_name, status, unresolved_deps, attempt, input, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			nodes[i].ID, nodes[i].RunID, nodes[i].NodeID, nodes[i].NodeKey, nodes[i].FunctionName,
			nodes[i].Status, nodes[i].UnresolvedDeps, nodes[i].Attempt, nodes[i].Input, nodes[i].CreatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) GetRunNodes(ctx context.Context, runID string) ([]domain.RunNode, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, run_id, node_id, node_key, function_name, status, unresolved_deps,
		        attempt, input, output, COALESCE(error_message, ''), COALESCE(lease_owner, ''), lease_expires_at,
		        started_at, finished_at, created_at
		 FROM dag_run_nodes WHERE run_id = $1 ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.RunNode
	for rows.Next() {
		n := domain.RunNode{}
		if err := rows.Scan(&n.ID, &n.RunID, &n.NodeID, &n.NodeKey, &n.FunctionName,
			&n.Status, &n.UnresolvedDeps, &n.Attempt, &n.Input, &n.Output, &n.ErrorMessage,
			&n.LeaseOwner, &n.LeaseExpiresAt, &n.StartedAt, &n.FinishedAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AcquireReadyNode atomically claims one ready node (or a node with an expired lease).
func (s *PostgresStore) AcquireReadyNode(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (*domain.RunNode, error) {
	now := time.Now().UTC()
	leaseExpires := now.Add(leaseDuration)

	n := &domain.RunNode{}
	err := s.pool.QueryRow(ctx,
		`WITH candidate AS (
			SELECT rn.id, w.tenant_id, w.namespace
			FROM dag_run_nodes rn
			JOIN dag_runs r ON r.id = rn.run_id
			JOIN dag_workflows w ON w.id = r.workflow_id
			WHERE (rn.status = 'ready') OR (rn.status = 'running' AND rn.lease_expires_at < $3)
			ORDER BY rn.created_at
			FOR UPDATE OF rn SKIP LOCKED
			LIMIT 1
		),
		updated AS (
			UPDATE dag_run_nodes rn
			SET status = 'running',
				lease_owner = $1,
				lease_expires_at = $2,
				started_at = $3,
				attempt = rn.attempt + 1
			FROM candidate c
			WHERE rn.id = c.id
			RETURNING rn.id, c.tenant_id, c.namespace, rn.run_id, rn.node_id, rn.node_key, rn.function_name, rn.status, rn.unresolved_deps,
			          rn.attempt, rn.input, rn.output, COALESCE(rn.error_message, '') AS error_message, COALESCE(rn.lease_owner, '') AS lease_owner, rn.lease_expires_at,
			          rn.started_at, rn.finished_at, rn.created_at
		)
		SELECT id, tenant_id, namespace, run_id, node_id, node_key, function_name, status, unresolved_deps,
		       attempt, input, output, error_message, lease_owner, lease_expires_at, started_at, finished_at, created_at
		FROM updated`,
		leaseOwner, leaseExpires, now).
		Scan(&n.ID, &n.TenantID, &n.Namespace, &n.RunID, &n.NodeID, &n.NodeKey, &n.FunctionName,
			&n.Status, &n.UnresolvedDeps, &n.Attempt, &n.Input, &n.Output, &n.ErrorMessage,
			&n.LeaseOwner, &n.LeaseExpiresAt, &n.StartedAt, &n.FinishedAt, &n.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil // No work available
	}
	return n, err
}

func (s *PostgresStore) UpdateRunNode(ctx context.Context, node *domain.RunNode) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE dag_run_nodes SET
			status = $2, attempt = $3, input = $4, output = $5, error_message = $6,
			lease_owner = $7, lease_expires_at = $8, started_at = $9, finished_at = $10
		 WHERE id = $1`,
		node.ID, node.Status, node.Attempt, node.Input, node.Output, node.ErrorMessage,
		node.LeaseOwner, node.LeaseExpiresAt, node.StartedAt, node.FinishedAt)
	return err
}

// DecrementDeps decrements unresolved_deps for the given node keys in a run,
// and promotes nodes with 0 deps to 'ready'.
func (s *PostgresStore) DecrementDeps(ctx context.Context, runID string, nodeKeys []string) error {
	if len(nodeKeys) == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE dag_run_nodes
		 SET unresolved_deps = unresolved_deps - 1,
		     status = CASE WHEN unresolved_deps - 1 <= 0 THEN 'ready' ELSE status END
		 WHERE run_id = $1 AND node_key = ANY($2) AND status = 'pending'`,
		runID, nodeKeys)
	return err
}

// --- Attempts ---

func (s *PostgresStore) CreateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	a.StartedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO dag_node_attempts (id, run_node_id, attempt, status, input, started_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		a.ID, a.RunNodeID, a.Attempt, a.Status, a.Input, a.StartedAt)
	return err
}

func (s *PostgresStore) UpdateNodeAttempt(ctx context.Context, a *domain.NodeAttempt) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE dag_node_attempts SET status = $2, output = $3, error = $4, duration_ms = $5, finished_at = $6
		 WHERE id = $1`,
		a.ID, a.Status, a.Output, a.Error, a.DurationMs, a.FinishedAt)
	return err
}

func (s *PostgresStore) GetNodeAttempts(ctx context.Context, runNodeID string) ([]domain.NodeAttempt, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, run_node_id, attempt, status, input, output, COALESCE(error, ''), duration_ms, started_at, finished_at
		 FROM dag_node_attempts WHERE run_node_id = $1 ORDER BY attempt`, runNodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.NodeAttempt
	for rows.Next() {
		a := domain.NodeAttempt{}
		if err := rows.Scan(&a.ID, &a.RunNodeID, &a.Attempt, &a.Status, &a.Input, &a.Output,
			&a.Error, &a.DurationMs, &a.StartedAt, &a.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
