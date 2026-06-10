// cmd/workers is the background worker binary for sophia-memory-engine.
//
// M2 design decision (D-M2-1, N.5, locked):
//
//	The phase.archived webhook receiver lives in the MAIN HTTP server
//	(cmd/memory-engine) under POST /api/v1/worker/phase-archived, NOT in this
//	binary. The receiver is registered in internal/adapters/inbound/http/server.go
//	by passing a non-nil HandlerV2 to NewRouter.
//
//	This binary (cmd/workers) stays minimal, pending M3+ scheduler work. Its
//	sole responsibility is graceful lifecycle management. When the scheduler
//	work ships, this binary will house cron-style maintenance jobs.
//
// For manual / CLI-triggered consolidation during development, set
// SOPHIA_MEMORY_WORKER_ENABLED=true in cmd/memory-engine to enable the HTTP
// receiver with a fully-wired HandlerV2 pipeline.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	_ = shared.RealClock{} // imported for future scheduler use

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// NOTE: The consolidation worker pipeline (HandlerV2) is wired into the
	// main HTTP server binary (cmd/memory-engine), not here. See D-M2-1 and
	// locked decision N.5 in the consolidation-worker SDD for the rationale.
	// This binary runs the graceful lifecycle and will host M3+ scheduled jobs.
	log.InfoContext(ctx, "workers: started (scheduler pending M3+)")

	<-ctx.Done()
	log.InfoContext(ctx, "workers: shutting down", slog.String("cause", ctx.Err().Error()))
}
