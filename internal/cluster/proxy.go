package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Proxy forwards function invocations to remote Comet nodes in the cluster.
type Proxy struct {
	client    *http.Client
	timeout   time.Duration
	connsMu   sync.Mutex
	grpcConns map[string]*grpc.ClientConn
}

// NewProxy creates a cluster proxy with configurable timeout.
func NewProxy(timeout time.Duration) *Proxy {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Proxy{
		client:    &http.Client{Timeout: timeout},
		timeout:   timeout,
		grpcConns: make(map[string]*grpc.ClientConn),
	}
}

// ForwardInvoke forwards a function invocation to a remote node.
// nodeAddress defaults to gRPC host:port. Prefix with grpc:// for explicit gRPC,
// or http(s):// for HTTP fallback.
func (p *Proxy) ForwardInvoke(ctx context.Context, nodeAddress, functionName string, payload json.RawMessage) (json.RawMessage, error) {
	normalized, useHTTP := normalizeNodeAddress(nodeAddress)
	if normalized == "" {
		return nil, fmt.Errorf("empty node address")
	}
	if useHTTP {
		return p.forwardInvokeHTTP(ctx, normalized, functionName, payload)
	}
	return p.forwardInvokeGRPC(ctx, normalized, functionName, payload)
}

// ForwardPrewarm asks a remote node to prewarm a function to the target replica
// count ahead of predicted traffic.
func (p *Proxy) ForwardPrewarm(ctx context.Context, nodeAddress, functionName string, targetReplicas int) error {
	normalized, useHTTP := normalizeNodeAddress(nodeAddress)
	if normalized == "" {
		return fmt.Errorf("empty node address")
	}
	if useHTTP {
		return p.forwardPrewarmHTTP(ctx, normalized, functionName, targetReplicas)
	}
	return p.forwardPrewarmGRPC(ctx, normalized, functionName, targetReplicas)
}

func (p *Proxy) forwardInvokeHTTP(ctx context.Context, nodeAddress, functionName string, payload json.RawMessage) (json.RawMessage, error) {
	target := fmt.Sprintf("%s/functions/%s/invoke", strings.TrimRight(nodeAddress, "/"), url.PathEscape(functionName))

	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nova-Forwarded", "true")
	req.Header.Set("X-Nova-Cluster-Forwarded", "true")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forward request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("remote invoke failed (status %d): %s", resp.StatusCode, body)
	}

	return json.RawMessage(body), nil
}

func (p *Proxy) forwardInvokeGRPC(ctx context.Context, addr, functionName string, payload json.RawMessage) (json.RawMessage, error) {
	conn, err := p.getGRPCConn(ctx, addr)
	if err != nil {
		return nil, err
	}

	client := novapb.NewNovaServiceClient(conn)
	ctx = withClusterForwardedMetadata(ctx)
	resp, err := client.Invoke(ctx, &novapb.InvokeRequest{
		Function: functionName,
		Payload:  payload,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc invoke: %w", err)
	}

	output := resp.Output
	if len(output) == 0 {
		output = []byte("null")
	}
	if !json.Valid(output) {
		output = []byte(strconv.Quote(base64.StdEncoding.EncodeToString(output)))
	}

	body, err := json.Marshal(&domain.InvokeResponse{
		RequestID:  resp.RequestId,
		Output:     output,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
		ColdStart:  resp.ColdStart,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal invoke response: %w", err)
	}

	return json.RawMessage(body), nil
}

func (p *Proxy) forwardPrewarmHTTP(ctx context.Context, nodeAddress, functionName string, targetReplicas int) error {
	target := fmt.Sprintf("%s/functions/%s/prewarm", strings.TrimRight(nodeAddress, "/"), url.PathEscape(functionName))
	body, err := json.Marshal(map[string]int{"target_replicas": targetReplicas})
	if err != nil {
		return fmt.Errorf("marshal prewarm request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create prewarm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nova-Cluster-Forwarded", "true")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("forward prewarm request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read prewarm response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("remote prewarm failed (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

func (p *Proxy) forwardPrewarmGRPC(ctx context.Context, addr, functionName string, targetReplicas int) error {
	conn, err := p.getGRPCConn(ctx, addr)
	if err != nil {
		return err
	}

	body, err := json.Marshal(map[string]int{"target_replicas": targetReplicas})
	if err != nil {
		return fmt.Errorf("marshal prewarm request: %w", err)
	}

	client := novapb.NewNovaServiceClient(conn)
	ctx = withClusterForwardedMetadata(ctx)
	resp, err := client.ProxyHTTP(ctx, &novapb.ProxyHTTPRequest{
		Method: "POST",
		Path:   fmt.Sprintf("/functions/%s/prewarm", url.PathEscape(functionName)),
		Body:   body,
		Headers: map[string]string{
			"Content-Type":             "application/json",
			"X-Nova-Cluster-Forwarded": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("grpc prewarm proxy: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("remote prewarm failed (status %d): %s", resp.StatusCode, string(resp.Body))
	}
	return nil
}

func (p *Proxy) getGRPCConn(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	p.connsMu.Lock()
	if conn, ok := p.grpcConns[addr]; ok {
		p.connsMu.Unlock()
		return conn, nil
	}
	p.connsMu.Unlock()

	dialCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial remote gRPC %s: %w", addr, err)
	}

	p.connsMu.Lock()
	if existing, ok := p.grpcConns[addr]; ok {
		p.connsMu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	p.grpcConns[addr] = conn
	p.connsMu.Unlock()

	return conn, nil
}

func normalizeNodeAddress(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return "", false
	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		return raw, true
	case strings.HasPrefix(raw, "grpc://"):
		return strings.TrimPrefix(raw, "grpc://"), false
	default:
		return raw, false
	}
}

func withClusterForwardedMetadata(ctx context.Context) context.Context {
	outgoing, _ := metadata.FromOutgoingContext(ctx)
	md := metadata.MD{}
	for k, v := range outgoing {
		md[k] = append([]string(nil), v...)
	}

	// Preserve common tenant/request scope if this call originates from an
	// inbound gRPC request context.
	if incoming, ok := metadata.FromIncomingContext(ctx); ok {
		for _, key := range []string{"x-nova-tenant", "x-nova-namespace", "x-request-id"} {
			if len(md.Get(key)) > 0 {
				continue
			}
			if values := incoming.Get(key); len(values) > 0 {
				md[key] = append([]string(nil), values...)
			}
		}
	}

	md.Set("x-nova-cluster-forwarded", "true")
	return metadata.NewOutgoingContext(ctx, md)
}
