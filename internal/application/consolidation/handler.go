package consolidation

import (
	"context"
	"log/slog"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// Handler is the consolidation pipeline's entrypoint for archive completion.
// PRE-0 stub: log receipt and return nil. M2 will replace the body with the
// actual consolidation work (episodic → semantic promotion, heuristic
// emission, ProjectDNA update).
type Handler struct {
	log   *slog.Logger
	clock shared.Clock
}

// NewHandler creates a Handler with the given logger and clock.
// The clock is injected so tests can control time without calling time.Now()
// directly (forbidden in domain/application per CLAUDE.md).
func NewHandler(log *slog.Logger, clock shared.Clock) *Handler {
	return &Handler{log: log, clock: clock}
}

// Handle is the EventHandler the worker registers with the EventSubscriber.
// M2 will inject the consolidation use-cases here; PRE-0 logs receipt and
// returns nil.
func (h *Handler) Handle(ctx context.Context, payload PhaseArchivedReceived) error {
	h.log.InfoContext(ctx, "phase.archived received",
		slog.String("change_id", payload.ChangeID),
		slog.String("change_name", payload.ChangeName),
		slog.Time("archived_at", payload.ArchivedAt),
		slog.Time("received_at", h.clock.Now()),
	)
	return nil
}
