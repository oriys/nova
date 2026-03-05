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
	"github.com/oriys/nova/internal/invoke"
	"github.com/oriys/nova/internal/logging"
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
//
// This is the unified invocation endpoint supporting AWS Lambda-style semantics:
//
//   - X-Nova-Invocation-Type: RequestResponse (default) → synchronous, 6 MB limit
//   - X-Nova-Invocation-Type: Event → asynchronous (enqueue), 1 MB limit
//   - X-Nova-Qualifier: alias or version number for targeted invocation
//
// Guardrails enforced:
//   - Payload size limits per invocation type
//   - Recursion protection via call-depth and cycle detection
//   - Invoke policy (caller permission check)
//   - Halted function check (emergency kill switch)
func (h *Handler) InvokeFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Parse invocation type from header (default: RequestResponse / sync)
	invType, err := invoke.ParseInvocationType(r.Header.Get(invoke.HeaderInvocationType))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve qualifier: alias name or version number
	qualifier := r.Header.Get(invoke.HeaderQualifier)
	if qualifier == "" {
		qualifier = r.URL.Query().Get("qualifier")
	}

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		safeError(w, "not found", http.StatusNotFound, err)
		return
	}

	// Qualifier resolution: resolve alias/version to a specific function config
	if qualifier != "" {
		fn, err = h.resolveQualifier(r.Context(), fn, qualifier)
		if err != nil {
			safeError(w, "qualifier resolution failed", http.StatusBadRequest, err)
			return
		}
	}

	// Halted function check (emergency kill switch, like AWS reserved concurrency = 0)
	if fn.Halted {
		http.Error(w, invoke.ErrFunctionHalted.Error(), http.StatusForbidden)
		return
	}

	// Extract call chain from headers for recursion protection
	callChain := invoke.ParseCallChain(
		r.Header.Get(invoke.HeaderCallDepth),
		r.Header.Get(invoke.HeaderCallChain),
		r.Header.Get(invoke.HeaderCallerFunction),
	)

	// Recursion protection: depth limit and cycle detection
	if err := invoke.ValidateCallChain(callChain, fn.Name); err != nil {
		status := http.StatusLoopDetected // 508
		if errors.Is(err, invoke.ErrCallDepthExceeded) {
			status = http.StatusLoopDetected
		}
		logging.Op().Warn("recursion protection triggered",
			"function", fn.Name,
			"caller", callChain.CallerFunction,
			"depth", callChain.Depth,
			"chain", callChain.ChainString(),
			"error", err,
		)
		http.Error(w, err.Error(), status)
		return
	}

	// Invoke policy: check if caller is allowed to invoke this function
	if fn.InvokePolicy != nil {
		if err := invoke.CheckInvokePolicy(fn.InvokePolicy, callChain.CallerFunction); err != nil {
			logging.Op().Warn("invoke policy denied",
				"function", fn.Name,
				"caller", callChain.CallerFunction,
				"error", err,
			)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		safeError(w, "forbidden", http.StatusForbidden, err)
		return
	}

	// Idempotency check (only for sync invocations)
	var idempotencyKey string
	if invType == invoke.InvocationTypeRequestResponse && h.Idempotency != nil {
		idem := h.Idempotency.CheckRequest(r, "invoke")
		if idem.Deduplicated {
			WriteDeduplicatedResponse(w, idem.CachedResult)
			return
		}
		idempotencyKey = idem.Key
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

	// Enforce payload size limit based on invocation type
	maxPayload := invoke.MaxPayloadBytes(invType)
	if len(payload) > maxPayload {
		http.Error(w, fmt.Sprintf("payload size %d bytes exceeds %s limit of %d bytes",
			len(payload), invType, maxPayload), http.StatusRequestEntityTooLarge)
		return
	}

	// Dispatch based on invocation type
	if invType == invoke.InvocationTypeEvent {
		h.invokeAsync(w, r, fn, payload, callChain)
		return
	}

	// Synchronous (RequestResponse) invocation
	h.invokeSync(w, r, fn, payload, idempotencyKey, callChain)
}

// invokeSync handles synchronous (RequestResponse) function invocation.
func (h *Handler) invokeSync(w http.ResponseWriter, r *http.Request, fn *domain.Function, payload json.RawMessage, idempotencyKey string, callChain invoke.CallChain) {
	scope := store.TenantScopeFromContext(r.Context())
	invQuotaDecision, err := h.Store.CheckAndConsumeTenantQuota(r.Context(), scope.TenantID, store.TenantDimensionInvocations, 1)
	if err != nil {
		safeError(w, "internal error", http.StatusInternalServerError, err)
		return
	}
	if invQuotaDecision != nil && !invQuotaDecision.Allowed {
		reason := "tenant_quota_" + invQuotaDecision.Dimension
		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		metrics.RecordShed(fn.Name, reason)
		writeTenantQuotaExceeded(w, invQuotaDecision)
		return
	}

	if h.ClusterRouter != nil && strings.TrimSpace(r.Header.Get("X-Nova-Cluster-Forwarded")) == "" {
		routedResp, routed, routeErr := h.ClusterRouter.TryRouteInvoke(r.Context(), fn.ID, fn.Name, payload)
		if routeErr != nil {
			logging.Op().Warn("cluster route invoke failed; fallback local",
				"function", fn.Name,
				"error", routeErr)
		} else if routed && routedResp != nil {
			metrics.RecordAdmissionResult(fn.Name, "accepted", "cluster_forwarded")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(routedResp)
			return
		}
	}

	metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
	metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	defer func() {
		metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
		metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	}()

	// Propagate call chain context into the request context so the executor
	// can inject it into the VM environment for downstream SDK use.
	ctx := r.Context()
	nextChain := callChain.Push(fn.Name)
	ctx = invoke.WithCallChain(ctx, nextChain)

	resp, err := h.Exec.Invoke(ctx, fn.Name, payload)
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

		safeError(w, "request failed", status, err)
		return
	}
	metrics.RecordAdmissionResult(fn.Name, "accepted", "ok")

	// Cache the result for idempotency replay.
	if h.Idempotency != nil && idempotencyKey != "" {
		if resultBytes, jErr := json.Marshal(resp); jErr == nil {
			h.Idempotency.CompleteRequest(idempotencyKey, resultBytes)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// invokeAsync handles asynchronous (Event) function invocation: enqueue and return immediately.
func (h *Handler) invokeAsync(w http.ResponseWriter, r *http.Request, fn *domain.Function, payload json.RawMessage, callChain invoke.CallChain) {
	scope := store.TenantScopeFromContext(r.Context())
	queueDepth, err := h.Store.GetTenantAsyncQueueDepth(r.Context(), scope.TenantID)
	if err != nil {
		safeError(w, "internal error", http.StatusInternalServerError, err)
		return
	}
	queueQuotaDecision, err := h.Store.CheckTenantAbsoluteQuota(r.Context(), scope.TenantID, store.TenantDimensionAsyncQueueDepth, queueDepth+1)
	if err != nil {
		safeError(w, "internal error", http.StatusInternalServerError, err)
		return
	}
	if queueQuotaDecision != nil && !queueQuotaDecision.Allowed {
		writeTenantQuotaExceeded(w, queueQuotaDecision)
		return
	}

	inv := store.NewAsyncInvocation(fn.ID, fn.Name, payload)

	// Store call chain metadata so the async worker can propagate it
	if callChain.Depth > 0 {
		inv.CallDepth = callChain.Depth
		inv.CallChain = callChain.ChainString()
		inv.CallerFunction = callChain.CallerFunction
	}

	if err := h.Store.EnqueueAsyncInvocation(r.Context(), inv); err != nil {
		safeError(w, "internal error", http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/async-invocations/"+inv.ID)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"invocation_id": inv.ID,
		"status":        inv.Status,
		"function":      fn.Name,
	})
}

// resolveQualifier resolves a qualifier (alias name or version number) to the
// appropriate function configuration. This enables production patterns like
// invoking "order-processor:stable" instead of the bare function name.
func (h *Handler) resolveQualifier(ctx context.Context, fn *domain.Function, qualifier string) (*domain.Function, error) {
	// Try parsing as version number first
	if version, err := strconv.Atoi(qualifier); err == nil {
		ver, err := h.Store.GetVersion(ctx, fn.ID, version)
		if err != nil {
			return nil, fmt.Errorf("version %d not found: %w", version, err)
		}
		// Apply version-specific overrides
		fn.Version = ver.Version
		fn.CodeHash = ver.CodeHash
		fn.Handler = ver.Handler
		fn.MemoryMB = ver.MemoryMB
		fn.TimeoutS = ver.TimeoutS
		if ver.Mode != "" {
			fn.Mode = ver.Mode
		}
		if ver.Limits != nil {
			fn.Limits = ver.Limits
		}
		if ver.EnvVars != nil {
			fn.EnvVars = ver.EnvVars
		}
		return fn, nil
	}

	// Try resolving as alias
	alias, err := h.Store.GetAlias(ctx, fn.ID, qualifier)
	if err != nil {
		return nil, fmt.Errorf("qualifier %q not found: %w", qualifier, err)
	}

	// If alias points to a single version, resolve it
	if alias.Version > 0 {
		return h.resolveQualifier(ctx, fn, strconv.Itoa(alias.Version))
	}

	// If alias has traffic split, apply it to the function
	if len(alias.TrafficSplit) > 0 {
		fn.TrafficSplit = alias.TrafficSplit
	}

	return fn, nil
}

// CallChainFromContext extracts the CallChain from a context, if present.
// Re-exported from the invoke package for convenience.
func CallChainFromContext(ctx context.Context) (invoke.CallChain, bool) {
	return invoke.CallChainFromContext(ctx)
}

// InvokeFunctionStream handles POST /functions/{name}/invoke-stream with streaming response
func (h *Handler) InvokeFunctionStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		safeError(w, "not found", http.StatusNotFound, err)
		return
	}
	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		safeError(w, "forbidden", http.StatusForbidden, err)
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
		safeError(w, "internal error", http.StatusInternalServerError, err)
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
			fmt.Fprintf(w, "event: error\ndata: invocation error\n\n")
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
		fmt.Fprintf(w, "event: error\ndata: invocation error\n\n")
		flusher.Flush()
		return
	}
	metrics.RecordAdmissionResult(fn.Name, "accepted", "ok")
}

// EnqueueAsyncFunction handles POST /functions/{name}/invoke-async
//
// This is the dedicated async invocation endpoint (kept for backward compatibility).
// The unified endpoint POST /functions/{name}/invoke with X-Nova-Invocation-Type: Event
// is the preferred approach for new integrations.
func (h *Handler) EnqueueAsyncFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		safeError(w, "not found", http.StatusNotFound, err)
		return
	}

	// Halted function check
	if fn.Halted {
		http.Error(w, invoke.ErrFunctionHalted.Error(), http.StatusForbidden)
		return
	}

	// Call chain extraction and recursion protection
	callChain := invoke.ParseCallChain(
		r.Header.Get(invoke.HeaderCallDepth),
		r.Header.Get(invoke.HeaderCallChain),
		r.Header.Get(invoke.HeaderCallerFunction),
	)
	if err := invoke.ValidateCallChain(callChain, fn.Name); err != nil {
		http.Error(w, err.Error(), http.StatusLoopDetected)
		return
	}

	// Invoke policy check
	if fn.InvokePolicy != nil {
		if err := invoke.CheckInvokePolicy(fn.InvokePolicy, callChain.CallerFunction); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		safeError(w, "forbidden", http.StatusForbidden, err)
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

	// Enforce async payload size limit (1 MB)
	if len(payload) > invoke.MaxAsyncPayloadBytes {
		http.Error(w, fmt.Sprintf("payload size %d bytes exceeds async limit of %d bytes",
			len(payload), invoke.MaxAsyncPayloadBytes), http.StatusRequestEntityTooLarge)
		return
	}

	scope := store.TenantScopeFromContext(r.Context())
	queueDepth, err := h.Store.GetTenantAsyncQueueDepth(r.Context(), scope.TenantID)
	if err != nil {
		safeError(w, "internal error", http.StatusInternalServerError, err)
		return
	}
	queueQuotaDecision, err := h.Store.CheckTenantAbsoluteQuota(r.Context(), scope.TenantID, store.TenantDimensionAsyncQueueDepth, queueDepth+1)
	if err != nil {
		safeError(w, "internal error", http.StatusInternalServerError, err)
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
				safeError(w, "bad request", http.StatusBadRequest, err)
				return
			}
			safeError(w, "internal error", http.StatusInternalServerError, err)
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
		safeError(w, "internal error", http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/async-invocations/"+inv.ID)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(inv)
}
