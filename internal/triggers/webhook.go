package triggers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/logging"
)

// WebhookConnector receives HTTP POST requests and triggers function invocations.
// Config keys: "listen_addr" (e.g. ":8090"), "path" (e.g. "/webhook"), "secret" (optional HMAC-SHA256).
type WebhookConnector struct {
	trigger    *Trigger
	handler    EventHandler
	listenAddr string
	path       string
	secret     string
	server     *http.Server
	healthy    bool
	mu         sync.Mutex
	doneCh     chan struct{}
}

// NewWebhookConnector creates a connector that starts an HTTP server for inbound webhooks.
func NewWebhookConnector(trigger *Trigger, handler EventHandler) (*WebhookConnector, error) {
	listenAddr := ":8090"
	if v, ok := trigger.Config["listen_addr"].(string); ok && v != "" {
		listenAddr = v
	}

	path := "/webhook"
	if v, ok := trigger.Config["path"].(string); ok && v != "" {
		path = v
	}

	var secret string
	if v, ok := trigger.Config["secret"].(string); ok {
		secret = v
	}

	return &WebhookConnector{
		trigger:    trigger,
		handler:    handler,
		listenAddr: listenAddr,
		path:       path,
		secret:     secret,
		doneCh:     make(chan struct{}),
	}, nil
}

func (w *WebhookConnector) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(w.path, w.handleRequest)

	w.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", w.listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", w.listenAddr, err)
	}

	w.mu.Lock()
	w.healthy = true
	w.mu.Unlock()

	go func() {
		defer close(w.doneCh)
		if err := w.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			logging.Op().Warn("webhook server error", "trigger", w.trigger.ID, "error", err)
			w.mu.Lock()
			w.healthy = false
			w.mu.Unlock()
		}
	}()

	logging.Op().Info("webhook connector started",
		"trigger", w.trigger.ID, "addr", w.listenAddr, "path", w.path)
	return nil
}

func (w *WebhookConnector) handleRequest(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(rw, "read body failed", http.StatusBadRequest)
		return
	}

	if w.secret != "" {
		sig := r.Header.Get("X-Nova-Signature")
		if !w.verifySignature(body, sig) {
			http.Error(rw, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	event := &TriggerEvent{
		TriggerID: w.trigger.ID,
		EventID:   uuid.New().String(),
		Source:    fmt.Sprintf("webhook://%s%s", r.Host, r.URL.Path),
		Type:      "webhook.request",
		Data:      json.RawMessage(body),
		Metadata: map[string]interface{}{
			"content_type": r.Header.Get("Content-Type"),
			"remote_addr":  r.RemoteAddr,
		},
		Timestamp: time.Now(),
	}

	if err := w.handler.Handle(r.Context(), event); err != nil {
		logging.Op().Warn("webhook dispatch failed", "trigger", w.trigger.ID, "error", err)
		http.Error(rw, "dispatch failed", http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(rw, `{"event_id":%q}`, event.EventID)
}

func (w *WebhookConnector) verifySignature(body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(w.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (w *WebhookConnector) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := w.server.Shutdown(ctx)
	<-w.doneCh

	w.mu.Lock()
	w.healthy = false
	w.mu.Unlock()

	logging.Op().Info("webhook connector stopped", "trigger", w.trigger.ID)
	return err
}

func (w *WebhookConnector) Type() TriggerType { return TriggerTypeWebhook }

func (w *WebhookConnector) IsHealthy() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.healthy
}
