package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/store"
)

func parseNotificationStatus(raw string) store.NotificationStatus {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "unread":
		return store.NotificationStatusUnread
	case "read":
		return store.NotificationStatusRead
	default:
		return store.NotificationStatusAll
	}
}

// ListNotifications handles GET /notifications
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 20, 200)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)
	status := parseNotificationStatus(r.URL.Query().Get("status"))

	items, err := h.Store.ListNotifications(r.Context(), limit, offset, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []*store.NotificationRecord{}
	}
	total := estimatePaginatedTotal(limit, offset, len(items))
	writePaginatedList(w, limit, offset, len(items), total, items)
}

// GetUnreadNotificationCount handles GET /notifications/unread-count
func (h *Handler) GetUnreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.Store.GetUnreadNotificationCount(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"unread": count})
}

// MarkNotificationRead handles POST /notifications/{id}/read
func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "notification id is required", http.StatusBadRequest)
		return
	}

	item, err := h.Store.MarkNotificationRead(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "notification not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// MarkAllNotificationsRead handles POST /notifications/read-all
func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	updated, err := h.Store.MarkAllNotificationsRead(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"updated": updated})
}
