package idempotency

import (
	"sync"
	"time"
)

// Inbox implements consumer-side deduplication.
// Before processing an async task, insert its message ID into the inbox.
// If the insert succeeds (new), process. If it conflicts (duplicate), skip.
type Inbox struct {
	mu      sync.Mutex
	entries map[string]*InboxEntry
	ttl     time.Duration
}

// InboxEntry tracks a processed message.
type InboxEntry struct {
	MessageID   string    `json:"message_id"`
	FunctionID  string    `json:"function_id"`
	Status      string    `json:"status"` // "processing", "done", "failed"
	ProcessedAt time.Time `json:"processed_at"`
}

// NewInbox creates a new inbox with the given TTL for entries.
func NewInbox(ttl time.Duration) *Inbox {
	ib := &Inbox{
		entries: make(map[string]*InboxEntry),
		ttl:     ttl,
	}
	go ib.gcLoop()
	return ib
}

// TryInsert attempts to insert a message ID. Returns true if this is a new
// message (should be processed), false if duplicate (should be skipped).
func (ib *Inbox) TryInsert(messageID, functionID string) bool {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	if _, exists := ib.entries[messageID]; exists {
		return false // Duplicate
	}

	ib.entries[messageID] = &InboxEntry{
		MessageID:   messageID,
		FunctionID:  functionID,
		Status:      "processing",
		ProcessedAt: time.Now(),
	}
	return true // New message
}

// MarkDone marks a message as successfully processed.
func (ib *Inbox) MarkDone(messageID string) {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	if entry, ok := ib.entries[messageID]; ok {
		entry.Status = "done"
	}
}

// MarkFailed marks a message as failed.
func (ib *Inbox) MarkFailed(messageID string) {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	if entry, ok := ib.entries[messageID]; ok {
		entry.Status = "failed"
	}
}

// Lookup returns the inbox entry for a message ID, or nil if not found.
func (ib *Inbox) Lookup(messageID string) *InboxEntry {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	return ib.entries[messageID]
}

func (ib *Inbox) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ib.mu.Lock()
		now := time.Now()
		for id, entry := range ib.entries {
			if now.Sub(entry.ProcessedAt) > ib.ttl {
				delete(ib.entries, id)
			}
		}
		ib.mu.Unlock()
	}
}
