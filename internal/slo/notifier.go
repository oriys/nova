package slo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
)

// Notifier sends SLO alerts to external channels (webhook, Slack, email).
type Notifier struct {
	client *http.Client
}

// NewNotifier creates a new SLO notifier.
func NewNotifier() *Notifier {
	return &Notifier{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// alertPayload is the JSON body sent to external notification targets.
type alertPayload struct {
	Status       string  `json:"status"`        // "breach" or "recovered"
	Function     string  `json:"function"`
	FunctionID   string  `json:"function_id"`
	Breaches     []string `json:"breaches,omitempty"`
	SuccessRate  float64 `json:"success_rate_pct"`
	P95Latency   int64   `json:"p95_duration_ms"`
	ColdStartPct float64 `json:"cold_start_rate_pct"`
	WindowS      int     `json:"window_seconds"`
	Timestamp    string  `json:"timestamp"`
}

// SendAlerts dispatches SLO alert to all configured external notification targets.
func (n *Notifier) SendAlerts(ctx context.Context, fn *domain.Function, breaches []string, isBreach bool, snapshot *alertSnapshot, targets []domain.SLONotificationTarget) {
	status := "recovered"
	if isBreach {
		status = "breach"
	}

	payload := &alertPayload{
		Status:       status,
		Function:     fn.Name,
		FunctionID:   fn.ID,
		Breaches:     breaches,
		SuccessRate:  snapshot.SuccessRatePct,
		P95Latency:   snapshot.P95DurationMs,
		ColdStartPct: snapshot.ColdStartRatePct,
		WindowS:      snapshot.WindowSeconds,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	for _, target := range targets {
		kind := strings.ToLower(strings.TrimSpace(target.Type))
		switch kind {
		case "webhook":
			go n.sendWebhook(ctx, target, payload)
		case "slack":
			go n.sendSlack(ctx, target, payload)
		case "email":
			go n.sendEmail(ctx, target, payload)
		}
	}
}

// alertSnapshot mirrors the snapshot fields needed for notifications.
type alertSnapshot struct {
	SuccessRatePct  float64
	P95DurationMs   int64
	ColdStartRatePct float64
	WindowSeconds   int
}

func (n *Notifier) sendWebhook(ctx context.Context, target domain.SLONotificationTarget, payload *alertPayload) {
	body, err := json.Marshal(payload)
	if err != nil {
		logging.Op().Warn("slo notifier: marshal webhook payload", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", target.URL, bytes.NewReader(body))
	if err != nil {
		logging.Op().Warn("slo notifier: create webhook request", "url", target.URL, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		logging.Op().Warn("slo notifier: webhook delivery failed", "url", target.URL, "error", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		logging.Op().Warn("slo notifier: webhook returned error", "url", target.URL, "status", resp.StatusCode)
		return
	}
	logging.Op().Debug("slo notifier: webhook delivered", "url", target.URL, "function", payload.Function)
}

func (n *Notifier) sendSlack(ctx context.Context, target domain.SLONotificationTarget, payload *alertPayload) {
	emoji := ":white_check_mark:"
	color := "#36a64f"
	if payload.Status == "breach" {
		emoji = ":rotating_light:"
		color = "#ff0000"
	}

	title := fmt.Sprintf("%s SLO %s: %s", emoji, payload.Status, payload.Function)
	text := fmt.Sprintf(
		"Success: %.2f%% | P95: %dms | Cold Start: %.2f%% | Window: %ds",
		payload.SuccessRate, payload.P95Latency, payload.ColdStartPct, payload.WindowS,
	)
	if len(payload.Breaches) > 0 {
		text += fmt.Sprintf("\nBreaches: %s", strings.Join(payload.Breaches, ", "))
	}

	slackPayload := map[string]interface{}{
		"text": title,
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"text":  text,
				"ts":    time.Now().Unix(),
			},
		},
	}

	body, err := json.Marshal(slackPayload)
	if err != nil {
		logging.Op().Warn("slo notifier: marshal slack payload", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", target.URL, bytes.NewReader(body))
	if err != nil {
		logging.Op().Warn("slo notifier: create slack request", "url", target.URL, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		logging.Op().Warn("slo notifier: slack delivery failed", "url", target.URL, "error", err)
		return
	}
	resp.Body.Close()
	logging.Op().Debug("slo notifier: slack delivered", "function", payload.Function)
}

func (n *Notifier) sendEmail(ctx context.Context, target domain.SLONotificationTarget, payload *alertPayload) {
	// Email notification is a placeholder — in production this would
	// integrate with an SMTP gateway or transactional email service
	// (e.g. SendGrid, SES). The target.URL would contain the recipient
	// or SMTP endpoint.
	logging.Op().Info("slo notifier: email alert (placeholder)",
		"to", target.URL,
		"function", payload.Function,
		"status", payload.Status,
	)
}
