package logging

import (
	"log/slog"
	"os"
)

// InitStructured reconfigures the operational logger based on format settings.
// format: "text" (default) or "json" (Loki/ELK compatible)
// level: "debug", "info", "warn", "error"
func InitStructured(format, level string) {
	SetLevelFromString(level)

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	logger := slog.New(handler)
	opLogger.Store(logger)
}

// OpWithTrace returns the operational logger with trace context fields.
// traceID and spanID are injected as attributes when available.
func OpWithTrace(traceID, spanID string) *slog.Logger {
	l := opLogger.Load()
	if traceID == "" {
		return l
	}
	args := []any{"trace_id", traceID}
	if spanID != "" {
		args = append(args, "span_id", spanID)
	}
	return l.With(args...)
}
