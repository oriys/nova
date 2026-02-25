package replay

import (
	"fmt"
	"sync"
	"time"
)

// TimeTravelRecorder records statement-level execution for interpreted runtimes.
type TimeTravelRecorder struct {
	mu     sync.Mutex
	steps  []ExecutionStep
	active bool
}

// ExecutionStep represents a single step in execution.
type ExecutionStep struct {
	Seq       int               `json:"seq"`
	Timestamp time.Time         `json:"timestamp"`
	File      string            `json:"file"`
	Line      int               `json:"line"`
	Function  string            `json:"function"`
	Event     string            `json:"event"` // "call", "line", "return", "exception"
	Variables map[string]string `json:"variables,omitempty"`
	Output    string            `json:"output,omitempty"`
}

// NewTimeTravelRecorder creates a new recorder.
func NewTimeTravelRecorder() *TimeTravelRecorder {
	return &TimeTravelRecorder{
		active: true,
	}
}

// RecordStep records a single execution step.
func (ttr *TimeTravelRecorder) RecordStep(step ExecutionStep) {
	ttr.mu.Lock()
	defer ttr.mu.Unlock()
	if !ttr.active {
		return
	}
	step.Seq = len(ttr.steps)
	step.Timestamp = time.Now()
	ttr.steps = append(ttr.steps, step)
}

// Steps returns all recorded steps.
func (ttr *TimeTravelRecorder) Steps() []ExecutionStep {
	ttr.mu.Lock()
	defer ttr.mu.Unlock()
	result := make([]ExecutionStep, len(ttr.steps))
	copy(result, ttr.steps)
	return result
}

// StepCount returns the number of recorded steps.
func (ttr *TimeTravelRecorder) StepCount() int {
	ttr.mu.Lock()
	defer ttr.mu.Unlock()
	return len(ttr.steps)
}

// Stop stops recording.
func (ttr *TimeTravelRecorder) Stop() {
	ttr.mu.Lock()
	defer ttr.mu.Unlock()
	ttr.active = false
}

// TimeTravelDebugger allows stepping through a recorded execution.
type TimeTravelDebugger struct {
	steps       []ExecutionStep
	currentStep int
	breakpoints map[int]bool
}

// NewTimeTravelDebugger creates a debugger from recorded steps.
func NewTimeTravelDebugger(steps []ExecutionStep) *TimeTravelDebugger {
	return &TimeTravelDebugger{
		steps:       steps,
		currentStep: -1,
		breakpoints: make(map[int]bool),
	}
}

// SetBreakpoint sets a breakpoint at a step number.
func (ttd *TimeTravelDebugger) SetBreakpoint(step int) {
	ttd.breakpoints[step] = true
}

// RemoveBreakpoint removes a breakpoint.
func (ttd *TimeTravelDebugger) RemoveBreakpoint(step int) {
	delete(ttd.breakpoints, step)
}

// StepForward advances one step.
func (ttd *TimeTravelDebugger) StepForward() (*ExecutionStep, bool) {
	if ttd.currentStep+1 >= len(ttd.steps) {
		return nil, false
	}
	ttd.currentStep++
	step := ttd.steps[ttd.currentStep]
	return &step, true
}

// StepBackward goes back one step.
func (ttd *TimeTravelDebugger) StepBackward() (*ExecutionStep, bool) {
	if ttd.currentStep <= 0 {
		return nil, false
	}
	ttd.currentStep--
	step := ttd.steps[ttd.currentStep]
	return &step, true
}

// StepTo jumps to a specific step.
func (ttd *TimeTravelDebugger) StepTo(target int) (*ExecutionStep, error) {
	if target < 0 || target >= len(ttd.steps) {
		return nil, fmt.Errorf("step %d out of range [0, %d)", target, len(ttd.steps))
	}
	ttd.currentStep = target
	return &ttd.steps[target], nil
}

// ContinueForward runs until a breakpoint or end.
func (ttd *TimeTravelDebugger) ContinueForward() (*ExecutionStep, bool) {
	for ttd.currentStep+1 < len(ttd.steps) {
		ttd.currentStep++
		if ttd.breakpoints[ttd.currentStep] {
			return &ttd.steps[ttd.currentStep], true
		}
	}
	if ttd.currentStep < len(ttd.steps) {
		return &ttd.steps[ttd.currentStep], true
	}
	return nil, false
}

// CurrentState returns the current execution state.
func (ttd *TimeTravelDebugger) CurrentState() *TimeTravelState {
	if ttd.currentStep < 0 || ttd.currentStep >= len(ttd.steps) {
		return &TimeTravelState{Step: -1, Completed: ttd.currentStep >= len(ttd.steps)}
	}

	step := ttd.steps[ttd.currentStep]

	// Build call stack from steps up to current
	var callStack []StackFrame
	for i := 0; i <= ttd.currentStep; i++ {
		s := ttd.steps[i]
		if s.Event == "call" {
			callStack = append(callStack, StackFrame{
				Function: s.Function,
				File:     s.File,
				Line:     s.Line,
			})
		} else if s.Event == "return" && len(callStack) > 0 {
			callStack = callStack[:len(callStack)-1]
		}
	}

	return &TimeTravelState{
		Step:      ttd.currentStep,
		Line:      step.Line,
		File:      step.File,
		Variables: step.Variables,
		CallStack: callStack,
		Output:    step.Output,
		Completed: ttd.currentStep >= len(ttd.steps)-1,
	}
}

// TimeTravelSession manages an interactive time-travel debugging session.
type TimeTravelSession struct {
	ID        string
	Debugger  *TimeTravelDebugger
	CreatedAt time.Time
}

// TimeTravelSessionManager manages active debug sessions.
type TimeTravelSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*TimeTravelSession
}

// NewTimeTravelSessionManager creates a session manager.
func NewTimeTravelSessionManager() *TimeTravelSessionManager {
	return &TimeTravelSessionManager{
		sessions: make(map[string]*TimeTravelSession),
	}
}

// CreateSession creates a new debug session.
func (m *TimeTravelSessionManager) CreateSession(id string, steps []ExecutionStep) *TimeTravelSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := &TimeTravelSession{
		ID:        id,
		Debugger:  NewTimeTravelDebugger(steps),
		CreatedAt: time.Now(),
	}
	m.sessions[id] = session
	return session
}

// GetSession retrieves a debug session.
func (m *TimeTravelSessionManager) GetSession(id string) (*TimeTravelSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// DeleteSession removes a debug session.
func (m *TimeTravelSessionManager) DeleteSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

// RuntimeTraceConfig provides trace configuration for different interpreted runtimes.
type RuntimeTraceConfig struct {
	Runtime   string `json:"runtime"`
	TraceCmd  string `json:"trace_cmd"`  // Command to enable tracing
	TraceArgs string `json:"trace_args"` // Arguments for the trace command
}

// GetRuntimeTraceConfigs returns trace configurations for supported runtimes.
func GetRuntimeTraceConfigs() []RuntimeTraceConfig {
	return []RuntimeTraceConfig{
		{Runtime: "python", TraceCmd: "python3", TraceArgs: "-m trace --trace"},
		{Runtime: "node", TraceCmd: "node", TraceArgs: "--inspect-brk"},
		{Runtime: "ruby", TraceCmd: "ruby", TraceArgs: "-r tracer"},
	}
}
