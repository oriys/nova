package executor

import (
	"context"
	"encoding/json"

	"github.com/oriys/nova/internal/domain"
)

// Invoker abstracts function invocation so that callers (scheduler, async
// queue, event bus, workflow engine) can invoke functions either locally
// (via the full Executor) or remotely (via a gRPC client to Comet).
//
// # Contract
//
// Implementations must be safe for concurrent use from multiple goroutines.
// Both the local Executor and the remote gRPC client satisfy this contract.
//
// # Idempotency
//
// Not guaranteed by the interface. Callers requiring at-most-once semantics
// must implement deduplication at the call site.
type Invoker interface {
	Invoke(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error)
}
