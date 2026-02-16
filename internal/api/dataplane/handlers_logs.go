package dataplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oriys/nova/internal/store"
)

// Stats handles GET /stats
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.Pool.Stats())
}

// Logs handles GET /functions/{name}/logs
func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Get request_id from query params if specified
	requestID := r.URL.Query().Get("request_id")
	if requestID != "" {
		entry, err := h.Store.GetInvocationLog(r.Context(), requestID)
		if err != nil {
			http.Error(w, "logs not found for request_id", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
		return
	}

	// Otherwise return recent logs for function
	tailStr := r.URL.Query().Get("tail")
	tail := 10
	if tailStr != "" {
		if n, err := fmt.Sscanf(tailStr, "%d", &tail); err != nil || n != 1 {
			tail = 10
		}
	}

	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	entries, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, tail, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure we return empty array instead of null
	if entries == nil {
		entries = []*store.InvocationLog{}
	}

	total := int64(len(entries))
	if pagedStore, ok := h.Store.MetadataStore.(invocationPaginationStore); ok {
		if counted, countErr := pagedStore.CountInvocationLogs(r.Context(), fn.ID); countErr == nil {
			total = counted
		}
	}
	writePaginatedList(w, tail, offset, len(entries), total, entries)
}

// StreamLogs handles GET /functions/{name}/logs/stream using Server-Sent Events.
// It polls the invocation logs store at a short interval and pushes new entries
// to the client as SSE data events in real time, enabling live log tailing from
// the dashboard without WebSocket infrastructure.
func (h *Handler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Track the last seen invocation timestamp to only send new entries.
	lastSeen := time.Now()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			entries, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, 20, 0)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.CreatedAt.After(lastSeen) {
					data, _ := json.Marshal(entry)
					fmt.Fprintf(w, "data: %s\n\n", data)
					lastSeen = entry.CreatedAt
				}
			}
			flusher.Flush()
		}
	}
}

// ListAllInvocations handles GET /invocations
func (h *Handler) ListAllInvocations(w http.ResponseWriter, r *http.Request) {
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 100, 500)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if search == "" {
		search = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	functionName := strings.TrimSpace(r.URL.Query().Get("function"))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	var successFilter *bool
	switch status {
	case "", "all":
	case "success", "succeeded":
		v := true
		successFilter = &v
	case "failed", "error", "errors":
		v := false
		successFilter = &v
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	var (
		entries []*store.InvocationLog
		err     error
		total   int64
		summary *store.InvocationLogSummary
	)
	if pagedStore, ok := h.Store.MetadataStore.(invocationPaginationStore); ok {
		if search != "" || functionName != "" || successFilter != nil {
			entries, err = pagedStore.ListAllInvocationLogsFiltered(r.Context(), limit, offset, search, functionName, successFilter)
			if err == nil {
				total, err = pagedStore.CountAllInvocationLogsFiltered(r.Context(), search, functionName, successFilter)
			}
			if err == nil {
				summary, _ = pagedStore.GetAllInvocationLogsSummaryFiltered(r.Context(), search, functionName, successFilter)
			}
		} else {
			entries, err = h.Store.ListAllInvocationLogs(r.Context(), limit, offset)
			if err == nil {
				total, err = pagedStore.CountAllInvocationLogs(r.Context())
			}
			if err == nil {
				summary, _ = pagedStore.GetAllInvocationLogsSummary(r.Context())
			}
		}
	} else {
		entries, err = h.Store.ListAllInvocationLogs(r.Context(), limit, offset)
		if err == nil {
			total = int64(len(entries))
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure we return empty array instead of null
	if entries == nil {
		entries = []*store.InvocationLog{}
	}
	if summary == nil {
		summary = summarizeInvocations(entries)
	}
	// Keep pagination total and summary counters consistent when writes are concurrent.
	// Both values are shown together on the history page.
	total = summary.TotalInvocations
	writePaginatedListWithSummary(w, limit, offset, len(entries), total, entries, summary)
}

func summarizeInvocations(entries []*store.InvocationLog) *store.InvocationLogSummary {
	summary := &store.InvocationLogSummary{}
	if len(entries) == 0 {
		return summary
	}

	summary.TotalInvocations = int64(len(entries))
	var totalDuration int64
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Success {
			summary.Successes++
		} else {
			summary.Failures++
		}
		if entry.ColdStart {
			summary.ColdStarts++
		}
		totalDuration += entry.DurationMs
	}
	if summary.TotalInvocations > 0 {
		summary.AvgDurationMs = totalDuration / summary.TotalInvocations
	}
	return summary
}
