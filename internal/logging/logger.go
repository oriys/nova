package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// RequestLog represents a single invocation log entry
type RequestLog struct {
	Timestamp   time.Time       `json:"timestamp"`
	RequestID   string          `json:"request_id"`
	TraceID     string          `json:"trace_id,omitempty"`
	SpanID      string          `json:"span_id,omitempty"`
	Function    string          `json:"function"`
	FunctionID  string          `json:"function_id"`
	Runtime     string          `json:"runtime,omitempty"`
	DurationMs  int64           `json:"duration_ms"`
	ColdStart   bool            `json:"cold_start"`
	Success     bool            `json:"success"`
	Error       string          `json:"error,omitempty"`
	InputSize   int             `json:"input_size"`
	OutputSize  int             `json:"output_size,omitempty"`
	Retries     int             `json:"retries,omitempty"`
	FromCache   bool            `json:"from_cache,omitempty"`
}

// Logger handles request logging
type Logger struct {
	mu      sync.Mutex
	enabled bool
	file    *os.File
	console bool
}

var defaultLogger = &Logger{enabled: true, console: true}

// Default returns the default logger
func Default() *Logger {
	return defaultLogger
}

// SetOutput sets the log output file
func (l *Logger) SetOutput(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Close()
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	l.file = f
	return nil
}

// SetConsole enables/disables console output
func (l *Logger) SetConsole(enabled bool) {
	l.mu.Lock()
	l.console = enabled
	l.mu.Unlock()
}

// Log writes a request log entry
func (l *Logger) Log(entry *RequestLog) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.enabled {
		return
	}

	entry.Timestamp = time.Now()

	// Console output (human-readable)
	if l.console {
		status := "✓"
		if !entry.Success {
			status = "✗"
		}
		cold := ""
		if entry.ColdStart {
			cold = " [cold]"
		}
		cache := ""
		if entry.FromCache {
			cache = " [cached]"
		}
		retry := ""
		if entry.Retries > 0 {
			retry = fmt.Sprintf(" [retry:%d]", entry.Retries)
		}
		fmt.Printf("[request] %s %s %s %dms%s%s%s\n",
			status, entry.RequestID, entry.Function, entry.DurationMs, cold, cache, retry)
		if entry.Error != "" {
			fmt.Printf("[request]   error: %s\n", entry.Error)
		}
	}

	// File output (JSON)
	if l.file != nil {
		data, _ := json.Marshal(entry)
		l.file.Write(append(data, '\n'))
	}
}

// Close closes the log file
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}
