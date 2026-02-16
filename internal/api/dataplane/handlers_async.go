package dataplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/oriys/nova/internal/store"
)

// GetAsyncInvocation handles GET /async-invocations/{id}
func (h *Handler) GetAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := h.Store.GetAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// AsyncInvocationsSummary handles GET /async-invocations/summary
func (h *Handler) AsyncInvocationsSummary(w http.ResponseWriter, r *http.Request) {
	pagedStore, ok := h.Store.MetadataStore.(asyncInvocationPaginationStore)
	if !ok {
		http.Error(w, "async invocation summary not supported", http.StatusNotImplemented)
		return
	}

	summary, err := pagedStore.GetAsyncInvocationSummary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if summary == nil {
		summary = &store.AsyncInvocationSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// ListAsyncInvocations handles GET /async-invocations
func (h *Handler) ListAsyncInvocations(w http.ResponseWriter, r *http.Request) {
	limit := parseLimitQuery(r.URL.Query().Get("limit"), store.DefaultAsyncListLimit, store.MaxAsyncListLimit)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	statuses, err := parseAsyncStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	invs, err := h.Store.ListAsyncInvocations(r.Context(), limit, offset, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invs == nil {
		invs = []*store.AsyncInvocation{}
	}
	total := estimatePaginatedTotal(limit, offset, len(invs))
	if pagedStore, ok := h.Store.MetadataStore.(asyncInvocationPaginationStore); ok {
		if counted, countErr := pagedStore.CountAsyncInvocations(r.Context(), statuses); countErr == nil {
			total = counted
		}
	}
	writePaginatedList(w, limit, offset, len(invs), total, invs)
}

// ListFunctionAsyncInvocations handles GET /functions/{name}/async-invocations
func (h *Handler) ListFunctionAsyncInvocations(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	limit := parseLimitQuery(r.URL.Query().Get("limit"), store.DefaultAsyncListLimit, store.MaxAsyncListLimit)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	statuses, err := parseAsyncStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	invs, err := h.Store.ListFunctionAsyncInvocations(r.Context(), fn.ID, limit, offset, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invs == nil {
		invs = []*store.AsyncInvocation{}
	}
	total := estimatePaginatedTotal(limit, offset, len(invs))
	if pagedStore, ok := h.Store.MetadataStore.(asyncInvocationPaginationStore); ok {
		if counted, countErr := pagedStore.CountFunctionAsyncInvocations(r.Context(), fn.ID, statuses); countErr == nil {
			total = counted
		}
	}
	writePaginatedList(w, limit, offset, len(invs), total, invs)
}

// RetryAsyncInvocation handles POST /async-invocations/{id}/retry
func (h *Handler) RetryAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req := retryAsyncInvokeRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	inv, err := h.Store.RequeueAsyncInvocation(r.Context(), id, req.MaxAttempts)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotDLQ) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// PauseAsyncInvocation handles POST /async-invocations/{id}/pause
func (h *Handler) PauseAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := h.Store.PauseAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotQueued) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// ResumeAsyncInvocation handles POST /async-invocations/{id}/resume
func (h *Handler) ResumeAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := h.Store.ResumeAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotPaused) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// DeleteAsyncInvocation handles DELETE /async-invocations/{id}
func (h *Handler) DeleteAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.Store.DeleteAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotDeletable) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PauseAsyncInvocationsByFunction handles POST /async-invocations/functions/{id}/pause
func (h *Handler) PauseAsyncInvocationsByFunction(w http.ResponseWriter, r *http.Request) {
	functionID := r.PathValue("id")
	count, err := h.Store.PauseAsyncInvocationsByFunction(r.Context(), functionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"function_id": functionID,
		"paused":      count,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ResumeAsyncInvocationsByFunction handles POST /async-invocations/functions/{id}/resume
func (h *Handler) ResumeAsyncInvocationsByFunction(w http.ResponseWriter, r *http.Request) {
	functionID := r.PathValue("id")
	count, err := h.Store.ResumeAsyncInvocationsByFunction(r.Context(), functionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"function_id": functionID,
		"resumed":     count,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// PauseAsyncInvocationsByWorkflow handles POST /async-invocations/workflows/{id}/pause
func (h *Handler) PauseAsyncInvocationsByWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	count, err := h.Store.PauseAsyncInvocationsByWorkflow(r.Context(), workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"workflow_id": workflowID,
		"paused":      count,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ResumeAsyncInvocationsByWorkflow handles POST /async-invocations/workflows/{id}/resume
func (h *Handler) ResumeAsyncInvocationsByWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	count, err := h.Store.ResumeAsyncInvocationsByWorkflow(r.Context(), workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"workflow_id": workflowID,
		"resumed":     count,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetGlobalAsyncPause handles GET /async-invocations/global-pause
func (h *Handler) GetGlobalAsyncPause(w http.ResponseWriter, r *http.Request) {
	paused, err := h.Store.GetGlobalAsyncPause(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]bool{
		"paused": paused,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// SetGlobalAsyncPause handles POST /async-invocations/global-pause
func (h *Handler) SetGlobalAsyncPause(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.Store.SetGlobalAsyncPause(r.Context(), req.Paused); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]bool{
		"paused": req.Paused,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListDLQInvocations handles GET /async-invocations/dlq
// Returns all async invocations that have been moved to the dead letter queue.
func (h *Handler) ListDLQInvocations(w http.ResponseWriter, r *http.Request) {
	limit := parseLimitQuery(r.URL.Query().Get("limit"), 50, 500)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)

	statuses := []store.AsyncInvocationStatus{store.AsyncInvocationStatusDLQ}

	jobs, err := h.Store.ListAsyncInvocations(r.Context(), limit, offset, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	total, err := h.Store.CountAsyncInvocations(r.Context(), statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if jobs == nil {
		jobs = []*store.AsyncInvocation{}
	}

	resp := map[string]interface{}{
		"items":  jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// RetryAllDLQ handles POST /async-invocations/dlq/retry-all
// Requeues all DLQ invocations in a single batch operation.
func (h *Handler) RetryAllDLQ(w http.ResponseWriter, r *http.Request) {
	req := retryAsyncInvokeRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	// List all DLQ items
	dlqItems, err := h.Store.ListAsyncInvocations(r.Context(), 1000, 0, []store.AsyncInvocationStatus{store.AsyncInvocationStatusDLQ})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	retried := 0
	failed := 0
	for _, item := range dlqItems {
		if _, err := h.Store.RequeueAsyncInvocation(r.Context(), item.ID, req.MaxAttempts); err != nil {
			failed++
		} else {
			retried++
		}
	}

	resp := map[string]interface{}{
		"retried": retried,
		"failed":  failed,
		"total":   len(dlqItems),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListWorkflowAsyncInvocations handles GET /workflows/{name}/async-invocations
func (h *Handler) ListWorkflowAsyncInvocations(w http.ResponseWriter, r *http.Request) {
	workflowName := r.PathValue("name")

	// Get workflow to find workflowID
	workflow, err := h.Store.GetWorkflowByName(r.Context(), workflowName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	limit := parseLimitQuery(r.URL.Query().Get("limit"), 50, 500)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)

	statusParam := r.URL.Query().Get("status")
	var statuses []store.AsyncInvocationStatus
	if statusParam != "" {
		statuses = append(statuses, store.AsyncInvocationStatus(statusParam))
	}

	jobs, err := h.Store.ListWorkflowAsyncInvocations(r.Context(), workflow.ID, limit, offset, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	total, err := h.Store.CountWorkflowAsyncInvocations(r.Context(), workflow.ID, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"items":  jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func parseAsyncStatuses(raw string) ([]store.AsyncInvocationStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	statuses := make([]store.AsyncInvocationStatus, 0, len(parts))
	for _, part := range parts {
		status := store.AsyncInvocationStatus(strings.TrimSpace(part))
		if status == "" {
			continue
		}
		switch status {
		case store.AsyncInvocationStatusQueued,
			store.AsyncInvocationStatusRunning,
			store.AsyncInvocationStatusSucceeded,
			store.AsyncInvocationStatusDLQ,
			store.AsyncInvocationStatusPaused:
			statuses = append(statuses, status)
		default:
			return nil, fmt.Errorf("invalid status: %s", status)
		}
	}
	return statuses, nil
}
