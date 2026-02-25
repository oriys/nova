// Package replay implements deterministic invocation replay for debugging.
// It records non-deterministic inputs (time, random, network, filesystem)
// during execution and allows replaying invocations with identical conditions.
package replay

import (
	"encoding/json"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// FormatVersion is the current recording format version.
const FormatVersion = 1

// EventType identifies the kind of non-deterministic event.
type EventType string

const (
	EventTime     EventType = "time"      // gettimeofday/clock_gettime
	EventRandom   EventType = "random"    // /dev/urandom reads
	EventFileRead EventType = "file_read" // File reads outside code dir
	EventNetRecv  EventType = "net_recv"  // Network receive
	EventNetSend  EventType = "net_send"  // Network send
	EventEnvVar   EventType = "env_var"   // Environment variable read
)

// NonDeterministicEvent records a single non-deterministic event.
type NonDeterministicEvent struct {
	Type      EventType       `json:"type"`
	Seq       int64           `json:"seq"`
	Timestamp time.Time       `json:"timestamp"`
	Value     json.RawMessage `json:"value"`

	// Type-specific fields
	Path    string `json:"path,omitempty"`    // For file_read
	Syscall string `json:"syscall,omitempty"` // Which syscall triggered this
	Size    int    `json:"size,omitempty"`    // Bytes involved
}

// Recording captures everything needed to deterministically replay an invocation.
type Recording struct {
	Version        int                     `json:"version"`
	FunctionID     string                  `json:"function_id"`
	FunctionName   string                  `json:"function_name"`
	RequestID      string                  `json:"request_id"`
	CodeHash       string                  `json:"code_hash"`
	Runtime        domain.Runtime          `json:"runtime"`
	RuntimeVersion string                  `json:"runtime_version"`
	Arch           domain.Arch             `json:"arch"`
	Handler        string                  `json:"handler"`
	EnvVars        map[string]string       `json:"env_vars"`
	InputPayload   json.RawMessage         `json:"input_payload"`
	Events         []NonDeterministicEvent `json:"nondeterministic_events"`
	OutputPayload  json.RawMessage         `json:"output_payload,omitempty"`
	OutputError    string                  `json:"output_error,omitempty"`
	DurationMs     int64                   `json:"duration_ms"`
	RecordedAt     time.Time               `json:"recorded_at"`
}

// Recorder captures non-deterministic events during execution.
type Recorder struct {
	recording *Recording
	seq       int64
	enabled   bool
}

// NewRecorder creates a new invocation recorder.
func NewRecorder(fn *domain.Function, requestID string, payload json.RawMessage) *Recorder {
	return &Recorder{
		recording: &Recording{
			Version:      FormatVersion,
			FunctionID:   fn.ID,
			FunctionName: fn.Name,
			RequestID:    requestID,
			CodeHash:     fn.CodeHash,
			Runtime:      fn.Runtime,
			Arch:         fn.Arch,
			Handler:      fn.Handler,
			EnvVars:      fn.EnvVars,
			InputPayload: payload,
			RecordedAt:   time.Now(),
		},
		enabled: true,
	}
}

// RecordEvent adds a non-deterministic event to the recording.
func (r *Recorder) RecordEvent(eventType EventType, value interface{}) {
	if !r.enabled {
		return
	}
	data, _ := json.Marshal(value)
	r.recording.Events = append(r.recording.Events, NonDeterministicEvent{
		Type:      eventType,
		Seq:       r.seq,
		Timestamp: time.Now(),
		Value:     data,
	})
	r.seq++
}

// SetOutput records the invocation output.
func (r *Recorder) SetOutput(output json.RawMessage, errMsg string, durationMs int64) {
	r.recording.OutputPayload = output
	r.recording.OutputError = errMsg
	r.recording.DurationMs = durationMs
}

// Finish returns the completed recording.
func (r *Recorder) Finish() *Recording {
	return r.recording
}

// Marshal serializes a recording to JSON.
func (rec *Recording) Marshal() ([]byte, error) {
	return json.Marshal(rec)
}

// UnmarshalRecording deserializes a recording from JSON.
func UnmarshalRecording(data []byte) (*Recording, error) {
	var rec Recording
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}
