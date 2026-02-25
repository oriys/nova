package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StorageConfig controls recording retention.
type StorageConfig struct {
	BaseDir       string        // Root directory for recordings
	HotTTL        time.Duration // Hot tier retention (default: 24h)
	WarmTTL       time.Duration // Warm tier retention (default: 7d)
	SampleRate    float64       // Fraction of invocations to record (0.0-1.0)
	RecordOnError bool          // Always record on error regardless of sample rate
	MaxSizeBytes  int64         // Max total storage
}

// DefaultStorageConfig returns sensible defaults.
func DefaultStorageConfig(baseDir string) StorageConfig {
	return StorageConfig{
		BaseDir:       baseDir,
		HotTTL:        24 * time.Hour,
		WarmTTL:       7 * 24 * time.Hour,
		SampleRate:    0.01, // 1% of invocations
		RecordOnError: true,
		MaxSizeBytes:  5 * 1024 * 1024 * 1024, // 5 GB
	}
}

// RecordingMeta is lightweight metadata for listing recordings.
type RecordingMeta struct {
	RequestID    string    `json:"request_id"`
	FunctionID   string    `json:"function_id"`
	FunctionName string    `json:"function_name"`
	RecordedAt   time.Time `json:"recorded_at"`
	SizeBytes    int64     `json:"size_bytes"`
	HasError     bool      `json:"has_error"`
}

// Store manages recording persistence with tiered retention.
type Store struct {
	cfg    StorageConfig
	mu     sync.RWMutex
	meta   map[string]*RecordingMeta // requestID -> meta
	ctx    context.Context
	cancel context.CancelFunc
}

// NewStore creates a new recording store.
func NewStore(cfg StorageConfig) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{
		cfg:    cfg,
		meta:   make(map[string]*RecordingMeta),
		ctx:    ctx,
		cancel: cancel,
	}
	os.MkdirAll(filepath.Join(cfg.BaseDir, "hot"), 0755)
	os.MkdirAll(filepath.Join(cfg.BaseDir, "warm"), 0755)
	s.loadMeta()
	go s.gcLoop()
	return s
}

// Save persists a recording to the hot tier.
func (s *Store) Save(rec *Recording) error {
	data, err := rec.Marshal()
	if err != nil {
		return fmt.Errorf("marshal recording: %w", err)
	}

	path := filepath.Join(s.cfg.BaseDir, "hot", rec.RequestID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write recording: %w", err)
	}

	meta := &RecordingMeta{
		RequestID:    rec.RequestID,
		FunctionID:   rec.FunctionID,
		FunctionName: rec.FunctionName,
		RecordedAt:   rec.RecordedAt,
		SizeBytes:    int64(len(data)),
		HasError:     rec.OutputError != "",
	}
	s.mu.Lock()
	s.meta[rec.RequestID] = meta
	s.mu.Unlock()

	return nil
}

// Load retrieves a recording by request ID.
func (s *Store) Load(requestID string) (*Recording, error) {
	// Check hot tier
	path := filepath.Join(s.cfg.BaseDir, "hot", requestID+".json")
	data, err := os.ReadFile(path)
	if err == nil {
		return UnmarshalRecording(data)
	}

	// Check warm tier
	path = filepath.Join(s.cfg.BaseDir, "warm", requestID+".json")
	data, err = os.ReadFile(path)
	if err == nil {
		return UnmarshalRecording(data)
	}

	return nil, fmt.Errorf("recording not found: %s", requestID)
}

// List returns metadata for all recordings of a function.
func (s *Store) List(functionID string) []*RecordingMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*RecordingMeta
	for _, m := range s.meta {
		if m.FunctionID == functionID {
			result = append(result, m)
		}
	}
	return result
}

// Stop shuts down the store.
func (s *Store) Stop() {
	s.cancel()
}

func (s *Store) loadMeta() {
	for _, tier := range []string{"hot", "warm"} {
		dir := filepath.Join(s.cfg.BaseDir, tier)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".json" {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			requestID := e.Name()[:len(e.Name())-5]
			s.meta[requestID] = &RecordingMeta{
				RequestID:  requestID,
				RecordedAt: info.ModTime(),
				SizeBytes:  info.Size(),
			}
		}
	}
}

func (s *Store) gcLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.gc()
		}
	}
}

// TieredStore is an alias for Store, used by the replay API layer.
type TieredStore = Store

// Get retrieves a recording by ID.
func (s *Store) Get(id string) (*Recording, error) {
	return s.Load(id)
}

// ListByFunction returns recordings for a function, limited to limit entries.
func (s *Store) ListByFunction(functionID string, limit int) ([]*Recording, error) {
	metas := s.List(functionID)
	if limit > 0 && len(metas) > limit {
		metas = metas[:limit]
	}
	var recordings []*Recording
	for _, m := range metas {
		rec, err := s.Load(m.RequestID)
		if err != nil {
			continue
		}
		recordings = append(recordings, rec)
	}
	return recordings, nil
}

func (s *Store) gc() {
	now := time.Now()

	// Move hot → warm
	hotDir := filepath.Join(s.cfg.BaseDir, "hot")
	warmDir := filepath.Join(s.cfg.BaseDir, "warm")
	entries, _ := os.ReadDir(hotDir)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > s.cfg.HotTTL {
			src := filepath.Join(hotDir, e.Name())
			dst := filepath.Join(warmDir, e.Name())
			os.Rename(src, dst)
		}
	}

	// Purge warm tier
	entries, _ = os.ReadDir(warmDir)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > s.cfg.WarmTTL {
			os.Remove(filepath.Join(warmDir, e.Name()))
			s.mu.Lock()
			requestID := e.Name()[:len(e.Name())-5]
			delete(s.meta, requestID)
			s.mu.Unlock()
		}
	}
}
