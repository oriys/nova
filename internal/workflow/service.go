package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// Service handles workflow business logic.
type Service struct {
	store *store.Store
}

func NewService(s *store.Store) *Service {
	return &Service{store: s}
}

// CreateWorkflow creates a new workflow.
func (s *Service) CreateWorkflow(ctx context.Context, name, description string) (*domain.Workflow, error) {
	w := &domain.Workflow{
		Name:        name,
		Description: description,
	}
	if err := s.store.CreateWorkflow(ctx, w); err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}
	return w, nil
}

// GetWorkflow retrieves a workflow by name.
func (s *Service) GetWorkflow(ctx context.Context, name string) (*domain.Workflow, error) {
	return s.store.GetWorkflowByName(ctx, name)
}

// ListWorkflows lists all non-deleted workflows.
func (s *Service) ListWorkflows(ctx context.Context, limit, offset int) ([]*domain.Workflow, error) {
	return s.store.ListWorkflows(ctx, limit, offset)
}

// DeleteWorkflow soft-deletes a workflow.
func (s *Service) DeleteWorkflow(ctx context.Context, name string) error {
	w, err := s.store.GetWorkflowByName(ctx, name)
	if err != nil {
		return err
	}
	return s.store.DeleteWorkflow(ctx, w.ID)
}

// PublishVersion validates, persists a new version, and updates the workflow's current_version.
func (s *Service) PublishVersion(ctx context.Context, workflowName string, def *domain.WorkflowDefinition) (*domain.WorkflowVersion, error) {
	// Validate DAG
	topoOrder, err := ValidateDAG(def)
	if err != nil {
		return nil, fmt.Errorf("invalid DAG: %w", err)
	}

	w, err := s.store.GetWorkflowByName(ctx, workflowName)
	if err != nil {
		return nil, err
	}

	nextVersion := w.CurrentVersion + 1

	// Serialize definition
	defJSON, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("marshal definition: %w", err)
	}

	version := &domain.WorkflowVersion{
		WorkflowID: w.ID,
		Version:    nextVersion,
		Definition: defJSON,
	}
	if err := s.store.CreateWorkflowVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("create version: %w", err)
	}

	// Build position map from topological order
	posMap := make(map[string]int, len(topoOrder))
	for i, key := range topoOrder {
		posMap[key] = i
	}

	// Create nodes
	nodes := make([]domain.WorkflowNode, len(def.Nodes))
	nodeKeyToID := make(map[string]string, len(def.Nodes))
	for i, nd := range def.Nodes {
		nodes[i] = domain.WorkflowNode{
			VersionID:    version.ID,
			NodeKey:      nd.NodeKey,
			FunctionName: nd.FunctionName,
			InputMapping: nd.InputMapping,
			RetryPolicy:  nd.RetryPolicy,
			TimeoutS:     nd.TimeoutS,
			Position:     posMap[nd.NodeKey],
		}
		if nodes[i].TimeoutS == 0 {
			nodes[i].TimeoutS = 30
		}
	}
	if err := s.store.CreateWorkflowNodes(ctx, nodes); err != nil {
		return nil, fmt.Errorf("create nodes: %w", err)
	}
	for _, n := range nodes {
		nodeKeyToID[n.NodeKey] = n.ID
	}

	// Create edges
	edges := make([]domain.WorkflowEdge, len(def.Edges))
	for i, ed := range def.Edges {
		edges[i] = domain.WorkflowEdge{
			VersionID:  version.ID,
			FromNodeID: nodeKeyToID[ed.From],
			ToNodeID:   nodeKeyToID[ed.To],
		}
	}
	if len(edges) > 0 {
		if err := s.store.CreateWorkflowEdges(ctx, edges); err != nil {
			return nil, fmt.Errorf("create edges: %w", err)
		}
	}

	// Update workflow current version
	if err := s.store.UpdateWorkflowVersion(ctx, w.ID, nextVersion); err != nil {
		return nil, fmt.Errorf("update workflow version: %w", err)
	}

	// Populate version with nodes/edges for response
	version.Nodes = nodes
	version.Edges = edges

	logging.Op().Info("published workflow version", "workflow", workflowName, "version", nextVersion, "nodes", len(nodes), "edges", len(edges))
	return version, nil
}

// GetVersion retrieves a specific version with nodes and edges.
func (s *Service) GetVersion(ctx context.Context, workflowName string, versionNum int) (*domain.WorkflowVersion, error) {
	w, err := s.store.GetWorkflowByName(ctx, workflowName)
	if err != nil {
		return nil, err
	}
	v, err := s.store.GetWorkflowVersionByNumber(ctx, w.ID, versionNum)
	if err != nil {
		return nil, err
	}
	v.Nodes, err = s.store.GetWorkflowNodes(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	v.Edges, err = s.store.GetWorkflowEdges(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// ListVersions lists all versions of a workflow.
func (s *Service) ListVersions(ctx context.Context, workflowName string, limit, offset int) ([]*domain.WorkflowVersion, error) {
	w, err := s.store.GetWorkflowByName(ctx, workflowName)
	if err != nil {
		return nil, err
	}
	return s.store.ListWorkflowVersions(ctx, w.ID, limit, offset)
}

// TriggerRun materializes a run from the current (or specified) version.
func (s *Service) TriggerRun(ctx context.Context, workflowName string, input json.RawMessage, triggerType string) (*domain.WorkflowRun, error) {
	w, err := s.store.GetWorkflowByName(ctx, workflowName)
	if err != nil {
		return nil, err
	}
	if w.CurrentVersion == 0 {
		return nil, fmt.Errorf("workflow %q has no published versions", workflowName)
	}

	v, err := s.store.GetWorkflowVersionByNumber(ctx, w.ID, w.CurrentVersion)
	if err != nil {
		return nil, err
	}

	// Parse definition to build dependency info
	var def domain.WorkflowDefinition
	if err := json.Unmarshal(v.Definition, &def); err != nil {
		return nil, fmt.Errorf("unmarshal definition: %w", err)
	}

	depMap := BuildDependencyMap(&def)

	// Get nodes for this version
	nodes, err := s.store.GetWorkflowNodes(ctx, v.ID)
	if err != nil {
		return nil, err
	}

	// Create the run
	run := &domain.WorkflowRun{
		WorkflowID:   w.ID,
		WorkflowName: w.Name,
		VersionID:    v.ID,
		Version:      v.Version,
		TriggerType:  triggerType,
		Input:        input,
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	// Materialize run nodes
	runNodes := make([]domain.RunNode, len(nodes))
	for i, n := range nodes {
		deps := len(depMap[n.NodeKey])
		status := domain.NodeStatusPending
		if deps == 0 {
			status = domain.NodeStatusReady
		}
		// Root nodes get the run input
		var nodeInput json.RawMessage
		if deps == 0 && len(input) > 0 {
			nodeInput = input
		}
		runNodes[i] = domain.RunNode{
			RunID:          run.ID,
			NodeID:         n.ID,
			NodeKey:        n.NodeKey,
			FunctionName:   n.FunctionName,
			Status:         status,
			UnresolvedDeps: deps,
			Input:          nodeInput,
		}
	}
	if err := s.store.CreateRunNodes(ctx, runNodes); err != nil {
		return nil, fmt.Errorf("create run nodes: %w", err)
	}

	run.Nodes = runNodes
	logging.Op().Info("triggered workflow run", "workflow", workflowName, "run_id", run.ID, "nodes", len(runNodes))
	return run, nil
}

// GetRun retrieves a run with its node statuses.
func (s *Service) GetRun(ctx context.Context, runID string) (*domain.WorkflowRun, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	run.Nodes, err = s.store.GetRunNodes(ctx, runID)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// ListRuns lists runs for a workflow.
func (s *Service) ListRuns(ctx context.Context, workflowName string, limit, offset int) ([]*domain.WorkflowRun, error) {
	w, err := s.store.GetWorkflowByName(ctx, workflowName)
	if err != nil {
		return nil, err
	}
	return s.store.ListRuns(ctx, w.ID, limit, offset)
}
