package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
)

// PhaseArchivedHandler is the HTTP handler interface for the worker pipeline.
// The concrete implementation is *consolidation.Handler; this interface
// allows tests to pass a simpler stub without wiring the full pipeline.
type PhaseArchivedHandler interface {
	Handle(ctx context.Context, payload consolidation.PhaseArchivedReceived) error
}

// WorkerHandler exposes the inbound worker endpoints:
//   - POST /api/v1/worker/phase-archived
//
// All routes require the API-key middleware (wired in server.go).
// The handler dispatches pipeline work asynchronously so the HTTP response
// is always 202 Accepted before the pipeline completes (per D-M2-1 and the
// locked decision N.5: webhook receiver lives in the main HTTP server).
type WorkerHandler struct {
	pipeline PhaseArchivedHandler
}

// NewWorkerHandler constructs a WorkerHandler.
func NewWorkerHandler(pipeline PhaseArchivedHandler) *WorkerHandler {
	return &WorkerHandler{pipeline: pipeline}
}

// PhaseArchived handles POST /api/v1/worker/phase-archived.
//
// On success it returns 202 Accepted immediately and dispatches the
// consolidation pipeline in a background goroutine.
// The endpoint is idempotent at the pipeline level (digest/{change_id}
// guard prevents double-processing).
func (h *WorkerHandler) PhaseArchived(w http.ResponseWriter, r *http.Request) {
	var payload consolidation.PhaseArchivedReceived
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
			"code":  "invalid_body",
		})
		return
	}

	// Return 202 before dispatching — request thread must not block on pipeline.
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})

	// Dispatch asynchronously. Use a detached background context derived from
	// the request context values (trace, auth) but not cancelled when the
	// HTTP response completes. The pipeline is responsible for its own logging.
	go func() {
		ctx := context.Background()
		if err := h.pipeline.Handle(ctx, payload); err != nil {
			slog.WarnContext(ctx, "worker: pipeline error",
				slog.String("change_id", payload.ChangeID),
				slog.String("error", err.Error()),
			)
		}
	}()
}
