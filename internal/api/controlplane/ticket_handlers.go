package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// ── Ticket Handlers ─────────────────────────────────────────────────────────

// CreateTicket handles POST /tickets
func (h *Handler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string                `json:"title"`
		Description string                `json:"description"`
		Priority    domain.TicketPriority `json:"priority"`
		Category    domain.TicketCategory `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	ticket := &domain.Ticket{
		ID:          uuid.NewString(),
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Category:    req.Category,
		Status:      domain.TicketStatusOpen,
	}

	if err := h.Store.CreateTicket(r.Context(), ticket); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ticket)
}

// ListTickets handles GET /tickets
func (h *Handler) ListTickets(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 20, 200)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	items, total, err := h.Store.ListTickets(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []*domain.Ticket{}
	}
	writePaginatedList(w, limit, offset, len(items), int64(total), items)
}

// GetTicket handles GET /tickets/{id}
func (h *Handler) GetTicket(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "ticket id is required", http.StatusBadRequest)
		return
	}

	ticket, err := h.Store.GetTicket(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ticket)
}

// UpdateTicket handles PATCH /tickets/{id}
func (h *Handler) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "ticket id is required", http.StatusBadRequest)
		return
	}

	var update store.TicketUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ticket, err := h.Store.UpdateTicket(r.Context(), id, &update)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ticket)
}

// DeleteTicket handles DELETE /tickets/{id}
func (h *Handler) DeleteTicket(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "ticket id is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteTicket(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "only the default tenant") {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CreateTicketComment handles POST /tickets/{id}/comments
func (h *Handler) CreateTicketComment(w http.ResponseWriter, r *http.Request) {
	ticketID := strings.TrimSpace(r.PathValue("id"))
	if ticketID == "" {
		http.Error(w, "ticket id is required", http.StatusBadRequest)
		return
	}

	var req struct {
		Content  string `json:"content"`
		Internal bool   `json:"internal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	comment := &domain.TicketComment{
		ID:       fmt.Sprintf("tc-%s", uuid.NewString()),
		TicketID: ticketID,
		Content:  req.Content,
		Internal: req.Internal,
	}

	if err := h.Store.CreateTicketComment(r.Context(), comment); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(comment)
}

// ListTicketComments handles GET /tickets/{id}/comments
func (h *Handler) ListTicketComments(w http.ResponseWriter, r *http.Request) {
	ticketID := strings.TrimSpace(r.PathValue("id"))
	if ticketID == "" {
		http.Error(w, "ticket id is required", http.StatusBadRequest)
		return
	}

	limit := parsePaginationParam(r.URL.Query().Get("limit"), 50, 200)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	items, total, err := h.Store.ListTicketComments(r.Context(), ticketID, limit, offset)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []*domain.TicketComment{}
	}
	writePaginatedList(w, limit, offset, len(items), int64(total), items)
}
