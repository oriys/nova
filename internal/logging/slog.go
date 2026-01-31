package logging

import (
	"log/slog"
	"os"
	"sync/atomic"
)

var (
	opLogger atomic.Pointer[slog.Logger]
	logLevel = new(slog.LevelVar)
)

func init() {
	logLevel.Set(slog.LevelInfo)
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})
	logger := slog.New(handler)
	opLogger.Store(logger)
}

// Op returns the operational logger for daemon/infrastructure logs.
// This is separate from the request Logger which logs individual invocations.
func Op() *slog.Logger {
	return opLogger.Load()
}

// SetLevel changes the log level for the operational logger.
// Valid levels: slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError
func SetLevel(level slog.Level) {
	logLevel.Set(level)
}

// SetLevelFromString sets the log level from a string.
// Valid values: "debug", "info", "warn", "error"
func SetLevelFromString(level string) {
	switch level {
	case "debug", "DEBUG":
		logLevel.Set(slog.LevelDebug)
	case "info", "INFO":
		logLevel.Set(slog.LevelInfo)
	case "warn", "WARN", "warning", "WARNING":
		logLevel.Set(slog.LevelWarn)
	case "error", "ERROR":
		logLevel.Set(slog.LevelError)
	}
}
