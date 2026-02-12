package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// RemoteInvoker implements Invoker by delegating to a Comet gRPC endpoint.
type RemoteInvoker struct {
	conn   *grpc.ClientConn
	client novapb.NovaServiceClient
}

// NewRemoteInvoker connects to the given Comet gRPC address and returns
// an Invoker that forwards every call over the wire.
func NewRemoteInvoker(cometAddr string) (*RemoteInvoker, error) {
	conn, err := grpc.NewClient(cometAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to comet gRPC %s: %w", cometAddr, err)
	}
	return &RemoteInvoker{
		conn:   conn,
		client: novapb.NewNovaServiceClient(conn),
	}, nil
}

// Invoke sends the invocation request to Comet and maps the response back.
func (r *RemoteInvoker) Invoke(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error) {
	resp, err := r.client.Invoke(ctx, &novapb.InvokeRequest{
		Function: funcName,
		Payload:  payload,
	})
	if err != nil {
		return nil, fmt.Errorf("remote invoke %s: %w", funcName, err)
	}
	return &domain.InvokeResponse{
		RequestID:  resp.RequestId,
		Output:     resp.Output,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
		ColdStart:  resp.ColdStart,
	}, nil
}

// Close shuts down the underlying gRPC connection.
func (r *RemoteInvoker) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
