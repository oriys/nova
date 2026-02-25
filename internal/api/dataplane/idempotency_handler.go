// Package dataplane provides HTTP handlers for the data-plane API.
// This file adds idempotency support to the invoke endpoint.
package dataplane

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/oriys/nova/internal/idempotency"
)

// IdempotencyMiddleware checks for idempotency keys on invoke requests
// and returns cached results for duplicate invocations.
type IdempotencyMiddleware struct {
	store *idempotency.Store
}

// NewIdempotencyMiddleware creates a new idempotency middleware.
func NewIdempotencyMiddleware(store *idempotency.Store) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{store: store}
}

// IdempotencyResult represents the result of an idempotency check.
type IdempotencyResult struct {
	Key          string          // The idempotency key (provided or generated)
	Deduplicated bool            // True if this is a cached result
	CachedResult json.RawMessage // The cached result, if any
}

// CheckRequest examines a request for an idempotency key.
// If the key exists and has a cached result, returns it.
// Otherwise claims the key for execution.
func (m *IdempotencyMiddleware) CheckRequest(r *http.Request, workerID string) *IdempotencyResult {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		// No idempotency key provided, proceed normally
		return &IdempotencyResult{}
	}

	result := m.store.Check(r.Context(), key, workerID)

	if result.Hit && result.Status == idempotency.StatusCompleted {
		return &IdempotencyResult{
			Key:          key,
			Deduplicated: true,
			CachedResult: result.Result,
		}
	}

	return &IdempotencyResult{Key: key}
}

// CompleteRequest stores the result for an idempotency key.
func (m *IdempotencyMiddleware) CompleteRequest(key string, result json.RawMessage) {
	if key == "" {
		return
	}
	m.store.Complete(key, result)
}

// FailRequest marks an idempotency key as failed.
func (m *IdempotencyMiddleware) FailRequest(key string, errMsg string) {
	if key == "" {
		return
	}
	m.store.Fail(key, errMsg)
}

// WriteDeduplicatedResponse writes a cached response for a deduplicated request.
func WriteDeduplicatedResponse(w http.ResponseWriter, cached json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Deduplicated", "true")
	w.Header().Set("X-Cache-Hit", time.Now().UTC().Format(time.RFC3339))
	w.WriteHeader(http.StatusOK)
	w.Write(cached)
}
