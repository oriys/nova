package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
	"github.com/google/uuid"
)

type Executor struct {
	store *store.RedisStore
	pool  *pool.Pool
}

func New(store *store.RedisStore, pool *pool.Pool) *Executor {
	return &Executor{
		store: store,
		pool:  pool,
	}
}

func (e *Executor) Invoke(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error) {
	fn, err := e.store.GetFunctionByName(ctx, funcName)
	if err != nil {
		return nil, fmt.Errorf("get function: %w", err)
	}

	reqID := uuid.New().String()[:8]
	start := time.Now()

	pvm, err := e.pool.Acquire(ctx, fn)
	if err != nil {
		return nil, fmt.Errorf("acquire VM: %w", err)
	}
	defer e.pool.Release(pvm)

	resp, err := pvm.Client.Execute(reqID, payload, fn.TimeoutS)
	if err != nil {
		return nil, fmt.Errorf("execute: %w", err)
	}

	return &domain.InvokeResponse{
		RequestID:  reqID,
		Output:     resp.Output,
		Error:      resp.Error,
		DurationMs: time.Since(start).Milliseconds(),
		ColdStart:  pvm.ColdStart,
	}, nil
}

func (e *Executor) Shutdown() {
	e.pool.Shutdown()
}
