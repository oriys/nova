package replay

import (
	"context"
	"encoding/json"
	"time"
)

// ReplayRequest represents a request to replay a specific invocation.
type ReplayRequest struct {
	InvocationID string            `json:"invocation_id"`
	FunctionID   string            `json:"function_id"`
	RecordingID  string            `json:"recording_id"`
	OverrideEnv  map[string]string `json:"override_env,omitempty"` // Optional env overrides
	DryRun       bool              `json:"dry_run"`                // If true, validate but don't execute
}

// ReplayResponse contains the result of a replay execution.
type ReplayResponse struct {
	ReplayID       string          `json:"replay_id"`
	Status         string          `json:"status"` // "success", "diverged", "failed"
	OriginalOutput json.RawMessage `json:"original_output,omitempty"`
	ReplayOutput   json.RawMessage `json:"replay_output,omitempty"`
	Divergences    []Divergence    `json:"divergences,omitempty"`
	Duration       time.Duration   `json:"duration_ms"`
	EventsReplayed int             `json:"events_replayed"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    time.Time       `json:"completed_at"`
}

// ReplayService orchestrates replay operations.
type ReplayService struct {
	engine *Engine
	store  *TieredStore
}

// NewReplayService creates a new replay service.
func NewReplayService(engine *Engine, store *TieredStore) *ReplayService {
	return &ReplayService{
		engine: engine,
		store:  store,
	}
}

// StartReplay initiates a replay of a recorded invocation.
func (rs *ReplayService) StartReplay(ctx context.Context, req *ReplayRequest) (*ReplayResponse, error) {
	// Load recording from store
	recording, err := rs.store.Get(req.RecordingID)
	if err != nil {
		return nil, err
	}

	// Apply env overrides
	if len(req.OverrideEnv) > 0 {
		for k, v := range req.OverrideEnv {
			recording.EnvVars[k] = v
		}
	}

	if req.DryRun {
		return &ReplayResponse{
			ReplayID:       "dry-run",
			Status:         "validated",
			EventsReplayed: len(recording.Events),
		}, nil
	}

	// Execute replay
	start := time.Now()
	result := rs.engine.Execute(recording)

	resp := &ReplayResponse{
		ReplayID:       result.ReplayID,
		StartedAt:      start,
		CompletedAt:    time.Now(),
		Duration:       time.Since(start),
		EventsReplayed: result.EventsReplayed,
	}

	if result.Output != nil {
		resp.ReplayOutput = result.Output
	}
	if result.OriginalOutput != nil {
		resp.OriginalOutput = result.OriginalOutput
	}

	if len(result.Divergences) > 0 {
		resp.Status = "diverged"
		resp.Divergences = result.Divergences
	} else if result.Error != "" {
		resp.Status = "failed"
	} else {
		resp.Status = "success"
	}

	return resp, nil
}

// ListRecordings returns available recordings for a function.
func (rs *ReplayService) ListRecordings(functionID string, limit int) ([]*Recording, error) {
	return rs.store.ListByFunction(functionID, limit)
}
