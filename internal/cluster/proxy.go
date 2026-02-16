package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Proxy forwards function invocations to remote Comet nodes in the cluster.
type Proxy struct {
	client *http.Client
}

// NewProxy creates a cluster proxy with configurable timeout.
func NewProxy(timeout time.Duration) *Proxy {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Proxy{
		client: &http.Client{Timeout: timeout},
	}
}

// ForwardInvoke forwards a function invocation to a remote node.
// The remote node's address should be the Comet gRPC/HTTP endpoint.
func (p *Proxy) ForwardInvoke(ctx context.Context, nodeAddress, functionName string, payload json.RawMessage) (json.RawMessage, error) {
	url := fmt.Sprintf("http://%s/functions/%s/invoke", nodeAddress, url.PathEscape(functionName))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nova-Forwarded", "true")

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
