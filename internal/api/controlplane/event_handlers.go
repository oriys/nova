package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/store"
)

type createEventTopicRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	RetentionHours int    `json:"retention_hours"`
}

type publishEventRequest struct {
	Payload     json.RawMessage `json:"payload"`
	Headers     json.RawMessage `json:"headers"`
	OrderingKey string          `json:"ordering_key"`
}

type createEventSubscriptionRequest struct {
	Name          string `json:"name"`
	ConsumerGroup string `json:"consumer_group"`
	Enabled       *bool  `json:"enabled,omitempty"`
	MaxAttempts   int    `json:"max_attempts,omitempty"`
	BackoffBaseMS int    `json:"backoff_base_ms,omitempty"`
	BackoffMaxMS  int    `json:"backoff_max_ms,omitempty"`
	MaxInflight   int    `json:"max_inflight,omitempty"`
	RateLimitPerS int    `json:"rate_limit_per_sec,omitempty"`

	// Subscription type: "function" (default) or "workflow"
	Type string `json:"type,omitempty"`

	// Function fields (required when type=function)
	FunctionName string `json:"function_name"`
	WorkflowName string `json:"workflow_name,omitempty"`

	// Webhook fields (optional for workflow subscriptions)
	WebhookURL           string          `json:"webhook_url,omitempty"`
	WebhookMethod        string          `json:"webhook_method,omitempty"`
	WebhookHeaders       json.RawMessage `json:"webhook_headers,omitempty"`
	WebhookSigningSecret string          `json:"webhook_signing_secret,omitempty"`
	WebhookTimeoutMS     int             `json:"webhook_timeout_ms,omitempty"`
}

type updateEventSubscriptionRequest struct {
	Name          *string `json:"name,omitempty"`
	ConsumerGroup *string `json:"consumer_group,omitempty"`
	FunctionName  *string `json:"function_name,omitempty"`
	WorkflowName  *string `json:"workflow_name,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
	MaxAttempts   *int    `json:"max_attempts,omitempty"`
	BackoffBaseMS *int    `json:"backoff_base_ms,omitempty"`
	BackoffMaxMS  *int    `json:"backoff_max_ms,omitempty"`
	MaxInflight   *int    `json:"max_inflight,omitempty"`
	RateLimitPerS *int    `json:"rate_limit_per_sec,omitempty"`

	// Webhook fields
	WebhookURL           *string         `json:"webhook_url,omitempty"`
	WebhookMethod        *string         `json:"webhook_method,omitempty"`
	WebhookHeaders       json.RawMessage `json:"webhook_headers,omitempty"`
	WebhookSigningSecret *string         `json:"webhook_signing_secret,omitempty"`
	WebhookTimeoutMS     *int            `json:"webhook_timeout_ms,omitempty"`
}

type replayEventSubscriptionRequest struct {
	FromSequence int64  `json:"from_sequence,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	FromTime     string `json:"from_time,omitempty"`
	ResetCursor  bool   `json:"reset_cursor,omitempty"`
}

type seekEventSubscriptionRequest struct {
	FromSequence int64  `json:"from_sequence,omitempty"`
	FromTime     string `json:"from_time,omitempty"`
}

type createEventOutboxRequest struct {
	Payload       json.RawMessage `json:"payload"`
	Headers       json.RawMessage `json:"headers"`
	OrderingKey   string          `json:"ordering_key"`
	MaxAttempts   int             `json:"max_attempts,omitempty"`
	BackoffBaseMS int             `json:"backoff_base_ms,omitempty"`
	BackoffMaxMS  int             `json:"backoff_max_ms,omitempty"`
}

type retryEventOutboxRequest struct {
	MaxAttempts int `json:"max_attempts,omitempty"`
}

type retryEventDeliveryRequest struct {
	MaxAttempts int `json:"max_attempts,omitempty"`
}

func (h *Handler) CreateEventTopic(w http.ResponseWriter, r *http.Request) {
	req := createEventTopicRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	topic := store.NewEventTopic(req.Name, req.Description)
	if req.RetentionHours > 0 {
		topic.RetentionHours = req.RetentionHours
	}
	if err := h.Store.CreateEventTopic(r.Context(), topic); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(topic)
}

func (h *Handler) ListEventTopics(w http.ResponseWriter, r *http.Request) {
	limit := parseEventLimitQuery(r.URL.Query().Get("limit"), store.DefaultEventListLimit, store.MaxEventListLimit)
	topics, err := h.Store.ListEventTopics(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if topics == nil {
		topics = []*store.EventTopic{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(topics)
}

func (h *Handler) GetEventTopic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	topic, err := h.Store.GetEventTopicByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(topic)
}

func (h *Handler) DeleteEventTopic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.Store.DeleteEventTopicByName(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "name": name})
}

func (h *Handler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	topicName := r.PathValue("name")
	topic, err := h.Store.GetEventTopicByName(r.Context(), topicName)
	if err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req := publishEventRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	scope := store.TenantScopeFromContext(r.Context())
	decision, err := h.Store.CheckAndConsumeTenantQuota(r.Context(), scope.TenantID, store.TenantDimensionEventPublishes, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if decision != nil && !decision.Allowed {
		writeTenantQuotaExceededResponse(w, decision)
		return
	}

	msg, fanout, err := h.Store.PublishEvent(r.Context(), topic.ID, req.OrderingKey, req.Payload, req.Headers)
	if err != nil {
		if errors.Is(err, store.ErrInvalidOrderingKey) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"message":    msg,
		"deliveries": fanout,
	})
}

func (h *Handler) CreateEventOutbox(w http.ResponseWriter, r *http.Request) {
	topicName := r.PathValue("name")
	topic, err := h.Store.GetEventTopicByName(r.Context(), topicName)
	if err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req := createEventOutboxRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	scope := store.TenantScopeFromContext(r.Context())
	decision, err := h.Store.CheckAndConsumeTenantQuota(r.Context(), scope.TenantID, store.TenantDimensionEventPublishes, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if decision != nil && !decision.Allowed {
		writeTenantQuotaExceededResponse(w, decision)
		return
	}

	job := store.NewEventOutbox(topic.ID, topic.Name, req.OrderingKey, req.Payload, req.Headers)
	if req.MaxAttempts > 0 {
		job.MaxAttempts = req.MaxAttempts
	}
	if req.BackoffBaseMS > 0 {
		job.BackoffBaseMS = req.BackoffBaseMS
	}
	if req.BackoffMaxMS > 0 {
		job.BackoffMaxMS = req.BackoffMaxMS
	}
	if err := h.Store.CreateEventOutbox(r.Context(), job); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/outbox/"+job.ID)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) ListEventOutbox(w http.ResponseWriter, r *http.Request) {
	topicName := r.PathValue("name")
	topic, err := h.Store.GetEventTopicByName(r.Context(), topicName)
	if err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit := parseEventLimitQuery(r.URL.Query().Get("limit"), store.DefaultEventListLimit, store.MaxEventListLimit)
	statuses, err := parseOutboxStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	jobs, err := h.Store.ListEventOutbox(r.Context(), topic.ID, limit, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []*store.EventOutbox{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (h *Handler) ListTopicMessages(w http.ResponseWriter, r *http.Request) {
	topicName := r.PathValue("name")
	topic, err := h.Store.GetEventTopicByName(r.Context(), topicName)
	if err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	limit := parseEventLimitQuery(r.URL.Query().Get("limit"), store.DefaultEventListLimit, store.MaxEventListLimit)
	messages, err := h.Store.ListEventMessages(r.Context(), topic.ID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if messages == nil {
		messages = []*store.EventMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (h *Handler) CreateEventSubscription(w http.ResponseWriter, r *http.Request) {
	topicName := r.PathValue("name")
	topic, err := h.Store.GetEventTopicByName(r.Context(), topicName)
	if err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req := createEventSubscriptionRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	subType := strings.TrimSpace(req.Type)
	if subType == "" {
		subType = string(store.EventSubscriptionTypeFunction)
	}

	var sub *store.EventSubscription

	switch store.EventSubscriptionType(subType) {
	case store.EventSubscriptionTypeFunction:
		if strings.TrimSpace(req.FunctionName) == "" {
			http.Error(w, "function_name is required for function subscriptions", http.StatusBadRequest)
			return
		}
		fn, err := h.Store.GetFunctionByName(r.Context(), req.FunctionName)
		if err != nil {
			http.Error(w, "function not found: "+req.FunctionName, http.StatusNotFound)
			return
		}
		sub = store.NewEventSubscription(topic.ID, topic.Name, req.Name, req.ConsumerGroup, fn.ID, fn.Name)

	case store.EventSubscriptionTypeWorkflow:
		if strings.TrimSpace(req.WorkflowName) == "" {
			http.Error(w, "workflow_name is required for workflow subscriptions", http.StatusBadRequest)
			return
		}
		if h.WorkflowService == nil {
			http.Error(w, "workflow service is not configured", http.StatusInternalServerError)
			return
		}
		wf, err := h.WorkflowService.GetWorkflow(r.Context(), req.WorkflowName)
		if err != nil {
			http.Error(w, "workflow not found: "+req.WorkflowName, http.StatusNotFound)
			return
		}
		sub = store.NewWorkflowSubscription(topic.ID, topic.Name, req.Name, req.ConsumerGroup, wf.ID, wf.Name)
		sub.WebhookURL = strings.TrimSpace(req.WebhookURL)
		sub.WebhookMethod = strings.TrimSpace(req.WebhookMethod)
		if len(req.WebhookHeaders) > 0 {
			sub.WebhookHeaders = req.WebhookHeaders
		}
		sub.WebhookSigningSecret = req.WebhookSigningSecret
		if req.WebhookTimeoutMS > 0 {
			sub.WebhookTimeoutMS = req.WebhookTimeoutMS
		}

	default:
		http.Error(w, "invalid type: must be 'function' or 'workflow'", http.StatusBadRequest)
		return
	}

	if req.Enabled != nil {
		sub.Enabled = *req.Enabled
	}
	if req.MaxAttempts > 0 {
		sub.MaxAttempts = req.MaxAttempts
	}
	if req.BackoffBaseMS > 0 {
		sub.BackoffBaseMS = req.BackoffBaseMS
	}
	if req.BackoffMaxMS > 0 {
		sub.BackoffMaxMS = req.BackoffMaxMS
	}
	sub.MaxInflight = req.MaxInflight
	sub.RateLimitPerSec = req.RateLimitPerS

	if err := h.Store.CreateEventSubscription(r.Context(), sub); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sub)
}

func (h *Handler) ListEventSubscriptions(w http.ResponseWriter, r *http.Request) {
	topicName := r.PathValue("name")
	topic, err := h.Store.GetEventTopicByName(r.Context(), topicName)
	if err != nil {
		if errors.Is(err, store.ErrEventTopicNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	subs, err := h.Store.ListEventSubscriptions(r.Context(), topic.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if subs == nil {
		subs = []*store.EventSubscription{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subs)
}

func (h *Handler) GetEventSubscription(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sub, err := h.Store.GetEventSubscription(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

func (h *Handler) UpdateEventSubscription(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req := updateEventSubscriptionRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	update := &store.EventSubscriptionUpdate{
		Name:          req.Name,
		ConsumerGroup: req.ConsumerGroup,
		Enabled:       req.Enabled,
		MaxAttempts:   req.MaxAttempts,
		BackoffBaseMS: req.BackoffBaseMS,
		BackoffMaxMS:  req.BackoffMaxMS,
		MaxInflight:   req.MaxInflight,
		RateLimitPerS: req.RateLimitPerS,
		// Webhook fields
		WebhookURL:           req.WebhookURL,
		WebhookMethod:        req.WebhookMethod,
		WebhookHeaders:       req.WebhookHeaders,
		WebhookSigningSecret: req.WebhookSigningSecret,
		WebhookTimeoutMS:     req.WebhookTimeoutMS,
	}

	if req.FunctionName != nil {
		fnName := strings.TrimSpace(*req.FunctionName)
		if fnName == "" {
			http.Error(w, "function_name cannot be empty", http.StatusBadRequest)
			return
		}
		fn, err := h.Store.GetFunctionByName(r.Context(), fnName)
		if err != nil {
			http.Error(w, "function not found: "+fnName, http.StatusNotFound)
			return
		}
		update.FunctionName = &fn.Name
		update.FunctionID = &fn.ID
	}
	if req.WorkflowName != nil {
		wfName := strings.TrimSpace(*req.WorkflowName)
		if wfName == "" {
			http.Error(w, "workflow_name cannot be empty", http.StatusBadRequest)
			return
		}
		if h.WorkflowService == nil {
			http.Error(w, "workflow service is not configured", http.StatusInternalServerError)
			return
		}
		wf, err := h.WorkflowService.GetWorkflow(r.Context(), wfName)
		if err != nil {
			http.Error(w, "workflow not found: "+wfName, http.StatusNotFound)
			return
		}
		update.WorkflowName = &wf.Name
		update.WorkflowID = &wf.ID
	}

	sub, err := h.Store.UpdateEventSubscription(r.Context(), id, update)
	if err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

func (h *Handler) DeleteEventSubscription(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteEventSubscription(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": id})
}

func (h *Handler) GetEventDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	delivery, err := h.Store.GetEventDelivery(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrEventDeliveryNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(delivery)
}

func (h *Handler) ListEventDeliveries(w http.ResponseWriter, r *http.Request) {
	subscriptionID := r.PathValue("id")
	limit := parseEventLimitQuery(r.URL.Query().Get("limit"), store.DefaultEventListLimit, store.MaxEventListLimit)
	statuses, err := parseEventStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	deliveries, err := h.Store.ListEventDeliveries(r.Context(), subscriptionID, limit, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if deliveries == nil {
		deliveries = []*store.EventDelivery{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deliveries)
}

func (h *Handler) ReplayEventSubscription(w http.ResponseWriter, r *http.Request) {
	subscriptionID := r.PathValue("id")
	req := replayEventSubscriptionRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	fromSequence := req.FromSequence
	if strings.TrimSpace(req.FromTime) != "" {
		fromTime, err := time.Parse(time.RFC3339, strings.TrimSpace(req.FromTime))
		if err != nil {
			http.Error(w, "from_time must be RFC3339", http.StatusBadRequest)
			return
		}
		seq, err := h.Store.ResolveEventReplaySequenceByTime(r.Context(), subscriptionID, fromTime)
		if err != nil {
			if errors.Is(err, store.ErrEventSubscriptionNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fromSequence = seq
	}
	if fromSequence <= 0 {
		fromSequence = 1
	}
	if req.ResetCursor {
		_, err := h.Store.SetEventSubscriptionCursor(r.Context(), subscriptionID, fromSequence-1)
		if err != nil {
			if errors.Is(err, store.ErrEventSubscriptionNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	count, err := h.Store.ReplayEventSubscription(r.Context(), subscriptionID, fromSequence, req.Limit)
	if err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "replayed",
		"subscriptionId": subscriptionID,
		"from_sequence":  fromSequence,
		"queued":         count,
	})
}

func (h *Handler) SeekEventSubscription(w http.ResponseWriter, r *http.Request) {
	subscriptionID := r.PathValue("id")
	req := seekEventSubscriptionRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	fromSequence := req.FromSequence
	if strings.TrimSpace(req.FromTime) != "" {
		fromTime, err := time.Parse(time.RFC3339, strings.TrimSpace(req.FromTime))
		if err != nil {
			http.Error(w, "from_time must be RFC3339", http.StatusBadRequest)
			return
		}
		seq, err := h.Store.ResolveEventReplaySequenceByTime(r.Context(), subscriptionID, fromTime)
		if err != nil {
			if errors.Is(err, store.ErrEventSubscriptionNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fromSequence = seq
	}
	if fromSequence <= 0 {
		fromSequence = 1
	}

	sub, err := h.Store.SetEventSubscriptionCursor(r.Context(), subscriptionID, fromSequence-1)
	if err != nil {
		if errors.Is(err, store.ErrEventSubscriptionNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "seeked",
		"subscriptionId": subscriptionID,
		"from_sequence":  fromSequence,
		"subscription":   sub,
	})
}

func (h *Handler) RetryEventDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req := retryEventDeliveryRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	delivery, err := h.Store.RequeueEventDelivery(r.Context(), id, req.MaxAttempts)
	if err != nil {
		if errors.Is(err, store.ErrEventDeliveryNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrEventDeliveryNotDLQ) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(delivery)
}

func (h *Handler) RetryEventOutbox(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req := retryEventOutboxRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	job, err := h.Store.RequeueEventOutbox(r.Context(), id, req.MaxAttempts)
	if err != nil {
		if errors.Is(err, store.ErrEventOutboxNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrEventOutboxNotFailed) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func parseEventLimitQuery(raw string, fallback, max int) int {
	limit := fallback
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = fallback
	}
	if max > 0 && limit > max {
		limit = max
	}
	return limit
}

func parseEventStatuses(raw string) ([]store.EventDeliveryStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	statuses := make([]store.EventDeliveryStatus, 0, len(parts))
	for _, part := range parts {
		status := store.EventDeliveryStatus(strings.TrimSpace(part))
		if status == "" {
			continue
		}
		switch status {
		case store.EventDeliveryStatusQueued,
			store.EventDeliveryStatusRunning,
			store.EventDeliveryStatusSucceeded,
			store.EventDeliveryStatusDLQ:
			statuses = append(statuses, status)
		default:
			return nil, fmt.Errorf("invalid status: %s", status)
		}
	}
	return statuses, nil
}

func parseOutboxStatuses(raw string) ([]store.EventOutboxStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	statuses := make([]store.EventOutboxStatus, 0, len(parts))
	for _, part := range parts {
		status := store.EventOutboxStatus(strings.TrimSpace(part))
		if status == "" {
			continue
		}
		switch status {
		case store.EventOutboxStatusPending,
			store.EventOutboxStatusPublishing,
			store.EventOutboxStatusPublished,
			store.EventOutboxStatusFailed:
			statuses = append(statuses, status)
		default:
			return nil, fmt.Errorf("invalid outbox status: %s", status)
		}
	}
	return statuses, nil
}
