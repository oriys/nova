package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// EngineConfig configures the workflow engine.
type EngineConfig struct {
	Workers       int           // Number of worker goroutines (default 4)
	PollInterval  time.Duration // How often workers poll for work (default 1s)
	LeaseDuration time.Duration // How long a node lease lasts (default 5m)
}

// Engine is the worker pool that polls for ready DAG nodes and executes them.
type Engine struct {
	store    *store.Store
	exec     *executor.Executor
	cfg      EngineConfig
	ownerID  string
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewEngine creates a new workflow engine.
func NewEngine(s *store.Store, exec *executor.Executor, cfg EngineConfig) *Engine {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 5 * time.Minute
	}
	return &Engine{
		store:   s,
		exec:    exec,
		cfg:     cfg,
		ownerID: fmt.Sprintf("engine-%d", time.Now().UnixNano()),
		stopCh:  make(chan struct{}),
	}
}

// Start launches the worker goroutines.
func (e *Engine) Start() {
	logging.Op().Info("starting workflow engine", "workers", e.cfg.Workers, "poll_interval", e.cfg.PollInterval)
	for i := 0; i < e.cfg.Workers; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}
}

// Stop gracefully shuts down the engine.
func (e *Engine) Stop() {
	logging.Op().Info("stopping workflow engine")
	close(e.stopCh)
	e.wg.Wait()
	logging.Op().Info("workflow engine stopped")
}

func (e *Engine) worker(id int) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.poll(id)
		}
	}
}

func (e *Engine) poll(workerID int) {
	ctx := context.Background()

	node, err := e.store.AcquireReadyNode(ctx, e.ownerID, e.cfg.LeaseDuration)
	if err != nil {
		logging.Op().Error("acquire ready node", "worker", workerID, "error", err)
		return
	}
	if node == nil {
		return // No work available
	}

	logging.Op().Info("executing node", "worker", workerID, "run_id", node.RunID, "node_key", node.NodeKey, "function", node.FunctionName, "attempt", node.Attempt)

	e.executeNode(ctx, node)
}

func (e *Engine) executeNode(ctx context.Context, node *domain.RunNode) {
	// Look up the workflow node for retry policy and timeout
	wfNode, err := e.getWorkflowNode(ctx, node.NodeID)
	if err != nil {
		logging.Op().Error("get workflow node", "node_id", node.NodeID, "error", err)
		e.failNode(ctx, node, fmt.Sprintf("internal: %v", err))
		return
	}

	// Set timeout
	timeout := time.Duration(wfNode.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build input: if node has explicit input, use it; otherwise use empty object
	payload := node.Input
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	// Record attempt
	attempt := &domain.NodeAttempt{
		RunNodeID: node.ID,
		Attempt:   node.Attempt,
		Status:    domain.NodeStatusRunning,
		Input:     payload,
	}
	if err := e.store.CreateNodeAttempt(ctx, attempt); err != nil {
		logging.Op().Error("create attempt", "error", err)
	}

	start := time.Now()

	// Execute via the function executor
	resp, invokeErr := e.exec.Invoke(execCtx, node.FunctionName, payload)

	duration := time.Since(start)

	// Update attempt
	now := time.Now().UTC()
	attempt.DurationMs = duration.Milliseconds()
	attempt.FinishedAt = &now

	if invokeErr != nil || (resp != nil && resp.Error != "") {
		errMsg := ""
		if invokeErr != nil {
			errMsg = invokeErr.Error()
		} else {
			errMsg = resp.Error
		}
		attempt.Status = domain.NodeStatusFailed
		attempt.Error = errMsg
		e.store.UpdateNodeAttempt(ctx, attempt)

		// Check retry
		maxAttempts := 1
		if wfNode.RetryPolicy != nil && wfNode.RetryPolicy.MaxAttempts > 1 {
			maxAttempts = wfNode.RetryPolicy.MaxAttempts
		}
		if node.Attempt < maxAttempts {
			// Schedule retry by resetting to ready
			e.retryNode(ctx, node, wfNode, errMsg)
			return
		}

		// Final failure
		e.failNode(ctx, node, errMsg)
		return
	}

	// Success
	attempt.Status = domain.NodeStatusSucceeded
	if resp != nil {
		attempt.Output = resp.Output
	}
	e.store.UpdateNodeAttempt(ctx, attempt)

	e.succeedNode(ctx, node, resp)
}

func (e *Engine) succeedNode(ctx context.Context, node *domain.RunNode, resp *domain.InvokeResponse) {
	now := time.Now().UTC()
	node.Status = domain.NodeStatusSucceeded
	node.FinishedAt = &now
	node.LeaseOwner = ""
	node.LeaseExpiresAt = nil
	if resp != nil {
		node.Output = resp.Output
	}
	if err := e.store.UpdateRunNode(ctx, node); err != nil {
		logging.Op().Error("update run node", "error", err)
		return
	}

	// Advance DAG: find successors and decrement their deps
	e.advanceDAG(ctx, node)

	// Check run completion
	e.checkRunCompletion(ctx, node.RunID)
}

func (e *Engine) failNode(ctx context.Context, node *domain.RunNode, errMsg string) {
	now := time.Now().UTC()
	node.Status = domain.NodeStatusFailed
	node.ErrorMessage = errMsg
	node.FinishedAt = &now
	node.LeaseOwner = ""
	node.LeaseExpiresAt = nil
	if err := e.store.UpdateRunNode(ctx, node); err != nil {
		logging.Op().Error("update run node failed", "error", err)
		return
	}

	// Fail the run
	e.store.UpdateRunStatus(ctx, node.RunID, domain.RunStatusFailed, fmt.Sprintf("node %q failed: %s", node.NodeKey, errMsg), nil)
}

func (e *Engine) retryNode(ctx context.Context, node *domain.RunNode, wfNode *domain.WorkflowNode, errMsg string) {
	// Calculate backoff
	backoff := e.calcBackoff(node.Attempt, wfNode.RetryPolicy)

	logging.Op().Info("retrying node", "node_key", node.NodeKey, "attempt", node.Attempt, "backoff", backoff)

	// Reset to ready (the next poll will pick it up after the backoff)
	// For simplicity, we use ready status; a production system might use a scheduled_at field
	node.Status = domain.NodeStatusReady
	node.ErrorMessage = errMsg
	node.LeaseOwner = ""
	node.LeaseExpiresAt = nil
	node.FinishedAt = nil
	if err := e.store.UpdateRunNode(ctx, node); err != nil {
		logging.Op().Error("retry node", "error", err)
	}
}

func (e *Engine) advanceDAG(ctx context.Context, completed *domain.RunNode) {
	// Get the workflow version's edges to find successors
	wfNode, err := e.getWorkflowNode(ctx, completed.NodeID)
	if err != nil {
		logging.Op().Error("get workflow node for advance", "error", err)
		return
	}

	edges, err := e.store.GetWorkflowEdges(ctx, wfNode.VersionID)
	if err != nil {
		logging.Op().Error("get edges for advance", "error", err)
		return
	}

	// Find successor node keys
	var successorKeys []string
	nodeIDToKey := make(map[string]string)
	nodes, _ := e.store.GetWorkflowNodes(ctx, wfNode.VersionID)
	for _, n := range nodes {
		nodeIDToKey[n.ID] = n.NodeKey
	}

	for _, edge := range edges {
		if edge.FromNodeID == completed.NodeID {
			if key, ok := nodeIDToKey[edge.ToNodeID]; ok {
				successorKeys = append(successorKeys, key)
			}
		}
	}

	if len(successorKeys) == 0 {
		return
	}

	// Decrement deps and promote to ready
	if err := e.store.DecrementDeps(ctx, completed.RunID, successorKeys); err != nil {
		logging.Op().Error("decrement deps", "error", err)
		return
	}

	// Propagate output as input to successors that just became ready
	e.propagateInput(ctx, completed)
}

func (e *Engine) propagateInput(ctx context.Context, completed *domain.RunNode) {
	// Get all run nodes for this run to find successors
	runNodes, err := e.store.GetRunNodes(ctx, completed.RunID)
	if err != nil {
		logging.Op().Error("get run nodes for propagation", "error", err)
		return
	}

	// Get edges
	wfNode, _ := e.getWorkflowNode(ctx, completed.NodeID)
	if wfNode == nil {
		return
	}
	edges, _ := e.store.GetWorkflowEdges(ctx, wfNode.VersionID)
	nodes, _ := e.store.GetWorkflowNodes(ctx, wfNode.VersionID)
	nodeIDToKey := make(map[string]string)
	for _, n := range nodes {
		nodeIDToKey[n.ID] = n.NodeKey
	}

	// Build predecessor map (to_node_key -> [from_node_key])
	predMap := make(map[string][]string)
	for _, edge := range edges {
		fromKey := nodeIDToKey[edge.FromNodeID]
		toKey := nodeIDToKey[edge.ToNodeID]
		predMap[toKey] = append(predMap[toKey], fromKey)
	}

	// Build runNode map by key
	rnByKey := make(map[string]*domain.RunNode)
	for i := range runNodes {
		rnByKey[runNodes[i].NodeKey] = &runNodes[i]
	}

	// For each successor that just became ready, build input
	for _, edge := range edges {
		if edge.FromNodeID != completed.NodeID {
			continue
		}
		succKey := nodeIDToKey[edge.ToNodeID]
		succNode := rnByKey[succKey]
		if succNode == nil || succNode.Status != domain.NodeStatusReady {
			continue
		}

		preds := predMap[succKey]
		if len(preds) == 1 {
			// Single predecessor: pass output directly
			succNode.Input = completed.Output
		} else {
			// Multiple predecessors: merge by node_key
			merged := make(map[string]json.RawMessage)
			for _, predKey := range preds {
				predNode := rnByKey[predKey]
				if predNode != nil && predNode.Status == domain.NodeStatusSucceeded {
					merged[predKey] = predNode.Output
				}
			}
			mergedJSON, _ := json.Marshal(merged)
			succNode.Input = mergedJSON
		}
		e.store.UpdateRunNode(ctx, succNode)
	}
}

func (e *Engine) checkRunCompletion(ctx context.Context, runID string) {
	runNodes, err := e.store.GetRunNodes(ctx, runID)
	if err != nil {
		logging.Op().Error("check run completion", "error", err)
		return
	}

	allTerminal := true
	anyFailed := false
	var lastOutput json.RawMessage

	for _, n := range runNodes {
		switch n.Status {
		case domain.NodeStatusSucceeded:
			lastOutput = n.Output
		case domain.NodeStatusFailed:
			anyFailed = true
		case domain.NodeStatusSkipped:
			// terminal
		default:
			allTerminal = false
		}
	}

	if !allTerminal {
		return
	}

	if anyFailed {
		e.store.UpdateRunStatus(ctx, runID, domain.RunStatusFailed, "one or more nodes failed", nil)
	} else {
		e.store.UpdateRunStatus(ctx, runID, domain.RunStatusSucceeded, "", lastOutput)
	}
}

func (e *Engine) getWorkflowNode(ctx context.Context, nodeID string) (*domain.WorkflowNode, error) {
	return e.store.GetWorkflowNodeByID(ctx, nodeID)
}

func (e *Engine) calcBackoff(attempt int, policy *domain.RetryPolicy) time.Duration {
	if policy == nil {
		return time.Second
	}
	baseMS := policy.BaseMS
	if baseMS <= 0 {
		baseMS = 1000
	}
	maxMS := policy.MaxBackoffMS
	if maxMS <= 0 {
		maxMS = 30000
	}

	ms := float64(baseMS) * math.Pow(2, float64(attempt-1))
	if ms > float64(maxMS) {
		ms = float64(maxMS)
	}

	// Add ±25% jitter
	jitter := ms * 0.25 * (2*rand.Float64() - 1)
	ms += jitter

	return time.Duration(ms) * time.Millisecond
}
