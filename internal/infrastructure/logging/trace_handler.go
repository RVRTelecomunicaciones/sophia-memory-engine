package logging

import (
	"context"
	"log/slog"

	"github.com/sophia-engine/memory-engine/internal/domain/trace"
)

// TraceHandler wraps a slog.Handler so that every log record emitted with a
// context carrying a trace.Trace gains "trace_id" and "span_id" attributes
// (ADR-0005 P2.2c). This is the leaf-service counterpart of the orchestator's
// trace-enrichment handler — same key names, same hex format, so a single
// `grep` across all service logs yields the full correlation chain.
//
// Use slog.InfoContext(ctx, ...) / WarnContext(...) / ErrorContext(...) so the
// request context (and therefore the trace) reaches the handler. Plain
// slog.Info(...) calls will still work, just without trace enrichment.
type TraceHandler struct {
	inner slog.Handler
}

// NewTraceHandler returns a TraceHandler that delegates to inner.
func NewTraceHandler(inner slog.Handler) *TraceHandler {
	return &TraceHandler{inner: inner}
}

// Enabled forwards to the inner handler unchanged.
func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle reads the trace from ctx (if any) and appends trace_id + span_id
// attributes before delegating to the inner handler.
func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	if t, ok := trace.FromContext(ctx); ok {
		r.AddAttrs(
			slog.String("trace_id", t.TraceID),
			slog.String("span_id", t.SpanID),
		)
	}
	return h.inner.Handle(ctx, r) //nolint:wrapcheck // pass-through wrapper
}

// WithAttrs delegates to the inner handler, keeping the wrapper intact.
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup delegates to the inner handler, keeping the wrapper intact.
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{inner: h.inner.WithGroup(name)}
}
