package controlplane

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/oriys/nova/internal/cluster"
	"github.com/oriys/nova/internal/store"
)

// RegisterClusterNode registers or updates a cluster node.
func (h *Handler) RegisterClusterNode(w http.ResponseWriter, r *http.Request) {
	var node cluster.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if node.ID == "" {
		http.Error(w, "node id is required", http.StatusBadRequest)
		return
	}
	if node.Name == "" {
		node.Name = node.ID
	}
	if node.State == "" {
		node.State = cluster.NodeStateActive
	}

	if h.ClusterRegistry != nil {
		if err := h.ClusterRegistry.RegisterNode(r.Context(), &node); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		lastHeartbeat := node.LastHeartbeat
		if lastHeartbeat.IsZero() {
			lastHeartbeat = time.Now()
		}
		if err := h.Store.UpsertClusterNode(r.Context(), &store.ClusterNodeRecord{
			ID:            node.ID,
			Name:          node.Name,
			Address:       node.Address,
			State:         string(node.State),
			CPUCores:      node.CPUCores,
			MemoryMB:      node.MemoryMB,
			MaxVMs:        node.MaxVMs,
			ActiveVMs:     node.ActiveVMs,
			QueueDepth:    node.QueueDepth,
			Version:       node.Version,
			Labels:        node.Labels,
			LastHeartbeat: lastHeartbeat,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(node)
}

// HeartbeatClusterNode updates node heartbeat and runtime metrics.
func (h *Handler) HeartbeatClusterNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		http.Error(w, "node id is required", http.StatusBadRequest)
		return
	}

	var metrics cluster.NodeMetrics
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if h.ClusterRegistry != nil {
		if err := h.ClusterRegistry.UpdateHeartbeat(r.Context(), nodeID, &metrics); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	} else {
		if err := h.Store.UpdateClusterNodeHeartbeat(r.Context(), nodeID, metrics.ActiveVMs, metrics.QueueDepth); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListClusterNodes lists all registered cluster nodes.
func (h *Handler) ListClusterNodes(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	nodes, err := h.Store.ListClusterNodes(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []*store.ClusterNodeRecord{}
	}
	total := estimatePaginatedTotal(limit, offset, len(nodes))
	writePaginatedList(w, limit, offset, len(nodes), total, nodes)
}

// ListHealthyClusterNodes returns active nodes ordered by heartbeat recency.
func (h *Handler) ListHealthyClusterNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.Store.ListActiveClusterNodes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []*store.ClusterNodeRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(nodes)
}

// GetClusterNode returns a single cluster node by ID.
func (h *Handler) GetClusterNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	node, err := h.Store.GetClusterNode(r.Context(), id)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(node)
}

// DeleteClusterNode removes a cluster node.
func (h *Handler) DeleteClusterNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteClusterNode(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
