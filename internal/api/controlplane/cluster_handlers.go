package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/store"
)

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
