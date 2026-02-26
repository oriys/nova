package triggers

import (
"bytes"
"context"
"encoding/json"
"fmt"
"net"
"net/url"
"net/http"
"time"

"github.com/oriys/nova/internal/logging"
)

// WebhookSink forwards function execution results to an HTTP endpoint
type WebhookSink struct {
url        string
method     string
headers    map[string]string
timeout    time.Duration
maxRetries int
client     *http.Client
}

// WebhookSinkConfig defines webhook sink configuration
type WebhookSinkConfig struct {
URL        string            `json:"url"`
Method     string            `json:"method"`      // Default: POST
Headers    map[string]string `json:"headers"`
TimeoutS   int               `json:"timeout_s"`   // Default: 30
MaxRetries int               `json:"max_retries"` // Default: 3
}

// NewWebhookSink creates a new webhook sink
func NewWebhookSink(config *WebhookSinkConfig) (*WebhookSink, error) {
if config.URL == "" {
return nil, fmt.Errorf("webhook URL is required")
}

// SSRF protection: block private/internal network targets
if err := validateWebhookURL(config.URL); err != nil {
return nil, fmt.Errorf("invalid webhook URL: %w", err)
}

if config.Method == "" {
config.Method = "POST"
}

if config.TimeoutS <= 0 {
config.TimeoutS = 30
}

if config.MaxRetries <= 0 {
config.MaxRetries = 3
}

return &WebhookSink{
url:        config.URL,
method:     config.Method,
headers:    config.Headers,
timeout:    time.Duration(config.TimeoutS) * time.Second,
maxRetries: config.MaxRetries,
client: &http.Client{
Timeout: time.Duration(config.TimeoutS) * time.Second,
},
}, nil
}

// SendResult sends a function execution result to the webhook
func (ws *WebhookSink) SendResult(ctx context.Context, result interface{}) error {
payload, err := json.Marshal(result)
if err != nil {
return fmt.Errorf("marshal result: %w", err)
}

var lastErr error
for attempt := 0; attempt <= ws.maxRetries; attempt++ {
if attempt > 0 {
time.Sleep(time.Duration(attempt) * time.Second)
}

req, err := http.NewRequestWithContext(ctx, ws.method, ws.url, bytes.NewReader(payload))
if err != nil {
return fmt.Errorf("create request: %w", err)
}

req.Header.Set("Content-Type", "application/json")
for k, v := range ws.headers {
req.Header.Set(k, v)
}

resp, err := ws.client.Do(req)
if err != nil {
lastErr = err
logging.Op().Warn("webhook request failed", "attempt", attempt+1, "url", ws.url, "error", err)
continue
}

resp.Body.Close()

if resp.StatusCode >= 200 && resp.StatusCode < 300 {
return nil
}

lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
logging.Op().Warn("webhook returned error status", "attempt", attempt+1, "url", ws.url, "status", resp.StatusCode)
}

return fmt.Errorf("webhook failed after %d attempts: %w", ws.maxRetries+1, lastErr)
}

// validateWebhookURL validates that the webhook URL does not target private/internal networks.
func validateWebhookURL(rawURL string) error {
u, err := url.Parse(rawURL)
if err != nil {
return fmt.Errorf("invalid URL: %w", err)
}
if u.Scheme != "http" && u.Scheme != "https" {
return fmt.Errorf("only http/https schemes allowed, got %s", u.Scheme)
}
host := u.Hostname()
if host == "" {
return fmt.Errorf("empty hostname")
}
ips, err := net.LookupIP(host)
if err != nil {
return fmt.Errorf("DNS resolution failed for %s: %w", host, err)
}
for _, ip := range ips {
if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
return fmt.Errorf("blocked: %s resolves to private/reserved IP %s", host, ip)
}
}
return nil
}
