package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// NovaClient wraps HTTP calls to the Nova API.
type NovaClient struct {
	BaseURL   string
	APIKey    string
	Tenant    string
	Namespace string
	client    *http.Client
}

func NewNovaClient(cfg *Config) *NovaClient {
	return &NovaClient{
		BaseURL:   cfg.URL,
		APIKey:    cfg.APIKey,
		Tenant:    cfg.Tenant,
		Namespace: cfg.Namespace,
		client:    &http.Client{},
	}
}

func (c *NovaClient) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	if c.Tenant != "" {
		req.Header.Set("X-Tenant-ID", c.Tenant)
	}
	if c.Namespace != "" {
		req.Header.Set("X-Namespace", c.Namespace)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if len(respBody) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return json.RawMessage(respBody), nil
}

func (c *NovaClient) Get(ctx context.Context, path string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *NovaClient) Post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

func (c *NovaClient) Put(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPut, path, body)
}

func (c *NovaClient) Patch(ctx context.Context, path string, body any) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPatch, path, body)
}

func (c *NovaClient) Delete(ctx context.Context, path string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodDelete, path, nil)
}
