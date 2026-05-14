package http

import (
	"log/slog"
	"net/http"
	"time"
)

// RequestLogger logs each HTTP request with method, path, and duration.
// Uses InfoContext so the trace-aware slog handler (P2.2c) enriches the log
// line with trace_id and span_id.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
