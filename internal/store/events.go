package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// EventDeliveryStatus values.
type EventDeliveryStatus string

const (
	EventDeliveryStatusQueued    EventDeliveryStatus = "queued"
	EventDeliveryStatusRunning   EventDeliveryStatus = "running"
	EventDeliveryStatusSucceeded EventDeliveryStatus = "succeeded"
	EventDeliveryStatusDLQ       EventDeliveryStatus = "dlq"
)

const (
	DefaultEventRetentionHours = 168 // 7 days
	DefaultEventListLimit      = 50
	MaxEventListLimit          = 500
	DefaultEventReplayLimit    = 100
	MaxEventReplayLimit        = 2000
	DefaultEventMaxAttempts    = 3
	DefaultEventBackoffBaseMS  = 1000
	DefaultEventBackoffMaxMS   = 60000
	DefaultEventMaxInflight    = 0 // 0 means unlimited
	DefaultEventRateLimitPerS  = 0 // 0 means unlimited
	DefaultEventLeaseTimeout   = 30 * time.Second
)

var (
	ErrEventTopicNotFound        = errors.New("event topic not found")
	ErrEventSubscriptionNotFound = errors.New("event subscription not found")
	ErrEventDeliveryNotFound     = errors.New("event delivery not found")
	ErrEventDeliveryNotDLQ       = errors.New("event delivery is not in dlq")
	ErrInvalidEventTopicName     = errors.New("invalid event topic name")
	ErrInvalidEventSubName       = errors.New("invalid event subscription name")
	ErrInvalidConsumerGroup      = errors.New("invalid consumer group")
	ErrInvalidOrderingKey        = errors.New("invalid ordering key")

	eventNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

// EventTopic is a publish/subscribe topic.
type EventTopic struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	RetentionHours int       `json:"retention_hours"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// EventSubscription binds a topic to a function consumer group.
type EventSubscription struct {
	ID              string     `json:"id"`
	TopicID         string     `json:"topic_id"`
	TopicName       string     `json:"topic_name,omitempty"`
	Name            string     `json:"name"`
	ConsumerGroup   string     `json:"consumer_group"`
	FunctionID      string     `json:"function_id"`
	FunctionName    string     `json:"function_name"`
	Enabled         bool       `json:"enabled"`
	MaxAttempts     int        `json:"max_attempts"`
	BackoffBaseMS   int        `json:"backoff_base_ms"`
	BackoffMaxMS    int        `json:"backoff_max_ms"`
	MaxInflight     int        `json:"max_inflight"`
	RateLimitPerSec int        `json:"rate_limit_per_sec"`
	LastDispatchAt  *time.Time `json:"last_dispatch_at,omitempty"`
	LastAckedSeq    int64      `json:"last_acked_sequence"`
	LastAckedAt     *time.Time `json:"last_acked_at,omitempty"`
	Lag             int64      `json:"lag"`
	Inflight        int        `json:"inflight"`
	Queued          int        `json:"queued"`
	DLQ             int        `json:"dlq"`
	OldestUnackedS  int64      `json:"oldest_unacked_age_s,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// EventSubscriptionUpdate describes mutable subscription fields.
type EventSubscriptionUpdate struct {
	Name          *string `json:"name,omitempty"`
	ConsumerGroup *string `json:"consumer_group,omitempty"`
	FunctionID    *string `json:"function_id,omitempty"`
	FunctionName  *string `json:"function_name,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
	MaxAttempts   *int    `json:"max_attempts,omitempty"`
	BackoffBaseMS *int    `json:"backoff_base_ms,omitempty"`
	BackoffMaxMS  *int    `json:"backoff_max_ms,omitempty"`
	MaxInflight   *int    `json:"max_inflight,omitempty"`
	RateLimitPerS *int    `json:"rate_limit_per_sec,omitempty"`
}

// EventMessage is an immutable record stored under a topic.
type EventMessage struct {
	ID          string          `json:"id"`
	TopicID     string          `json:"topic_id"`
	TopicName   string          `json:"topic_name,omitempty"`
	Sequence    int64           `json:"sequence"`
	OrderingKey string          `json:"ordering_key,omitempty"`
	Payload     json.RawMessage `json:"payload"`
	Headers     json.RawMessage `json:"headers,omitempty"`
	PublishedAt time.Time       `json:"published_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

// EventDelivery tracks fanout delivery lifecycle for a subscription.
type EventDelivery struct {
	ID               string              `json:"id"`
	TopicID          string              `json:"topic_id"`
	TopicName        string              `json:"topic_name,omitempty"`
	SubscriptionID   string              `json:"subscription_id"`
	SubscriptionName string              `json:"subscription_name,omitempty"`
	ConsumerGroup    string              `json:"consumer_group,omitempty"`
	MessageID        string              `json:"message_id"`
	MessageSequence  int64               `json:"message_sequence"`
	OrderingKey      string              `json:"ordering_key,omitempty"`
	Payload          json.RawMessage     `json:"payload"`
	Headers          json.RawMessage     `json:"headers,omitempty"`
	Status           EventDeliveryStatus `json:"status"`
	Attempt          int                 `json:"attempt"`
	MaxAttempts      int                 `json:"max_attempts"`
	BackoffBaseMS    int                 `json:"backoff_base_ms"`
	BackoffMaxMS     int                 `json:"backoff_max_ms"`
	NextRunAt        time.Time           `json:"next_run_at"`
	LockedBy         string              `json:"locked_by,omitempty"`
	LockedUntil      *time.Time          `json:"locked_until,omitempty"`
	FunctionID       string              `json:"function_id"`
	FunctionName     string              `json:"function_name"`
	RequestID        string              `json:"request_id,omitempty"`
	Output           json.RawMessage     `json:"output,omitempty"`
	DurationMS       int64               `json:"duration_ms,omitempty"`
	ColdStart        bool                `json:"cold_start"`
	LastError        string              `json:"last_error,omitempty"`
	StartedAt        *time.Time          `json:"started_at,omitempty"`
	CompletedAt      *time.Time          `json:"completed_at,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// NewEventTopic creates a topic with defaults.
func NewEventTopic(name, description string) *EventTopic {
	now := time.Now().UTC()
	return &EventTopic{
		ID:             uuid.New().String(),
		Name:           strings.TrimSpace(name),
		Description:    strings.TrimSpace(description),
		RetentionHours: DefaultEventRetentionHours,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// NewEventSubscription creates a subscription with defaults.
func NewEventSubscription(topicID, topicName, name, consumerGroup, functionID, functionName string) *EventSubscription {
	now := time.Now().UTC()
	return &EventSubscription{
		ID:              uuid.New().String(),
		TopicID:         strings.TrimSpace(topicID),
		TopicName:       strings.TrimSpace(topicName),
		Name:            strings.TrimSpace(name),
		ConsumerGroup:   strings.TrimSpace(consumerGroup),
		FunctionID:      strings.TrimSpace(functionID),
		FunctionName:    strings.TrimSpace(functionName),
		Enabled:         true,
		MaxAttempts:     DefaultEventMaxAttempts,
		BackoffBaseMS:   DefaultEventBackoffBaseMS,
		BackoffMaxMS:    DefaultEventBackoffMaxMS,
		MaxInflight:     DefaultEventMaxInflight,
		RateLimitPerSec: DefaultEventRateLimitPerS,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (s *PostgresStore) CreateEventTopic(ctx context.Context, topic *EventTopic) error {
	if topic == nil {
		return fmt.Errorf("event topic is required")
	}
	if err := normalizeEventTopic(topic); err != nil {
		return err
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO event_topics (id, name, description, retention_hours, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, topic.ID, topic.Name, topic.Description, topic.RetentionHours, topic.CreatedAt, topic.UpdatedAt)
	if err != nil {
		if isPGUniqueViolation(err) {
			return fmt.Errorf("event topic already exists: %s", topic.Name)
		}
		return fmt.Errorf("create event topic: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetEventTopic(ctx context.Context, id string) (*EventTopic, error) {
	topic, err := scanEventTopic(s.pool.QueryRow(ctx, `
		SELECT id, name, description, retention_hours, created_at, updated_at
		FROM event_topics
		WHERE id = $1
	`, id))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrEventTopicNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get event topic: %w", err)
	}
	return topic, nil
}

func (s *PostgresStore) GetEventTopicByName(ctx context.Context, name string) (*EventTopic, error) {
	topicName := strings.TrimSpace(name)
	topic, err := scanEventTopic(s.pool.QueryRow(ctx, `
		SELECT id, name, description, retention_hours, created_at, updated_at
		FROM event_topics
		WHERE name = $1
	`, topicName))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrEventTopicNotFound, topicName)
	}
	if err != nil {
		return nil, fmt.Errorf("get event topic by name: %w", err)
	}
	return topic, nil
}

func (s *PostgresStore) ListEventTopics(ctx context.Context, limit int) ([]*EventTopic, error) {
	limit = normalizeEventListLimit(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, retention_hours, created_at, updated_at
		FROM event_topics
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list event topics: %w", err)
	}
	defer rows.Close()

	out := make([]*EventTopic, 0, limit)
	for rows.Next() {
		topic, err := scanEventTopic(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event topic: %w", err)
		}
		out = append(out, topic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event topics rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) DeleteEventTopicByName(ctx context.Context, name string) error {
	topicName := strings.TrimSpace(name)
	ct, err := s.pool.Exec(ctx, `DELETE FROM event_topics WHERE name = $1`, topicName)
	if err != nil {
		return fmt.Errorf("delete event topic: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrEventTopicNotFound, topicName)
	}
	return nil
}

func (s *PostgresStore) CreateEventSubscription(ctx context.Context, sub *EventSubscription) error {
	if sub == nil {
		return fmt.Errorf("event subscription is required")
	}
	if err := normalizeEventSubscription(sub); err != nil {
		return err
	}

	if sub.TopicName == "" {
		if err := s.pool.QueryRow(ctx, `SELECT name FROM event_topics WHERE id = $1`, sub.TopicID).Scan(&sub.TopicName); err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("%w: %s", ErrEventTopicNotFound, sub.TopicID)
			}
			return fmt.Errorf("lookup topic for subscription: %w", err)
		}
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO event_subscriptions (
			id, topic_id, name, consumer_group, function_id, function_name,
			enabled, max_attempts, backoff_base_ms, backoff_max_ms,
			max_inflight, rate_limit_per_sec, last_acked_sequence, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14, $15
		)
	`, sub.ID, sub.TopicID, sub.Name, sub.ConsumerGroup, sub.FunctionID, sub.FunctionName,
		sub.Enabled, sub.MaxAttempts, sub.BackoffBaseMS, sub.BackoffMaxMS,
		sub.MaxInflight, sub.RateLimitPerSec, sub.LastAckedSeq, sub.CreatedAt, sub.UpdatedAt)
	if err != nil {
		if isPGUniqueViolation(err) {
			return fmt.Errorf("event subscription name or consumer_group already exists on topic: %s", sub.TopicName)
		}
		return fmt.Errorf("create event subscription: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetEventSubscription(ctx context.Context, id string) (*EventSubscription, error) {
	sub, err := scanEventSubscription(s.pool.QueryRow(ctx, `
		SELECT s.id, s.topic_id, t.name, s.name, s.consumer_group, s.function_id, s.function_name,
		       s.enabled, s.max_attempts, s.backoff_base_ms, s.backoff_max_ms,
		       s.max_inflight, s.rate_limit_per_sec, s.last_dispatch_at, s.last_acked_sequence, s.last_acked_at,
		       COALESCE(stats.inflight_count, 0), COALESCE(stats.queued_count, 0), COALESCE(stats.dlq_count, 0),
		       GREATEST(COALESCE(stats.latest_sequence, s.last_acked_sequence) - s.last_acked_sequence, 0), stats.oldest_unacked_at,
		       s.created_at, s.updated_at
		FROM event_subscriptions s
		JOIN event_topics t ON t.id = s.topic_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE d.status = 'running') AS inflight_count,
				COUNT(*) FILTER (WHERE d.status = 'queued') AS queued_count,
				COUNT(*) FILTER (WHERE d.status = 'dlq') AS dlq_count,
				MAX(m.sequence) AS latest_sequence,
				MIN(d.created_at) FILTER (WHERE d.status IN ('queued', 'running', 'dlq')) AS oldest_unacked_at
			FROM event_deliveries d
			JOIN event_messages m ON m.id = d.message_id
			WHERE d.subscription_id = s.id
		) stats ON TRUE
		WHERE s.id = $1
	`, id))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrEventSubscriptionNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get event subscription: %w", err)
	}
	return sub, nil
}

func (s *PostgresStore) ListEventSubscriptions(ctx context.Context, topicID string) ([]*EventSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.id, s.topic_id, t.name, s.name, s.consumer_group, s.function_id, s.function_name,
		       s.enabled, s.max_attempts, s.backoff_base_ms, s.backoff_max_ms,
		       s.max_inflight, s.rate_limit_per_sec, s.last_dispatch_at, s.last_acked_sequence, s.last_acked_at,
		       COALESCE(stats.inflight_count, 0), COALESCE(stats.queued_count, 0), COALESCE(stats.dlq_count, 0),
		       GREATEST(COALESCE(stats.latest_sequence, s.last_acked_sequence) - s.last_acked_sequence, 0), stats.oldest_unacked_at,
		       s.created_at, s.updated_at
		FROM event_subscriptions s
		JOIN event_topics t ON t.id = s.topic_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE d.status = 'running') AS inflight_count,
				COUNT(*) FILTER (WHERE d.status = 'queued') AS queued_count,
				COUNT(*) FILTER (WHERE d.status = 'dlq') AS dlq_count,
				MAX(m.sequence) AS latest_sequence,
				MIN(d.created_at) FILTER (WHERE d.status IN ('queued', 'running', 'dlq')) AS oldest_unacked_at
			FROM event_deliveries d
			JOIN event_messages m ON m.id = d.message_id
			WHERE d.subscription_id = s.id
		) stats ON TRUE
		WHERE s.topic_id = $1
		ORDER BY s.created_at ASC
	`, strings.TrimSpace(topicID))
	if err != nil {
		return nil, fmt.Errorf("list event subscriptions: %w", err)
	}
	defer rows.Close()

	out := make([]*EventSubscription, 0)
	for rows.Next() {
		sub, err := scanEventSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event subscription: %w", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event subscriptions rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) UpdateEventSubscription(ctx context.Context, id string, update *EventSubscriptionUpdate) (*EventSubscription, error) {
	sub, err := s.GetEventSubscription(ctx, id)
	if err != nil {
		return nil, err
	}
	if update == nil {
		return sub, nil
	}

	if update.Name != nil {
		sub.Name = strings.TrimSpace(*update.Name)
	}
	if update.ConsumerGroup != nil {
		sub.ConsumerGroup = strings.TrimSpace(*update.ConsumerGroup)
	}
	if update.FunctionID != nil {
		sub.FunctionID = strings.TrimSpace(*update.FunctionID)
	}
	if update.FunctionName != nil {
		sub.FunctionName = strings.TrimSpace(*update.FunctionName)
	}
	if update.Enabled != nil {
		sub.Enabled = *update.Enabled
	}
	if update.MaxAttempts != nil {
		sub.MaxAttempts = *update.MaxAttempts
	}
	if update.BackoffBaseMS != nil {
		sub.BackoffBaseMS = *update.BackoffBaseMS
	}
	if update.BackoffMaxMS != nil {
		sub.BackoffMaxMS = *update.BackoffMaxMS
	}
	if update.MaxInflight != nil {
		sub.MaxInflight = *update.MaxInflight
	}
	if update.RateLimitPerS != nil {
		sub.RateLimitPerSec = *update.RateLimitPerS
	}
	if err := normalizeEventSubscription(sub); err != nil {
		return nil, err
	}
	if sub.TopicName == "" {
		if err := s.pool.QueryRow(ctx, `SELECT name FROM event_topics WHERE id = $1`, sub.TopicID).Scan(&sub.TopicName); err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("%w: %s", ErrEventTopicNotFound, sub.TopicID)
			}
			return nil, fmt.Errorf("lookup topic for subscription: %w", err)
		}
	}
	sub.UpdatedAt = time.Now().UTC()

	ct, err := s.pool.Exec(ctx, `
		UPDATE event_subscriptions SET
			name = $2,
			consumer_group = $3,
			function_id = $4,
			function_name = $5,
			enabled = $6,
			max_attempts = $7,
			backoff_base_ms = $8,
			backoff_max_ms = $9,
			max_inflight = $10,
			rate_limit_per_sec = $11,
			updated_at = $12
		WHERE id = $1
	`, sub.ID, sub.Name, sub.ConsumerGroup, sub.FunctionID, sub.FunctionName, sub.Enabled,
		sub.MaxAttempts, sub.BackoffBaseMS, sub.BackoffMaxMS,
		sub.MaxInflight, sub.RateLimitPerSec, sub.UpdatedAt)
	if err != nil {
		if isPGUniqueViolation(err) {
			return nil, fmt.Errorf("event subscription name or consumer_group already exists on topic: %s", sub.TopicName)
		}
		return nil, fmt.Errorf("update event subscription: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEventSubscriptionNotFound, id)
	}
	return sub, nil
}

func (s *PostgresStore) DeleteEventSubscription(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM event_subscriptions WHERE id = $1`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete event subscription: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrEventSubscriptionNotFound, id)
	}
	return nil
}

func (s *PostgresStore) PublishEvent(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*EventMessage, int, error) {
	topicID = strings.TrimSpace(topicID)
	if topicID == "" {
		return nil, 0, fmt.Errorf("topic id is required")
	}
	ok, err := normalizeOrderingKey(orderingKey)
	if err != nil {
		return nil, 0, err
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if len(headers) == 0 {
		headers = json.RawMessage(`{}`)
	}

	now := time.Now().UTC()
	msg := &EventMessage{
		ID:          uuid.New().String(),
		TopicID:     topicID,
		OrderingKey: ok,
		Payload:     payload,
		Headers:     headers,
		PublishedAt: now,
		CreatedAt:   now,
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx, `SELECT name FROM event_topics WHERE id = $1`, topicID).Scan(&msg.TopicName); err != nil {
		if err == pgx.ErrNoRows {
			return nil, 0, fmt.Errorf("%w: %s", ErrEventTopicNotFound, topicID)
		}
		return nil, 0, fmt.Errorf("lookup event topic: %w", err)
	}

	if err := tx.QueryRow(ctx, `
		INSERT INTO event_messages (id, topic_id, ordering_key, payload, headers, published_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING sequence
	`, msg.ID, msg.TopicID, msg.OrderingKey, msg.Payload, msg.Headers, msg.PublishedAt, msg.CreatedAt).Scan(&msg.Sequence); err != nil {
		return nil, 0, fmt.Errorf("insert event message: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, function_id, function_name, max_attempts, backoff_base_ms, backoff_max_ms
		FROM event_subscriptions
		WHERE topic_id = $1 AND enabled = TRUE
		ORDER BY created_at ASC
	`, topicID)
	if err != nil {
		return nil, 0, fmt.Errorf("list event subscriptions for publish: %w", err)
	}
	type subTarget struct {
		ID            string
		FunctionID    string
		FunctionName  string
		MaxAttempts   int
		BackoffBaseMS int
		BackoffMaxMS  int
	}
	targets := make([]subTarget, 0)
	for rows.Next() {
		var target subTarget
		if err := rows.Scan(&target.ID, &target.FunctionID, &target.FunctionName, &target.MaxAttempts, &target.BackoffBaseMS, &target.BackoffMaxMS); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("scan subscription for publish: %w", err)
		}
		target.MaxAttempts = normalizeEventMaxAttempts(target.MaxAttempts)
		target.BackoffBaseMS, target.BackoffMaxMS = normalizeEventBackoff(target.BackoffBaseMS, target.BackoffMaxMS)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, fmt.Errorf("iterate subscriptions for publish: %w", err)
	}
	rows.Close()

	deliveries := 0
	for _, target := range targets {
		deliveryID := uuid.New().String()
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_deliveries (
				id, topic_id, subscription_id, message_id, function_id, function_name,
				ordering_key, status, attempt, max_attempts, backoff_base_ms, backoff_max_ms,
				next_run_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, 'queued', 0, $8, $9, $10,
				$11, $11, $11
			)
		`, deliveryID, topicID, target.ID, msg.ID, target.FunctionID, target.FunctionName,
			msg.OrderingKey, target.MaxAttempts, target.BackoffBaseMS, target.BackoffMaxMS, now); err != nil {
			return nil, 0, fmt.Errorf("insert event delivery: %w", err)
		}
		deliveries++
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("commit publish event: %w", err)
	}
	return msg, deliveries, nil
}

func (s *PostgresStore) ListEventMessages(ctx context.Context, topicID string, limit int) ([]*EventMessage, error) {
	limit = normalizeEventListLimit(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.topic_id, t.name, m.sequence, m.ordering_key, m.payload, m.headers, m.published_at, m.created_at
		FROM event_messages m
		JOIN event_topics t ON t.id = m.topic_id
		WHERE m.topic_id = $1
		ORDER BY m.sequence DESC
		LIMIT $2
	`, strings.TrimSpace(topicID), limit)
	if err != nil {
		return nil, fmt.Errorf("list event messages: %w", err)
	}
	defer rows.Close()

	out := make([]*EventMessage, 0, limit)
	for rows.Next() {
		msg, err := scanEventMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event message: %w", err)
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event messages rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) GetEventDelivery(ctx context.Context, id string) (*EventDelivery, error) {
	delivery, err := scanEventDelivery(s.pool.QueryRow(ctx, `
		SELECT d.id, d.topic_id, t.name, d.subscription_id, s.name, s.consumer_group,
		       d.message_id, m.sequence, d.ordering_key, m.payload, m.headers,
		       d.status, d.attempt, d.max_attempts, d.backoff_base_ms, d.backoff_max_ms,
		       d.next_run_at, d.locked_by, d.locked_until, d.function_id, d.function_name,
		       d.request_id, d.output, d.duration_ms, d.cold_start, d.last_error,
		       d.started_at, d.completed_at, d.created_at, d.updated_at
		FROM event_deliveries d
		JOIN event_topics t ON t.id = d.topic_id
		JOIN event_subscriptions s ON s.id = d.subscription_id
		JOIN event_messages m ON m.id = d.message_id
		WHERE d.id = $1
	`, strings.TrimSpace(id)))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrEventDeliveryNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get event delivery: %w", err)
	}
	return delivery, nil
}

func (s *PostgresStore) ListEventDeliveries(ctx context.Context, subscriptionID string, limit int, statuses []EventDeliveryStatus) ([]*EventDelivery, error) {
	limit = normalizeEventListLimit(limit)
	query := `
		SELECT d.id, d.topic_id, t.name, d.subscription_id, s.name, s.consumer_group,
		       d.message_id, m.sequence, d.ordering_key, m.payload, m.headers,
		       d.status, d.attempt, d.max_attempts, d.backoff_base_ms, d.backoff_max_ms,
		       d.next_run_at, d.locked_by, d.locked_until, d.function_id, d.function_name,
		       d.request_id, d.output, d.duration_ms, d.cold_start, d.last_error,
		       d.started_at, d.completed_at, d.created_at, d.updated_at
		FROM event_deliveries d
		JOIN event_topics t ON t.id = d.topic_id
		JOIN event_subscriptions s ON s.id = d.subscription_id
		JOIN event_messages m ON m.id = d.message_id
		WHERE d.subscription_id = $1
	`
	args := []any{strings.TrimSpace(subscriptionID)}

	if len(statuses) > 0 {
		args = append(args, eventStatusesToStrings(statuses))
		query += " AND d.status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	args = append(args, limit)
	query += " ORDER BY d.created_at DESC LIMIT $" + strconv.Itoa(len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list event deliveries: %w", err)
	}
	defer rows.Close()

	out := make([]*EventDelivery, 0, limit)
	for rows.Next() {
		delivery, err := scanEventDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event delivery: %w", err)
		}
		out = append(out, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event deliveries rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) AcquireDueEventDelivery(ctx context.Context, workerID string, leaseDuration time.Duration) (*EventDelivery, error) {
	if workerID == "" {
		workerID = "event-worker"
	}
	if leaseDuration <= 0 {
		leaseDuration = DefaultEventLeaseTimeout
	}

	now := time.Now().UTC()
	leaseUntil := now.Add(leaseDuration)

	delivery, err := scanEventDelivery(s.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT d.id, d.subscription_id
			FROM event_deliveries d
			JOIN event_subscriptions s ON s.id = d.subscription_id
			WHERE s.enabled = TRUE
			  AND (
				(d.status = 'queued' AND d.next_run_at <= $3)
				OR (d.status = 'running' AND d.locked_until < $3)
			  )
			  AND (
			  	s.max_inflight <= 0
			  	OR (
			  		SELECT COUNT(*) FROM event_deliveries infl
			  		WHERE infl.subscription_id = d.subscription_id
			  		  AND infl.status = 'running'
			  		  AND infl.id <> d.id
			  		  AND infl.locked_until >= $3
			  	) < s.max_inflight
			  )
			  AND (
			  	s.rate_limit_per_sec <= 0
			  	OR s.last_dispatch_at IS NULL
			  	OR s.last_dispatch_at <= $3 - ((1000.0 / GREATEST(s.rate_limit_per_sec, 1)) * INTERVAL '1 millisecond')
			  )
			  AND (
				d.ordering_key = ''
				OR NOT EXISTS (
					SELECT 1
					FROM event_deliveries prev
					WHERE prev.subscription_id = d.subscription_id
					  AND prev.ordering_key = d.ordering_key
					  AND prev.id <> d.id
					  AND prev.created_at < d.created_at
					  AND prev.status IN ('queued', 'running')
				)
			  )
			ORDER BY d.next_run_at ASC, d.created_at ASC
			FOR UPDATE OF d, s SKIP LOCKED
			LIMIT 1
		), updated AS (
			UPDATE event_deliveries d
			SET status = 'running',
				attempt = d.attempt + 1,
				locked_by = $1,
				locked_until = $2,
				started_at = COALESCE(d.started_at, $3),
				updated_at = $3
			FROM candidate c
			WHERE d.id = c.id
			RETURNING d.id
		), touched_sub AS (
			UPDATE event_subscriptions s
			SET last_dispatch_at = $3
			FROM candidate c
			WHERE s.id = c.subscription_id
			RETURNING s.id
		)
		SELECT d.id, d.topic_id, t.name, d.subscription_id, s.name, s.consumer_group,
		       d.message_id, m.sequence, d.ordering_key, m.payload, m.headers,
		       d.status, d.attempt, d.max_attempts, d.backoff_base_ms, d.backoff_max_ms,
		       d.next_run_at, d.locked_by, d.locked_until, d.function_id, d.function_name,
		       d.request_id, d.output, d.duration_ms, d.cold_start, d.last_error,
		       d.started_at, d.completed_at, d.created_at, d.updated_at
		FROM event_deliveries d
		JOIN updated u ON u.id = d.id
		JOIN event_topics t ON t.id = d.topic_id
		JOIN event_subscriptions s ON s.id = d.subscription_id
		JOIN event_messages m ON m.id = d.message_id
	`, workerID, leaseUntil, now))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("acquire event delivery: %w", err)
	}
	return delivery, nil
}

func (s *PostgresStore) MarkEventDeliverySucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	now := time.Now().UTC()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var subscriptionID string
	var sequence int64
	err = tx.QueryRow(ctx, `
		WITH updated AS (
			UPDATE event_deliveries SET
				status = 'succeeded',
				request_id = $2,
				output = $3,
				duration_ms = $4,
				cold_start = $5,
				last_error = NULL,
				locked_by = NULL,
				locked_until = NULL,
				completed_at = $6,
				updated_at = $6
			WHERE id = $1
			RETURNING subscription_id, message_id
		)
		SELECT updated.subscription_id, m.sequence
		FROM updated
		JOIN event_messages m ON m.id = updated.message_id
	`, id, nullIfEmpty(requestID), output, durationMS, coldStart, now).Scan(&subscriptionID, &sequence)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: %s", ErrEventDeliveryNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("mark event delivery succeeded: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE event_subscriptions
		SET last_acked_sequence = GREATEST(last_acked_sequence, $2),
		    last_acked_at = $3
		WHERE id = $1
	`, subscriptionID, sequence, now); err != nil {
		return fmt.Errorf("update event subscription cursor: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit event success tx: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkEventDeliveryForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	if nextRunAt.IsZero() {
		nextRunAt = time.Now().UTC()
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE event_deliveries SET
			status = 'queued',
			last_error = $2,
			next_run_at = $3,
			locked_by = NULL,
			locked_until = NULL,
			updated_at = NOW()
		WHERE id = $1
	`, id, nullIfEmpty(lastError), nextRunAt)
	if err != nil {
		return fmt.Errorf("mark event delivery retry: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrEventDeliveryNotFound, id)
	}
	return nil
}

func (s *PostgresStore) MarkEventDeliveryDLQ(ctx context.Context, id, lastError string) error {
	now := time.Now().UTC()
	ct, err := s.pool.Exec(ctx, `
		UPDATE event_deliveries SET
			status = 'dlq',
			last_error = $2,
			locked_by = NULL,
			locked_until = NULL,
			completed_at = $3,
			updated_at = $3
		WHERE id = $1
	`, id, nullIfEmpty(lastError), now)
	if err != nil {
		return fmt.Errorf("mark event delivery dlq: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrEventDeliveryNotFound, id)
	}
	return nil
}

func (s *PostgresStore) RequeueEventDelivery(ctx context.Context, id string, maxAttempts int) (*EventDelivery, error) {
	now := time.Now().UTC()
	maxAttempts = normalizeEventMaxAttempts(maxAttempts)

	ct, err := s.pool.Exec(ctx, `
		UPDATE event_deliveries SET
			status = 'queued',
			attempt = 0,
			max_attempts = $2,
			next_run_at = $3,
			locked_by = NULL,
			locked_until = NULL,
			request_id = NULL,
			output = NULL,
			duration_ms = 0,
			cold_start = FALSE,
			last_error = NULL,
			started_at = NULL,
			completed_at = NULL,
			updated_at = $3
		WHERE id = $1 AND status = 'dlq'
	`, id, maxAttempts, now)
	if err != nil {
		return nil, fmt.Errorf("requeue event delivery: %w", err)
	}
	if ct.RowsAffected() == 0 {
		var status string
		statusErr := s.pool.QueryRow(ctx, `SELECT status FROM event_deliveries WHERE id = $1`, id).Scan(&status)
		if statusErr == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrEventDeliveryNotFound, id)
		}
		if statusErr != nil {
			return nil, fmt.Errorf("requeue event delivery lookup: %w", statusErr)
		}
		return nil, fmt.Errorf("%w: %s (%s)", ErrEventDeliveryNotDLQ, id, status)
	}

	return s.GetEventDelivery(ctx, id)
}

func (s *PostgresStore) ResolveEventReplaySequenceByTime(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
	if from.IsZero() {
		return 1, nil
	}
	var topicID string
	if err := s.pool.QueryRow(ctx, `
		SELECT topic_id FROM event_subscriptions WHERE id = $1
	`, strings.TrimSpace(subscriptionID)).Scan(&topicID); err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("%w: %s", ErrEventSubscriptionNotFound, subscriptionID)
		}
		return 0, fmt.Errorf("resolve replay topic: %w", err)
	}

	var sequence int64
	err := s.pool.QueryRow(ctx, `
		SELECT sequence
		FROM event_messages
		WHERE topic_id = $1 AND published_at >= $2
		ORDER BY sequence ASC
		LIMIT 1
	`, topicID, from.UTC()).Scan(&sequence)
	if err == nil {
		return sequence, nil
	}
	if err != pgx.ErrNoRows {
		return 0, fmt.Errorf("resolve replay sequence by time: %w", err)
	}

	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(sequence) + 1, 1)
		FROM event_messages
		WHERE topic_id = $1
	`, topicID).Scan(&sequence)
	if err != nil {
		return 0, fmt.Errorf("resolve replay sequence fallback: %w", err)
	}
	return sequence, nil
}

func (s *PostgresStore) SetEventSubscriptionCursor(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*EventSubscription, error) {
	if lastAckedSequence < 0 {
		return nil, fmt.Errorf("last_acked_sequence must be >= 0")
	}
	now := time.Now().UTC()
	ct, err := s.pool.Exec(ctx, `
		UPDATE event_subscriptions
		SET last_acked_sequence = $2,
		    last_acked_at = $3,
		    updated_at = $3
		WHERE id = $1
	`, strings.TrimSpace(subscriptionID), lastAckedSequence, now)
	if err != nil {
		return nil, fmt.Errorf("set event subscription cursor: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEventSubscriptionNotFound, subscriptionID)
	}
	return s.GetEventSubscription(ctx, subscriptionID)
}

func (s *PostgresStore) ReplayEventSubscription(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
	if fromSequence <= 0 {
		fromSequence = 1
	}
	limit = normalizeEventReplayLimit(limit)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	type subCfg struct {
		TopicID       string
		FunctionID    string
		FunctionName  string
		MaxAttempts   int
		BackoffBaseMS int
		BackoffMaxMS  int
	}
	var cfg subCfg
	err = tx.QueryRow(ctx, `
		SELECT topic_id, function_id, function_name, max_attempts, backoff_base_ms, backoff_max_ms
		FROM event_subscriptions
		WHERE id = $1
	`, strings.TrimSpace(subscriptionID)).Scan(
		&cfg.TopicID,
		&cfg.FunctionID,
		&cfg.FunctionName,
		&cfg.MaxAttempts,
		&cfg.BackoffBaseMS,
		&cfg.BackoffMaxMS,
	)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("%w: %s", ErrEventSubscriptionNotFound, subscriptionID)
	}
	if err != nil {
		return 0, fmt.Errorf("lookup subscription for replay: %w", err)
	}
	cfg.MaxAttempts = normalizeEventMaxAttempts(cfg.MaxAttempts)
	cfg.BackoffBaseMS, cfg.BackoffMaxMS = normalizeEventBackoff(cfg.BackoffBaseMS, cfg.BackoffMaxMS)

	rows, err := tx.Query(ctx, `
		SELECT id, ordering_key
		FROM event_messages
		WHERE topic_id = $1 AND sequence >= $2
		ORDER BY sequence ASC
		LIMIT $3
	`, cfg.TopicID, fromSequence, limit)
	if err != nil {
		return 0, fmt.Errorf("list messages for replay: %w", err)
	}

	type replayMessage struct {
		ID          string
		OrderingKey string
	}
	messages := make([]replayMessage, 0)
	for rows.Next() {
		var msg replayMessage
		if err := rows.Scan(&msg.ID, &msg.OrderingKey); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan replay message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate replay messages: %w", err)
	}
	rows.Close()

	now := time.Now().UTC()
	count := 0
	for _, msg := range messages {
		deliveryID := uuid.New().String()
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_deliveries (
				id, topic_id, subscription_id, message_id, function_id, function_name,
				ordering_key, status, attempt, max_attempts, backoff_base_ms, backoff_max_ms,
				next_run_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, 'queued', 0, $8, $9, $10,
				$11, $11, $11
			)
		`, deliveryID, cfg.TopicID, subscriptionID, msg.ID, cfg.FunctionID, cfg.FunctionName,
			msg.OrderingKey, cfg.MaxAttempts, cfg.BackoffBaseMS, cfg.BackoffMaxMS, now); err != nil {
			return 0, fmt.Errorf("insert replay delivery: %w", err)
		}
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit replay: %w", err)
	}
	return count, nil
}

func normalizeEventTopic(topic *EventTopic) error {
	now := time.Now().UTC()
	if topic.ID == "" {
		topic.ID = uuid.New().String()
	}
	topic.Name = strings.TrimSpace(topic.Name)
	topic.Description = strings.TrimSpace(topic.Description)
	if err := validateEventName(topic.Name, ErrInvalidEventTopicName); err != nil {
		return err
	}
	if topic.RetentionHours <= 0 {
		topic.RetentionHours = DefaultEventRetentionHours
	}
	if topic.RetentionHours > 24*365 {
		topic.RetentionHours = 24 * 365
	}
	if topic.CreatedAt.IsZero() {
		topic.CreatedAt = now
	}
	topic.UpdatedAt = now
	return nil
}

func normalizeEventSubscription(sub *EventSubscription) error {
	now := time.Now().UTC()
	if sub.ID == "" {
		sub.ID = uuid.New().String()
	}
	sub.TopicID = strings.TrimSpace(sub.TopicID)
	sub.Name = strings.TrimSpace(sub.Name)
	sub.ConsumerGroup = strings.TrimSpace(sub.ConsumerGroup)
	sub.FunctionID = strings.TrimSpace(sub.FunctionID)
	sub.FunctionName = strings.TrimSpace(sub.FunctionName)

	if sub.TopicID == "" {
		return fmt.Errorf("topic id is required")
	}
	if err := validateEventName(sub.Name, ErrInvalidEventSubName); err != nil {
		return err
	}
	if sub.ConsumerGroup == "" {
		sub.ConsumerGroup = sub.Name
	}
	if err := validateEventName(sub.ConsumerGroup, ErrInvalidConsumerGroup); err != nil {
		return err
	}
	if sub.FunctionID == "" || sub.FunctionName == "" {
		return fmt.Errorf("function id and function name are required")
	}
	if sub.MaxAttempts <= 0 {
		sub.MaxAttempts = DefaultEventMaxAttempts
	}
	sub.BackoffBaseMS, sub.BackoffMaxMS = normalizeEventBackoff(sub.BackoffBaseMS, sub.BackoffMaxMS)
	if sub.MaxInflight < 0 {
		return fmt.Errorf("max_inflight must be >= 0")
	}
	if sub.MaxInflight > 100000 {
		sub.MaxInflight = 100000
	}
	if sub.RateLimitPerSec < 0 {
		return fmt.Errorf("rate_limit_per_sec must be >= 0")
	}
	if sub.RateLimitPerSec > 10000 {
		sub.RateLimitPerSec = 10000
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now
	return nil
}

func validateEventName(value string, baseErr error) error {
	if value == "" {
		return fmt.Errorf("%w: value is required", baseErr)
	}
	if !eventNamePattern.MatchString(value) {
		return fmt.Errorf("%w: only [A-Za-z0-9._-], max length 128", baseErr)
	}
	return nil
}

func normalizeOrderingKey(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if len(trimmed) > 256 {
		return "", fmt.Errorf("%w: max length is 256", ErrInvalidOrderingKey)
	}
	return trimmed, nil
}

func normalizeEventMaxAttempts(maxAttempts int) int {
	if maxAttempts <= 0 {
		return DefaultEventMaxAttempts
	}
	if maxAttempts > 50 {
		return 50
	}
	return maxAttempts
}

func normalizeEventBackoff(baseMS, maxMS int) (int, int) {
	if baseMS <= 0 {
		baseMS = DefaultEventBackoffBaseMS
	}
	if maxMS <= 0 {
		maxMS = DefaultEventBackoffMaxMS
	}
	if maxMS < baseMS {
		maxMS = baseMS
	}
	if maxMS > 24*60*60*1000 {
		maxMS = 24 * 60 * 60 * 1000
	}
	return baseMS, maxMS
}

func normalizeEventListLimit(limit int) int {
	if limit <= 0 {
		return DefaultEventListLimit
	}
	if limit > MaxEventListLimit {
		return MaxEventListLimit
	}
	return limit
}

func normalizeEventReplayLimit(limit int) int {
	if limit <= 0 {
		return DefaultEventReplayLimit
	}
	if limit > MaxEventReplayLimit {
		return MaxEventReplayLimit
	}
	return limit
}

func eventStatusesToStrings(statuses []EventDeliveryStatus) []string {
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		if status == "" {
			continue
		}
		out = append(out, string(status))
	}
	return out
}

func isPGUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type eventTopicScanner interface {
	Scan(dest ...any) error
}

func scanEventTopic(scanner eventTopicScanner) (*EventTopic, error) {
	var topic EventTopic
	if err := scanner.Scan(
		&topic.ID,
		&topic.Name,
		&topic.Description,
		&topic.RetentionHours,
		&topic.CreatedAt,
		&topic.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &topic, nil
}

type eventSubscriptionScanner interface {
	Scan(dest ...any) error
}

func scanEventSubscription(scanner eventSubscriptionScanner) (*EventSubscription, error) {
	var sub EventSubscription
	var oldestUnackedAt *time.Time
	if err := scanner.Scan(
		&sub.ID,
		&sub.TopicID,
		&sub.TopicName,
		&sub.Name,
		&sub.ConsumerGroup,
		&sub.FunctionID,
		&sub.FunctionName,
		&sub.Enabled,
		&sub.MaxAttempts,
		&sub.BackoffBaseMS,
		&sub.BackoffMaxMS,
		&sub.MaxInflight,
		&sub.RateLimitPerSec,
		&sub.LastDispatchAt,
		&sub.LastAckedSeq,
		&sub.LastAckedAt,
		&sub.Inflight,
		&sub.Queued,
		&sub.DLQ,
		&sub.Lag,
		&oldestUnackedAt,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if oldestUnackedAt != nil {
		age := time.Since(*oldestUnackedAt).Seconds()
		if age > 0 {
			sub.OldestUnackedS = int64(age)
		}
	}
	return &sub, nil
}

type eventMessageScanner interface {
	Scan(dest ...any) error
}

func scanEventMessage(scanner eventMessageScanner) (*EventMessage, error) {
	var msg EventMessage
	var payload []byte
	var headers []byte
	if err := scanner.Scan(
		&msg.ID,
		&msg.TopicID,
		&msg.TopicName,
		&msg.Sequence,
		&msg.OrderingKey,
		&payload,
		&headers,
		&msg.PublishedAt,
		&msg.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		msg.Payload = payload
	} else {
		msg.Payload = json.RawMessage(`{}`)
	}
	if len(headers) > 0 {
		msg.Headers = headers
	}
	return &msg, nil
}

type eventDeliveryScanner interface {
	Scan(dest ...any) error
}

func scanEventDelivery(scanner eventDeliveryScanner) (*EventDelivery, error) {
	var delivery EventDelivery
	var status string
	var payload []byte
	var headers []byte
	var output []byte
	var lockedBy *string
	var requestID *string
	var lastError *string

	if err := scanner.Scan(
		&delivery.ID,
		&delivery.TopicID,
		&delivery.TopicName,
		&delivery.SubscriptionID,
		&delivery.SubscriptionName,
		&delivery.ConsumerGroup,
		&delivery.MessageID,
		&delivery.MessageSequence,
		&delivery.OrderingKey,
		&payload,
		&headers,
		&status,
		&delivery.Attempt,
		&delivery.MaxAttempts,
		&delivery.BackoffBaseMS,
		&delivery.BackoffMaxMS,
		&delivery.NextRunAt,
		&lockedBy,
		&delivery.LockedUntil,
		&delivery.FunctionID,
		&delivery.FunctionName,
		&requestID,
		&output,
		&delivery.DurationMS,
		&delivery.ColdStart,
		&lastError,
		&delivery.StartedAt,
		&delivery.CompletedAt,
		&delivery.CreatedAt,
		&delivery.UpdatedAt,
	); err != nil {
		return nil, err
	}

	delivery.Status = EventDeliveryStatus(status)
	if len(payload) > 0 {
		delivery.Payload = payload
	} else {
		delivery.Payload = json.RawMessage(`{}`)
	}
	if len(headers) > 0 {
		delivery.Headers = headers
	}
	if len(output) > 0 {
		delivery.Output = output
	}
	if lockedBy != nil {
		delivery.LockedBy = *lockedBy
	}
	if requestID != nil {
		delivery.RequestID = *requestID
	}
	if lastError != nil {
		delivery.LastError = *lastError
	}
	return &delivery, nil
}
