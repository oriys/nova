package eventbus

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/store"
)

// WebhookResult captures the full request/response audit trail of a webhook delivery.
type WebhookResult struct {
	Request  WebhookRequestRecord  `json:"request"`
	Response WebhookResponseRecord `json:"response"`
}

// WebhookRequestRecord records what was sent.
type WebhookRequestRecord struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	BodyBytes int               `json:"body_bytes"`
}

// WebhookResponseRecord records what was received.
type WebhookResponseRecord struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// WebhookError indicates a non-2xx response from the webhook endpoint.
type WebhookError struct {
	StatusCode int
	Body       string
}

func (e *WebhookError) Error() string {
	return fmt.Sprintf("webhook returned status %d", e.StatusCode)
}

const maxWebhookResponseBody = 64 * 1024 // 64KB

// deliverWebhook sends the event payload to the webhook URL.
// Returns the full audit result and any error.
func (w *WorkerPool) deliverWebhook(ctx context.Context, delivery *store.EventDelivery, payload json.RawMessage) (*WebhookResult, int64, error) {
	// Check outbound ACL (SSRF protection)
	if err := checkOutboundACL(delivery.WebhookURL); err != nil {
		return nil, 0, fmt.Errorf("outbound ACL blocked: %w", err)
	}

	// Build HTTP request
	body := []byte(payload)
	method := delivery.WebhookMethod
	if method == "" {
		method = "POST"
	}
	req, err := http.NewRequestWithContext(ctx, method, delivery.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("create webhook request: %w", err)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Nova-Webhook/1.0")
	req.Header.Set("X-Nova-Delivery-ID", delivery.ID)
	req.Header.Set("X-Nova-Topic", delivery.TopicName)
	req.Header.Set("X-Nova-Message-ID", delivery.MessageID)

	// Apply custom headers
	if len(delivery.WebhookHeaders) > 0 {
		var headers map[string]string
		if err := json.Unmarshal(delivery.WebhookHeaders, &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	// HMAC-SHA256 signing
	if delivery.WebhookSigningSecret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := signWebhookPayload(delivery.WebhookSigningSecret, timestamp, body)
		req.Header.Set("X-Nova-Signature", signature)
		req.Header.Set("X-Nova-Timestamp", timestamp)
	}

	// Send request with timeout
	timeout := time.Duration(delivery.WebhookTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	start := time.Now()
	resp, err := client.Do(req)
	durationMS := time.Since(start).Milliseconds()

	result := &WebhookResult{
		Request: WebhookRequestRecord{
			URL:       delivery.WebhookURL,
			Method:    method,
			Headers:   flattenHeaders(req.Header),
			BodyBytes: len(body),
		},
	}

	if err != nil {
		result.Response = WebhookResponseRecord{
			Status: 0,
			Body:   err.Error(),
		}
		return result, durationMS, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (limited)
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxWebhookResponseBody))

	result.Response = WebhookResponseRecord{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    string(respBody),
	}

	// Non-2xx is a delivery failure (will be retried)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, durationMS, &WebhookError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return result, durationMS, nil
}

// signWebhookPayload generates an HMAC-SHA256 signature in the format "v1=<hex>".
// The signed content is: timestamp.body
func signWebhookPayload(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// checkOutboundACL validates that the webhook URL is not targeting private/internal networks.
func checkOutboundACL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http/https
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("blocked: only http/https schemes allowed, got %s", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("blocked: empty hostname")
	}

	// Resolve hostname to check IPs
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

// flattenHeaders converts http.Header to a simple map for audit logging.
func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vals := range h {
		// Redact sensitive headers in audit
		lower := strings.ToLower(k)
		if lower == "authorization" || lower == "x-nova-signature" {
			out[k] = "[REDACTED]"
		} else {
			out[k] = strings.Join(vals, ", ")
		}
	}
	return out
}
