package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// BalancedRemoteInvoker implements Invoker with intelligent client-side
// load balancing across multiple Comet gRPC endpoints. It uses a
// least-connections strategy: each invocation is routed to the endpoint
// with the fewest in-flight requests, maximizing throughput under load.
//
// When only one endpoint is configured, it behaves identically to
// RemoteInvoker with zero overhead.
type BalancedRemoteInvoker struct {
	endpoints []*cometEndpoint
}

type cometEndpoint struct {
	addr     string
	conn     *grpc.ClientConn
	client   novapb.NovaServiceClient
	inflight atomic.Int64
	// Resource-aware scheduling weights (0.0–1.0 utilization).
	// Updated asynchronously via ReportLoad.
	cpuUsage atomic.Value // float64
	memUsage atomic.Value // float64
}

// NewBalancedRemoteInvoker connects to the given Comet gRPC addresses and
// returns an Invoker that distributes calls using least-connections balancing.
func NewBalancedRemoteInvoker(addrs []string) (*BalancedRemoteInvoker, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("at least one comet address is required")
	}

	endpoints := make([]*cometEndpoint, 0, len(addrs))
	for _, addr := range addrs {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			// Clean up already-established connections
			for _, ep := range endpoints {
				ep.conn.Close()
			}
			return nil, fmt.Errorf("connect to comet gRPC %s: %w", addr, err)
		}
		endpoints = append(endpoints, &cometEndpoint{
			addr:   addr,
			conn:   conn,
			client: novapb.NewNovaServiceClient(conn),
		})
	}

	logging.Op().Info("balanced remote invoker created",
		"endpoints", len(endpoints),
		"strategy", "least-connections")

	return &BalancedRemoteInvoker{endpoints: endpoints}, nil
}

// Invoke sends the invocation request to the least-loaded Comet endpoint.
func (b *BalancedRemoteInvoker) Invoke(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error) {
	ep := b.leastLoaded()
	ep.inflight.Add(1)
	defer ep.inflight.Add(-1)

	resp, err := ep.client.Invoke(ctx, &novapb.InvokeRequest{
		Function: funcName,
		Payload:  payload,
	})
	if err != nil {
		return nil, fmt.Errorf("remote invoke %s via %s: %w", funcName, ep.addr, err)
	}
	return &domain.InvokeResponse{
		RequestID:  resp.RequestId,
		Output:     resp.Output,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
		ColdStart:  resp.ColdStart,
	}, nil
}

// leastLoaded returns the endpoint with the lowest composite load score.
// The score combines in-flight requests with CPU and memory utilization
// when resource data is available, falling back to pure least-connections
// when no resource metrics have been reported.
func (b *BalancedRemoteInvoker) leastLoaded() *cometEndpoint {
	best := b.endpoints[0]
	bestScore := endpointScore(best)
	for _, ep := range b.endpoints[1:] {
		score := endpointScore(ep)
		if score < bestScore {
			best = ep
			bestScore = score
		}
	}
	return best
}

// endpointScore computes a composite load score for scheduling.
// Lower scores indicate a better target for the next invocation.
// The formula weights: 50% in-flight connections, 30% CPU, 20% memory.
func endpointScore(ep *cometEndpoint) float64 {
	inflight := float64(ep.inflight.Load())

	cpu := 0.0
	if v, ok := ep.cpuUsage.Load().(float64); ok {
		cpu = v
	}
	mem := 0.0
	if v, ok := ep.memUsage.Load().(float64); ok {
		mem = v
	}

	// Normalize inflight to 0-1 range (assume 100 as a soft cap for scoring)
	inflightNorm := inflight / 100.0
	if inflightNorm > 1.0 {
		inflightNorm = 1.0
	}

	return 0.5*inflightNorm + 0.3*cpu + 0.2*mem
}

// ReportLoad updates the resource utilization metrics for a Comet endpoint.
// cpuUsage and memUsage should be fractions in [0.0, 1.0].
func (b *BalancedRemoteInvoker) ReportLoad(addr string, cpuUsage, memUsage float64) {
	for _, ep := range b.endpoints {
		if ep.addr == addr {
			ep.cpuUsage.Store(cpuUsage)
			ep.memUsage.Store(memUsage)
			return
		}
	}
}

// Close shuts down all underlying gRPC connections.
func (b *BalancedRemoteInvoker) Close() error {
	var firstErr error
	for _, ep := range b.endpoints {
		if err := ep.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// PersistentVsockStream provides a reusable vsock connection for
// "persistent" mode functions. Instead of dialing/closing the vsock
// connection for each invocation, it keeps the connection alive and
// multiplexes requests over it. This reduces per-invocation overhead
// from handshake + init to just sending the exec message.
//
// The stream automatically reconnects on connection failures and
// serializes concurrent access via a mutex.
type PersistentVsockStream struct {
	mu      sync.Mutex
	dialer  func() error
	sender  func(msg interface{}) error
	recver  func() (interface{}, error)
	closer  func() error
	alive   bool
}

// NewPersistentVsockStream creates a persistent stream wrapper.
// The dialer, sender, receiver and closer functions are injected to
// decouple from the concrete VsockClient implementation.
func NewPersistentVsockStream(
	dialer func() error,
	sender func(msg interface{}) error,
	recver func() (interface{}, error),
	closer func() error,
) *PersistentVsockStream {
	return &PersistentVsockStream{
		dialer: dialer,
		sender: sender,
		recver: recver,
		closer: closer,
	}
}

// Execute sends a request and receives a response, reusing the existing
// connection when possible. On connection errors, it reconnects
// automatically and retries once.
func (p *PersistentVsockStream) Execute(msg interface{}) (interface{}, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Ensure connection is alive
	if !p.alive {
		if err := p.dialer(); err != nil {
			return nil, fmt.Errorf("persistent vsock dial: %w", err)
		}
		p.alive = true
	}

	// Try to send on existing connection
	if err := p.sender(msg); err != nil {
		// Connection broken, try reconnect once
		p.alive = false
		if err := p.dialer(); err != nil {
			return nil, fmt.Errorf("persistent vsock redial: %w", err)
		}
		p.alive = true
		if err := p.sender(msg); err != nil {
			p.alive = false
			return nil, fmt.Errorf("persistent vsock send after redial: %w", err)
		}
		// Retry send succeeded; fall through to receive below
	}

	// Receive the response (reached after either first send or retry send)
	resp, err := p.recver()
	if err != nil {
		p.alive = false
		return nil, fmt.Errorf("persistent vsock receive: %w", err)
	}

	return resp, nil
}

// Close shuts down the persistent connection.
func (p *PersistentVsockStream) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.alive = false
	if p.closer != nil {
		return p.closer()
	}
	return nil
}
