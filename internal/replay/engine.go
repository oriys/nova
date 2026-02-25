package replay

import (
	"encoding/json"
	"fmt"
	"time"
)

// ReplayResult describes the outcome of a replay.
type ReplayResult struct {
	RequestID        string          `json:"request_id"`
	OriginalOutput   json.RawMessage `json:"original_output"`
	ReplayOutput     json.RawMessage `json:"replay_output"`
	OriginalError    string          `json:"original_error"`
	ReplayError      string          `json:"replay_error"`
	Match            bool            `json:"match"`
	Divergence       *Divergence     `json:"divergence,omitempty"`
	OriginalDuration int64           `json:"original_duration_ms"`
	ReplayDuration   int64           `json:"replay_duration_ms"`
	ReplayedAt       time.Time       `json:"replayed_at"`
}

// Divergence describes where a replay diverged from the original.
type Divergence struct {
	EventSeq    int64  `json:"event_seq"`
	EventType   string `json:"event_type"`
	Expected    string `json:"expected"`
	Actual      string `json:"actual"`
	Description string `json:"description"`
}

// Engine orchestrates deterministic replay of recorded invocations.
type Engine struct {
	store *Store
}

// NewEngine creates a new replay engine.
func NewEngine(store *Store) *Engine {
	return &Engine{store: store}
}

// Replay re-executes a recorded invocation.
// In a full implementation, this would:
// 1. Load the recording
// 2. Start a VM with the same runtime/rootfs/code
// 3. Configure the agent in replay mode
// 4. Feed recorded non-deterministic values
// 5. Compare output
func (e *Engine) Replay(requestID string) (*ReplayResult, error) {
	rec, err := e.store.Load(requestID)
	if err != nil {
		return nil, fmt.Errorf("load recording: %w", err)
	}

	result := &ReplayResult{
		RequestID:        requestID,
		OriginalOutput:   rec.OutputPayload,
		OriginalError:    rec.OutputError,
		OriginalDuration: rec.DurationMs,
		ReplayedAt:       time.Now(),
	}

	// The actual replay would invoke the function via the executor
	// with the recorded non-deterministic events injected.
	// For now, return the recording info.
	result.ReplayOutput = rec.OutputPayload
	result.ReplayError = rec.OutputError
	result.Match = true

	return result, nil
}

// CompareOutputs checks whether two outputs are semantically equivalent.
func CompareOutputs(original, replay json.RawMessage) (bool, *Divergence) {
	if string(original) == string(replay) {
		return true, nil
	}

	// Try JSON-level comparison (ignoring whitespace)
	var o1, o2 interface{}
	if json.Unmarshal(original, &o1) == nil && json.Unmarshal(replay, &o2) == nil {
		b1, _ := json.Marshal(o1)
		b2, _ := json.Marshal(o2)
		if string(b1) == string(b2) {
			return true, nil
		}
	}

	return false, &Divergence{
		Description: "output mismatch",
		Expected:    truncate(string(original), 200),
		Actual:      truncate(string(replay), 200),
	}
}

// ExecuteResult holds the outcome of an Engine.Execute call.
type ExecuteResult struct {
	ReplayID       string          `json:"replay_id"`
	Output         json.RawMessage `json:"output,omitempty"`
	OriginalOutput json.RawMessage `json:"original_output,omitempty"`
	Divergences    []Divergence    `json:"divergences,omitempty"`
	EventsReplayed int             `json:"events_replayed"`
	Error          string          `json:"error,omitempty"`
}

// Execute replays a recording and returns the result.
func (e *Engine) Execute(rec *Recording) *ExecuteResult {
	result := &ExecuteResult{
		ReplayID:       rec.RequestID,
		OriginalOutput: rec.OutputPayload,
		EventsReplayed: len(rec.Events),
	}

	// Simulated replay: return original output.
	// A full implementation would start a VM in replay mode,
	// feed recorded non-deterministic values, and compare output.
	result.Output = rec.OutputPayload

	if rec.OutputError != "" {
		result.Error = rec.OutputError
	}

	match, div := CompareOutputs(result.OriginalOutput, result.Output)
	if !match && div != nil {
		result.Divergences = append(result.Divergences, *div)
	}

	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
