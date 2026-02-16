package dataplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

func writeTenantQuotaExceeded(w http.ResponseWriter, decision *store.TenantQuotaDecision) {
	if decision == nil {
		http.Error(w, "tenant quota exceeded", http.StatusTooManyRequests)
		return
	}
	if decision.RetryAfterS > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(decision.RetryAfterS))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":         "tenant quota exceeded",
		"tenant_id":     decision.TenantID,
		"dimension":     decision.Dimension,
		"used":          decision.Used,
		"limit":         decision.Limit,
		"window_s":      decision.WindowS,
		"retry_after_s": decision.RetryAfterS,
	})
}

func capacityShedStatus(fn *domain.Function) int {
	if fn != nil && fn.CapacityPolicy != nil && fn.CapacityPolicy.ShedStatusCode != 0 {
		return fn.CapacityPolicy.ShedStatusCode
	}
	return http.StatusServiceUnavailable
}

func capacityRetryAfter(fn *domain.Function) int {
	if fn != nil && fn.CapacityPolicy != nil && fn.CapacityPolicy.RetryAfterS > 0 {
		return fn.CapacityPolicy.RetryAfterS
	}
	return 1
}

// InvokeFunction handles POST /functions/{name}/invoke
func (h *Handler) InvokeFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	var payload json.RawMessage
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	} else {
		payload = json.RawMessage("{}")
	}

	scope := store.TenantScopeFromContext(r.Context())
	invQuotaDecision, err := h.Store.CheckAndConsumeTenantQuota(r.Context(), scope.TenantID, store.TenantDimensionInvocations, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invQuotaDecision != nil && !invQuotaDecision.Allowed {
		reason := "tenant_quota_" + invQuotaDecision.Dimension
		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		metrics.RecordShed(fn.Name, reason)
		writeTenantQuotaExceeded(w, invQuotaDecision)
		return
	}

	metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
	metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	defer func() {
		metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
		metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	}()

	resp, err := h.Exec.Invoke(r.Context(), name, payload)
	if err != nil {
		status := http.StatusInternalServerError
		reason := "internal_error"

		switch {
		case errors.Is(err, pool.ErrQueueFull):
			status = capacityShedStatus(fn)
			reason = "queue_full"
		case errors.Is(err, pool.ErrInflightLimit):
			status = capacityShedStatus(fn)
			reason = "inflight_limit"
		case errors.Is(err, pool.ErrQueueWaitTimeout):
			status = capacityShedStatus(fn)
			reason = "queue_wait_timeout"
		case errors.Is(err, pool.ErrConcurrencyLimit):
			status = http.StatusServiceUnavailable
			reason = "concurrency_limit"
		case errors.Is(err, executor.ErrCircuitOpen):
			status = http.StatusServiceUnavailable
			reason = "circuit_breaker_open"
		case errors.Is(err, context.DeadlineExceeded):
			status = http.StatusGatewayTimeout
			reason = "timeout"
		}

		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			metrics.RecordShed(fn.Name, reason)
			if retryAfter := capacityRetryAfter(fn); retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			}
		}

		http.Error(w, err.Error(), status)
		return
	}
	metrics.RecordAdmissionResult(fn.Name, "accepted", "ok")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// InvokeFunctionStream handles POST /functions/{name}/invoke-stream with streaming response
func (h *Handler) InvokeFunctionStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	var payload json.RawMessage
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	} else {
		payload = json.RawMessage("{}")
	}

	// Check tenant quota
	scope := store.TenantScopeFromContext(r.Context())
	invQuotaDecision, err := h.Store.CheckAndConsumeTenantQuota(r.Context(), scope.TenantID, store.TenantDimensionInvocations, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invQuotaDecision != nil && !invQuotaDecision.Allowed {
		reason := "tenant_quota_" + invQuotaDecision.Dimension
		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		metrics.RecordShed(fn.Name, reason)
		writeTenantQuotaExceeded(w, invQuotaDecision)
		return
	}

	// Track queue metrics
	metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
	metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	defer func() {
		metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
		metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	}()

	// Set up streaming response headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Write headers immediately
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Execute in streaming mode
	execErr := h.Exec.InvokeStream(r.Context(), name, payload, func(chunk []byte, isLast bool, err error) error {
		if err != nil {
			// Send error as SSE event
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return err
		}

		if len(chunk) > 0 {
			// Send data chunk as SSE event
			// Base64 encode binary data for safe transport
			encoded := base64.StdEncoding.EncodeToString(chunk)
			fmt.Fprintf(w, "data: %s\n\n", encoded)
			flusher.Flush()
		}

		if isLast {
			// Send completion event
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
		}

		return nil
	})

	if execErr != nil {
		// Handle errors that occur before streaming starts
		status := http.StatusInternalServerError
		reason := "internal_error"

		switch {
		case errors.Is(execErr, pool.ErrQueueFull):
			status = capacityShedStatus(fn)
			reason = "queue_full"
		case errors.Is(execErr, pool.ErrInflightLimit):
			status = capacityShedStatus(fn)
			reason = "inflight_limit"
		case errors.Is(execErr, pool.ErrQueueWaitTimeout):
			status = capacityShedStatus(fn)
			reason = "queue_wait_timeout"
		case errors.Is(execErr, pool.ErrConcurrencyLimit):
			status = http.StatusServiceUnavailable
			reason = "concurrency_limit"
		case errors.Is(execErr, executor.ErrCircuitOpen):
			status = http.StatusServiceUnavailable
			reason = "circuit_breaker_open"
		case errors.Is(execErr, context.DeadlineExceeded):
			status = http.StatusGatewayTimeout
			reason = "timeout"
		}

		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			metrics.RecordShed(fn.Name, reason)
		}

		// Send error event
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", execErr.Error())
		flusher.Flush()
		return
	}
	metrics.RecordAdmissionResult(fn.Name, "accepted", "ok")
}

// EnqueueAsyncFunction handles POST /functions/{name}/invoke-async
func (h *Handler) EnqueueAsyncFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	req := enqueueAsyncInvokeRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	scope := store.TenantScopeFromContext(r.Context())
	queueDepth, err := h.Store.GetTenantAsyncQueueDepth(r.Context(), scope.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	queueQuotaDecision, err := h.Store.CheckTenantAbsoluteQuota(r.Context(), scope.TenantID, store.TenantDimensionAsyncQueueDepth, queueDepth+1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if queueQuotaDecision != nil && !queueQuotaDecision.Allowed {
		writeTenantQuotaExceeded(w, queueQuotaDecision)
		return
	}

	inv := store.NewAsyncInvocation(fn.ID, fn.Name, payload)
	if req.MaxAttempts > 0 {
		inv.MaxAttempts = req.MaxAttempts
	}
	if req.BackoffBaseMS > 0 {
		inv.BackoffBaseMS = req.BackoffBaseMS
	}
	if req.BackoffMaxMS > 0 {
		inv.BackoffMaxMS = req.BackoffMaxMS
	}
	if inv.BackoffMaxMS < inv.BackoffBaseMS {
		inv.BackoffMaxMS = inv.BackoffBaseMS
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" {
		ttl := time.Duration(req.IdempotencyTTLS) * time.Second
		enqueued, deduplicated, err := h.Store.EnqueueAsyncInvocationWithIdempotency(r.Context(), inv, idempotencyKey, ttl)
		if err != nil {
			if errors.Is(err, store.ErrInvalidIdempotencyKey) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if deduplicated {
			w.Header().Set("X-Idempotency-Status", "replay")
			w.WriteHeader(http.StatusOK)
		} else {
			w.Header().Set("Location", "/async-invocations/"+enqueued.ID)
			w.WriteHeader(http.StatusAccepted)
		}
		json.NewEncoder(w).Encode(enqueued)
		return
	}

	if err := h.Store.EnqueueAsyncInvocation(r.Context(), inv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/async-invocations/"+inv.ID)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(inv)
}
