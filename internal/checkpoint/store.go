package checkpoint

import (
	"encoding/json"
	"sync"
	"time"
)

// State represents a checkpointed execution state for a function invocation.
type State struct {
	RequestID  string          `json:"request_id"`
	FunctionID string          `json:"function_id"`
	Step       string          `json:"step"`        // Current execution step identifier
	Data       json.RawMessage `json:"data"`        // Serialized intermediate state
	CreatedAt  time.Time       `json:"created_at"`
	ExpiresAt  time.Time       `json:"expires_at"`
}

// Store provides in-memory checkpoint storage for function execution state.
// This enables workflows and long-running invocations to resume from
// intermediate steps after failures.
type Store struct {
	mu     sync.RWMutex
	states map[string]*State // request ID -> checkpoint
	ttl    time.Duration
}

// NewStore creates a new checkpoint store.
func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	s := &Store{
		states: make(map[string]*State),
		ttl:    ttl,
	}
	go s.cleanupLoop()
	return s
}

// Save stores a checkpoint for a given request.
func (s *Store) Save(requestID, functionID, step string, data json.RawMessage) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.states[requestID] = &State{
		RequestID:  requestID,
		FunctionID: functionID,
		Step:       step,
		Data:       data,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.ttl),
	}
}

// Load retrieves the checkpoint for a request, or nil if none exists.
func (s *Store) Load(requestID string) *State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[requestID]
	if !ok {
		return nil
	}
	if time.Now().After(state.ExpiresAt) {
		return nil
	}
	// Return a copy
	cp := *state
	return &cp
}

// Delete removes the checkpoint for a request.
func (s *Store) Delete(requestID string) {
	s.mu.Lock()
	delete(s.states, requestID)
	s.mu.Unlock()
}

// ListByFunction returns all checkpoints for a given function.
func (s *Store) ListByFunction(functionID string) []*State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var out []*State
	for _, state := range s.states {
		if state.FunctionID == functionID && now.Before(state.ExpiresAt) {
			cp := *state
			out = append(out, &cp)
		}
	}
	return out
}

// cleanupLoop periodically removes expired checkpoints.
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(s.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, state := range s.states {
			if now.After(state.ExpiresAt) {
				delete(s.states, id)
			}
		}
		s.mu.Unlock()
	}
}
