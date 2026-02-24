package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oriys/nova/internal/store"
)

func TestWriteTenantQuotaExceededResponse(t *testing.T) {
	t.Run("nil_decision", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeTenantQuotaExceededResponse(w, nil)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", w.Code)
		}
	})

	t.Run("with_decision", func(t *testing.T) {
		w := httptest.NewRecorder()
		decision := &store.TenantQuotaDecision{
			TenantID:    "t1",
			Dimension:   "invocations",
			Used:        100,
			Limit:       100,
			WindowS:     60,
			RetryAfterS: 30,
		}
		writeTenantQuotaExceededResponse(w, decision)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", w.Code)
		}
		if w.Header().Get("Retry-After") != "30" {
			t.Fatalf("expected Retry-After: 30, got: %s", w.Header().Get("Retry-After"))
		}
	})

	t.Run("no_retry_after", func(t *testing.T) {
		w := httptest.NewRecorder()
		decision := &store.TenantQuotaDecision{
			TenantID:    "t1",
			Dimension:   "invocations",
			RetryAfterS: 0,
		}
		writeTenantQuotaExceededResponse(w, decision)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", w.Code)
		}
		if w.Header().Get("Retry-After") != "" {
			t.Fatalf("expected no Retry-After, got: %s", w.Header().Get("Retry-After"))
		}
	})
}
