package executor

import (
	"context"
	"encoding/json"

	"github.com/oriys/nova/internal/domain"
)

// Invoker abstracts function invocation so that callers (scheduler, async
// queue, event bus, workflow engine) can invoke functions either locally
// (via the full Executor) or remotely (via a gRPC client to Comet).
type Invoker interface {
	Invoke(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error)
}
