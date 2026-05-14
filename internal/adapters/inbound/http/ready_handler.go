package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// readyPingTimeout caps how long the /ready endpoint will wait on the DB.
// ADR-0005 requires < 100ms happy-path response time; 2s is a hard ceiling
// to bound failure-mode latency before returning 503.
const readyPingTimeout = 2 * time.Second

// DBPinger abstracts the bit of the connection pool that /ready exercises.
// The real implementation is *pgxpool.Pool (its Ping method satisfies this
// interface), but the indirection lets us mock the pool in unit tests
// without standing up a real Postgres.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// ReadyHandler probes downstream dependencies (currently just Postgres)
// and reports readiness for Kubernetes-style probes.
type ReadyHandler struct {
	db DBPinger
}

// NewReadyHandler constructs a ReadyHandler bound to the given pinger.
func NewReadyHandler(db DBPinger) *ReadyHandler {
	return &ReadyHandler{db: db}
}

// Ready handles GET /ready. It returns 200 when every dependency check
// passes and 503 otherwise. The response body always reports per-check
// state under "checks" so operators can see which dependency degraded.
func (h *ReadyHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyPingTimeout)
	defer cancel()

	checks := map[string]string{}
	status := http.StatusOK
	overall := "ready"

	if err := h.db.Ping(ctx); err != nil {
		checks["db"] = err.Error()
		status = http.StatusServiceUnavailable
		overall = "degraded"
		// Log with InfoContext so the trace_id from P2.2c reaches the log.
		slog.WarnContext(r.Context(), "ready: db ping failed",
			slog.String("error", err.Error()),
		)
	} else {
		checks["db"] = "ok"
	}

	slog.InfoContext(r.Context(), "ready: probe complete",
		slog.String("status", overall),
	)

	writeJSON(w, status, map[string]any{
		"status": overall,
		"checks": checks,
	})
}
