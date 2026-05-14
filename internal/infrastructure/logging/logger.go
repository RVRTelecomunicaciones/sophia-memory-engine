package logging

import (
	"io"
	"log/slog"
	"os"
)

// NewLogger creates a structured JSON logger with the given level.
// If writer is nil, os.Stdout is used.
//
// The returned logger is wrapped with TraceHandler so that any log line
// emitted with slog.InfoContext / WarnContext / ErrorContext within a request
// scope automatically carries "trace_id" and "span_id" attributes (ADR-0005
// P2.2c). Plain slog.Info(...) calls still work — they just won't be
// trace-enriched.
func NewLogger(level slog.Level, writer io.Writer) *slog.Logger {
	if writer == nil {
		writer = os.Stdout
	}

	base := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(NewTraceHandler(base))
}

// DefaultLogger returns a logger with INFO level writing to stdout.
func DefaultLogger() *slog.Logger {
	return NewLogger(slog.LevelInfo, nil)
}
