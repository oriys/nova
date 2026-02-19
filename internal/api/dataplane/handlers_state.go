package dataplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/oriys/nova/internal/store"
)

func (h *Handler) GetFunctionState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key != "" {
		entry, err := h.Store.GetFunctionState(r.Context(), fn.ID, key)
		if errors.Is(err, store.ErrFunctionStateNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entry)
		return
	}

	prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 100, 500)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	entries, err := h.Store.ListFunctionStates(r.Context(), fn.ID, &store.FunctionStateListOptions{
		Prefix: prefix,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []*store.FunctionStateEntry{}
	}
	total, err := h.Store.CountFunctionStates(r.Context(), fn.ID, prefix)
	if err != nil {
		total = estimatePaginatedTotal(limit, offset, len(entries))
	}
	writePaginatedList(w, limit, offset, len(entries), total, entries)
}

func (h *Handler) PutFunctionState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		http.Error(w, "key query parameter is required", http.StatusBadRequest)
		return
	}

	var req struct {
		Value           json.RawMessage `json:"value"`
		TTLS            int             `json:"ttl_s,omitempty"`
		ExpectedVersion int64           `json:"expected_version,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	if len(req.Value) == 0 || !json.Valid(req.Value) {
		http.Error(w, "value must be valid JSON", http.StatusBadRequest)
		return
	}
	if req.TTLS < 0 {
		http.Error(w, "ttl_s must be >= 0", http.StatusBadRequest)
		return
	}
	if req.ExpectedVersion < 0 {
		http.Error(w, "expected_version must be >= 0", http.StatusBadRequest)
		return
	}

	opts := &store.FunctionStatePutOptions{
		ExpectedVersion: req.ExpectedVersion,
	}
	if req.TTLS > 0 {
		opts.TTL = time.Duration(req.TTLS) * time.Second
	}

	entry, err := h.Store.PutFunctionState(r.Context(), fn.ID, key, req.Value, opts)
	switch {
	case errors.Is(err, store.ErrFunctionStateVersionConflict):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, store.ErrFunctionStateNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

func (h *Handler) DeleteFunctionState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		http.Error(w, "key query parameter is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteFunctionState(r.Context(), fn.ID, key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
