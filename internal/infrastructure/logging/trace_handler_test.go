package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/trace"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/logging"
	"github.com/stretchr/testify/require"
)

// Test: log line emitted inside a request-scoped context carries trace_id and
// span_id attributes. This is the integration point that ties P2.2c together
// — without it, the trace context exists but never reaches the log stream.
func TestTraceHandler_AddsTraceAttrs(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(logging.NewTraceHandler(inner))

	tr := trace.Trace{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Sampled: true,
	}
	ctx := trace.NewContext(context.Background(), tr)

	logger.InfoContext(ctx, "ingest memory", slog.String("id", "abc"))

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", rec["trace_id"])
	require.Equal(t, "00f067aa0ba902b7", rec["span_id"])
	require.Equal(t, "abc", rec["id"])
	require.Equal(t, "ingest memory", rec["msg"])
}

// Test: log line emitted without a trace-bearing context does NOT include
// trace_id / span_id (they are optional, not synthesized).
func TestTraceHandler_NoTraceInContext_NoAttrs(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(logging.NewTraceHandler(inner))

	logger.InfoContext(context.Background(), "plain log")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	_, hasTrace := rec["trace_id"]
	_, hasSpan := rec["span_id"]
	require.False(t, hasTrace, "trace_id must not be emitted when ctx has no trace")
	require.False(t, hasSpan, "span_id must not be emitted when ctx has no trace")
}

// Test: WithAttrs and WithGroup preserve the wrapper so derived loggers still
// enrich with trace_id.
func TestTraceHandler_WithAttrs_PreservesWrapper(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(logging.NewTraceHandler(inner)).With(slog.String("svc", "memory-engine"))

	tr := trace.Trace{
		TraceID: "aaaabbbbccccddddaaaabbbbccccdddd",
		SpanID:  "1122334455667788",
		Sampled: true,
	}
	ctx := trace.NewContext(context.Background(), tr)

	logger.InfoContext(ctx, "after-with-attrs")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "aaaabbbbccccddddaaaabbbbccccdddd", rec["trace_id"])
	require.Equal(t, "memory-engine", rec["svc"])
}
