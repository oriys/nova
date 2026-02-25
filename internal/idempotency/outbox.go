package idempotency

import (
	"encoding/json"
	"sync"
	"time"
)

// OutboxEntry represents a pending outbox message that needs to be published.
type OutboxEntry struct {
	ID             string          `json:"id"`
	Topic          string          `json:"topic"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         string          `json:"status"` // "pending", "published", "failed"
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	NextRetryAt    time.Time       `json:"next_retry_at"`
	CreatedAt      time.Time       `json:"created_at"`
	PublishedAt    time.Time       `json:"published_at,omitempty"`
}

// AtomicOutbox wraps task creation and outbox insert into a single atomic operation.
// This ensures that either both the task and the outbox entry are created, or neither.
type AtomicOutbox struct {
	mu      sync.Mutex
	entries map[string]*OutboxEntry
	inbox   *Inbox // Consumer-side dedup
}

// NewAtomicOutbox creates a new atomic outbox with inbox integration.
func NewAtomicOutbox(inbox *Inbox) *AtomicOutbox {
	return &AtomicOutbox{
		entries: make(map[string]*OutboxEntry),
		inbox:   inbox,
	}
}

// SubmitAtomic creates a task and outbox entry atomically.
// The idempotency key is checked against the inbox to prevent duplicate processing.
func (ao *AtomicOutbox) SubmitAtomic(id, topic string, payload json.RawMessage, idempotencyKey string) (*OutboxEntry, error) {
	ao.mu.Lock()
	defer ao.mu.Unlock()

	// Check inbox for duplicate
	if ao.inbox != nil && !ao.inbox.TryInsert(idempotencyKey, id) {
		return nil, ErrDuplicate
	}

	entry := &OutboxEntry{
		ID:             id,
		Topic:          topic,
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
		Status:         "pending",
		MaxAttempts:    5,
		CreatedAt:      time.Now(),
	}
	ao.entries[id] = entry
	return entry, nil
}

// MarkPublished marks an outbox entry as successfully published.
func (ao *AtomicOutbox) MarkPublished(id string) {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	if entry, ok := ao.entries[id]; ok {
		entry.Status = "published"
		entry.PublishedAt = time.Now()
	}
}

// MarkFailed marks an outbox entry as failed with retry backoff.
func (ao *AtomicOutbox) MarkFailed(id string) {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	if entry, ok := ao.entries[id]; ok {
		entry.Attempts++
		if entry.Attempts >= entry.MaxAttempts {
			entry.Status = "failed"
		} else {
			backoff := time.Duration(1<<uint(entry.Attempts)) * time.Second
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
			entry.NextRetryAt = time.Now().Add(backoff)
		}
	}
}

// PendingEntries returns outbox entries ready for publishing.
func (ao *AtomicOutbox) PendingEntries() []*OutboxEntry {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	now := time.Now()
	var pending []*OutboxEntry
	for _, entry := range ao.entries {
		if entry.Status == "pending" && (entry.NextRetryAt.IsZero() || now.After(entry.NextRetryAt)) {
			pending = append(pending, entry)
		}
	}
	return pending
}

// ErrDuplicate indicates the idempotency key was already processed.
var ErrDuplicate = &DuplicateError{}

// DuplicateError represents a duplicate idempotency key.
type DuplicateError struct{}

func (e *DuplicateError) Error() string { return "duplicate idempotency key" }
