package logging

import (
	"io"
	"log/slog"
	"os"
)

// NewLogger creates a structured JSON logger with the given level.
// If writer is nil, os.Stdout is used.
func NewLogger(level slog.Level, writer io.Writer) *slog.Logger {
	if writer == nil {
		writer = os.Stdout
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler)
}

// DefaultLogger returns a logger with INFO level writing to stdout.
func DefaultLogger() *slog.Logger {
	return NewLogger(slog.LevelInfo, nil)
}
