package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OutputEntry stores captured function output
type OutputEntry struct {
	RequestID  string    `json:"request_id"`
	FunctionID string    `json:"function_id"`
	Stdout     string    `json:"stdout,omitempty"`
	Stderr     string    `json:"stderr,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// OutputStore manages function output capture with TTL cleanup
type OutputStore struct {
	mu         sync.RWMutex
	storageDir string
	maxSize    int64
	retentionS int
	entries    map[string]*OutputEntry // requestID -> entry
}

var globalOutputStore *OutputStore

// InitOutputStore initializes the global output store
func InitOutputStore(storageDir string, maxSize int64, retentionS int) error {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return err
	}

	globalOutputStore = &OutputStore{
		storageDir: storageDir,
		maxSize:    maxSize,
		retentionS: retentionS,
		entries:    make(map[string]*OutputEntry),
	}

	// Start cleanup goroutine
	go globalOutputStore.cleanupLoop()

	return nil
}

// GetOutputStore returns the global output store
func GetOutputStore() *OutputStore {
	return globalOutputStore
}

// Store saves function output for a request
func (s *OutputStore) Store(requestID, functionID, stdout, stderr string) {
	if s == nil {
		return
	}

	// Truncate if over max size
	if s.maxSize > 0 {
		if int64(len(stdout)) > s.maxSize {
			stdout = stdout[:s.maxSize] + "...[truncated]"
		}
		if int64(len(stderr)) > s.maxSize {
			stderr = stderr[:s.maxSize] + "...[truncated]"
		}
	}

	entry := &OutputEntry{
		RequestID:  requestID,
		FunctionID: functionID,
		Stdout:     stdout,
		Stderr:     stderr,
		Timestamp:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Duration(s.retentionS) * time.Second),
	}

	s.mu.Lock()
	s.entries[requestID] = entry
	s.mu.Unlock()

	// Also persist to disk
	s.persistEntry(entry)
}

// Get retrieves output for a request
func (s *OutputStore) Get(requestID string) (*OutputEntry, bool) {
	if s == nil {
		return nil, false
	}

	s.mu.RLock()
	entry, ok := s.entries[requestID]
	s.mu.RUnlock()

	if ok {
		return entry, true
	}

	// Try loading from disk
	return s.loadEntry(requestID)
}

// GetByFunction retrieves the last N outputs for a function
func (s *OutputStore) GetByFunction(functionID string, limit int) []*OutputEntry {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*OutputEntry
	for _, entry := range s.entries {
		if entry.FunctionID == functionID && time.Now().Before(entry.ExpiresAt) {
			results = append(results, entry)
		}
	}

	// Sort by timestamp descending and limit
	// Simple bubble sort since we expect small lists
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Timestamp.After(results[i].Timestamp) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

func (s *OutputStore) persistEntry(entry *OutputEntry) {
	path := filepath.Join(s.storageDir, entry.RequestID+".json")
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

func (s *OutputStore) loadEntry(requestID string) (*OutputEntry, bool) {
	path := filepath.Join(s.storageDir, requestID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry OutputEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		os.Remove(path)
		return nil, false
	}

	// Cache in memory
	s.mu.Lock()
	s.entries[requestID] = &entry
	s.mu.Unlock()

	return &entry, true
}

func (s *OutputStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

func (s *OutputStore) cleanup() {
	now := time.Now()

	// Clean memory cache
	s.mu.Lock()
	for id, entry := range s.entries {
		if now.After(entry.ExpiresAt) {
			delete(s.entries, id)
		}
	}
	s.mu.Unlock()

	// Clean disk storage
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(s.storageDir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Remove files older than retention period
		if now.Sub(info.ModTime()) > time.Duration(s.retentionS)*time.Second {
			os.Remove(path)
		}
	}
}
