package controlplane

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/oriys/nova/internal/store"
)

// ListAuditLogs returns paginated audit log entries with optional filters.
func (h *Handler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 50, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	filter := &store.AuditLogFilter{
		Actor:        r.URL.Query().Get("actor"),
		ResourceType: r.URL.Query().Get("resource_type"),
		ResourceName: r.URL.Query().Get("resource_name"),
		Action:       r.URL.Query().Get("action"),
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			filter.Since = &t
		}
	}
	if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			filter.Until = &t
		}
	}

	logs, err := h.Store.ListAuditLogs(r.Context(), filter, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []*store.AuditLog{}
	}

	total, _ := h.Store.CountAuditLogs(r.Context(), filter)
	writePaginatedList(w, limit, offset, len(logs), total, logs)
}

// GetAuditLog returns a single audit log entry by ID.
func (h *Handler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	log, err := h.Store.GetAuditLog(r.Context(), id)
	if err != nil {
		http.Error(w, "audit log not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(log)
}
