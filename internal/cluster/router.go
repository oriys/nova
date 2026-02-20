package cluster

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oriys/nova/internal/domain"
)

// Router combines registry, scheduler, and proxy to route invocations/prewarm
// requests across nodes.
type Router struct {
	registry    *Registry
	scheduler   *Scheduler
	proxy       *Proxy
	localNodeID string
}

func NewRouter(registry *Registry, scheduler *Scheduler, proxy *Proxy, localNodeID string) *Router {
	return &Router{
		registry:    registry,
		scheduler:   scheduler,
		proxy:       proxy,
		localNodeID: localNodeID,
	}
}

// TryRouteInvoke attempts to forward invocation to another healthy node.
// Returns (response, true, nil) when forwarded and successful.
func (r *Router) TryRouteInvoke(ctx context.Context, functionID, functionName string, payload json.RawMessage) (*domain.InvokeResponse, bool, error) {
	node, err := r.pickRemoteNode(ctx, functionID)
	if err != nil || node == nil {
		return nil, false, err
	}

	raw, err := r.proxy.ForwardInvoke(ctx, node.Address, functionName, payload)
	if err != nil {
		return nil, false, err
	}

	var resp domain.InvokeResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, false, fmt.Errorf("decode remote invoke response: %w", err)
	}

	r.scheduler.RecordFunctionPlacement(functionID, node.ID)
	return &resp, true, nil
}

// TryRoutePrewarm attempts to prewarm on another healthy node.
// Returns (targetNodeID, true, nil) when dispatched remotely.
func (r *Router) TryRoutePrewarm(ctx context.Context, functionID, functionName string, targetReplicas int) (string, bool, error) {
	node, err := r.pickRemoteNode(ctx, functionID)
	if err != nil || node == nil {
		return "", false, err
	}

	if err := r.proxy.ForwardPrewarm(ctx, node.Address, functionName, targetReplicas); err != nil {
		return "", false, err
	}

	r.scheduler.RecordFunctionPlacement(functionID, node.ID)
	return node.ID, true, nil
}

func (r *Router) pickRemoteNode(ctx context.Context, functionID string) (*Node, error) {
	if r == nil || r.registry == nil || r.scheduler == nil || r.proxy == nil || r.localNodeID == "" {
		return nil, nil
	}

	if err := r.registry.SyncFromStore(ctx); err != nil {
		return nil, fmt.Errorf("sync cluster registry: %w", err)
	}
	if _, err := r.registry.GetNode(r.localNodeID); err != nil {
		// Local node is not part of the active registry yet; avoid forwarding
		// until cluster membership has converged.
		return nil, nil
	}

	node, err := r.scheduler.SelectNodeForFunction(functionID)
	if err != nil || node == nil {
		return nil, nil
	}
	if node.ID == "" || node.ID == r.localNodeID {
		return nil, nil
	}
	return node, nil
}
