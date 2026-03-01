package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
)

func TestRouteExecutionPolicy_Defaults(t *testing.T) {
	attempts, backoff := routeExecutionPolicy(nil)
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if backoff != 0 {
		t.Fatalf("backoff = %v, want 0", backoff)
	}
}

func TestRouteExecutionPolicy_NormalizesValues(t *testing.T) {
	route := &domain.GatewayRoute{
		RetryPolicy: &domain.RouteRetryPolicy{
			MaxAttempts: 0,
			BackoffMs:   -10,
		},
	}
	attempts, backoff := routeExecutionPolicy(route)
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if backoff != 0 {
		t.Fatalf("backoff = %v, want 0", backoff)
	}
}

func TestInvokeWithRetry_SucceedsAfterRetries(t *testing.T) {
	ctx := context.Background()
	calls := 0
	resp, err := invokeWithRetry(ctx, 3, 0, func(context.Context) (*domain.InvokeResponse, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("transient")
		}
		return &domain.InvokeResponse{RequestID: "ok"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.RequestID != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestInvokeWithRetry_StopsAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	calls := 0
	_, err := invokeWithRetry(ctx, 2, 0, func(context.Context) (*domain.InvokeResponse, error) {
		calls++
		return nil, errors.New("always fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestInvokeWithRetry_StopsOnContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	calls := 0
	_, err := invokeWithRetry(ctx, 3, 100*time.Millisecond, func(context.Context) (*domain.InvokeResponse, error) {
		calls++
		return nil, errors.New("fail then backoff")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
