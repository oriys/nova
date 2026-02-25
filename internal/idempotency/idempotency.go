// Package idempotency provides exactly-once execution semantics for function
// invocations via idempotency keys and deduplication windows.
//
// The package supports two storage backends:
//   - In-memory (for development and single-node deployments)
//   - Postgres (for production multi-node deployments)
//
// Flow:
//  1. Client provides an idempotency key (or one is auto-generated)
//  2. Before execution: Check(key) → hit: return cached result, miss: claim key
//  3. After execution: Complete(key, result) → store result with TTL
//  4. On failure: Release(key) → allow retry
package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Status represents the state of an idempotency key.
type Status string

const (
	StatusClaimed   Status = "claimed"   // Key claimed, execution in progress
	StatusCompleted Status = "completed" // Execution completed, result cached
	StatusFailed    Status = "failed"    // Execution failed, retryable
)

// Entry represents a cached idempotency result.
type Entry struct {
	Key         string          `json:"key"`
	Status      Status          `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	ClaimedBy   string          `json:"claimed_by,omitempty"` // Worker ID
	ClaimedAt   time.Time       `json:"claimed_at"`
	CompletedAt time.Time       `json:"completed_at,omitempty"`
	ExpiresAt   time.Time       `json:"expires_at"`
}

// CheckResult describes the outcome of an idempotency check.
type CheckResult struct {
	Hit    bool            // True if key was already known
	Status Status          // Current status of the key
	Result json.RawMessage // Cached result (only if Status == completed)
	Error  string          // Cached error (only if Status == failed)
}

// Config holds idempotency cache configuration.
type Config struct {
	DefaultTTL      time.Duration // How long to keep completed results (default: 24h)
	ClaimTimeout    time.Duration // Max time a claim can be held before expiry (default: 5min)
	CleanupInterval time.Duration // How often to purge expired entries (default: 1min)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultTTL:      24 * time.Hour,
		ClaimTimeout:    5 * time.Minute,
		CleanupInterval: time.Minute,
	}
}

// Store is the idempotency cache.
type Store struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewStore creates a new in-memory idempotency store.
func NewStore(cfg Config) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{
		entries: make(map[string]*Entry),
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
	}
	go s.cleanupLoop()
	return s
}

// Check looks up an idempotency key. If the key is new, it claims it.
// If the key exists and is completed, it returns the cached result.
// If the key exists and is claimed (in-progress), it returns that status.
func (s *Store) Check(ctx context.Context, key string, workerID string) CheckResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.entries[key]; ok {
		// Key exists
		switch entry.Status {
		case StatusCompleted:
			return CheckResult{Hit: true, Status: StatusCompleted, Result: entry.Result}
		case StatusFailed:
			return CheckResult{Hit: true, Status: StatusFailed, Error: entry.Error}
		case StatusClaimed:
			// Check if claim has expired
			if time.Now().After(entry.ClaimedAt.Add(s.cfg.ClaimTimeout)) {
				// Expired claim, reclaim
				entry.ClaimedBy = workerID
				entry.ClaimedAt = time.Now()
				return CheckResult{Hit: false}
			}
			return CheckResult{Hit: true, Status: StatusClaimed}
		}
	}

	// New key - claim it
	s.entries[key] = &Entry{
		Key:       key,
		Status:    StatusClaimed,
		ClaimedBy: workerID,
		ClaimedAt: time.Now(),
		ExpiresAt: time.Now().Add(s.cfg.DefaultTTL),
	}
	return CheckResult{Hit: false}
}

// Complete marks an idempotency key as completed with the given result.
func (s *Store) Complete(key string, result json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[key]
	if !ok {
		return fmt.Errorf("idempotency key not found: %s", key)
	}

	entry.Status = StatusCompleted
	entry.Result = result
	entry.CompletedAt = time.Now()
	entry.ExpiresAt = time.Now().Add(s.cfg.DefaultTTL)
	return nil
}

// Fail marks an idempotency key as failed (retryable).
func (s *Store) Fail(key string, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[key]
	if !ok {
		return fmt.Errorf("idempotency key not found: %s", key)
	}

	entry.Status = StatusFailed
	entry.Error = errMsg
	entry.CompletedAt = time.Now()
	entry.ExpiresAt = time.Now().Add(s.cfg.DefaultTTL)
	return nil
}

// Release removes a claim, allowing retry with the same key.
func (s *Store) Release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

// Stop shuts down the cleanup goroutine.
func (s *Store) Stop() {
	s.cancel()
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for key, entry := range s.entries {
		if now.After(entry.ExpiresAt) {
			delete(s.entries, key)
		}
	}
}
