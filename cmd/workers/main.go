package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sophia-engine/memory-engine/internal/application/consolidation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	clock := shared.RealClock{}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	handler := consolidation.NewHandler(log, clock)

	// TODO(M2): replace with concrete EventSubscriber (SSE client, webhook
	// receiver, or message bus consumer). PRE-0 ships a nil subscriber so
	// the binary builds, the graceful lifecycle is exercised, and tests can
	// substitute FakeSubscriber.
	var subscriber consolidation.EventSubscriber // nil — see TODO above
	if subscriber == nil {
		log.WarnContext(ctx, "worker started with no EventSubscriber wired; awaiting M2 transport decision",
			slog.String("event_type", consolidation.PhaseArchivedEventType),
		)
		<-ctx.Done()
		log.InfoContext(ctx, "worker shutting down", slog.String("cause", ctx.Err().Error()))
		return
	}

	if err := subscriber.Subscribe(ctx, consolidation.PhaseArchivedEventType, handler.Handle); err != nil {
		log.ErrorContext(ctx, "subscribe failed", slog.String("err", err.Error()))
		os.Exit(1)
	}

	<-ctx.Done()
	log.InfoContext(ctx, "worker shutting down", slog.String("cause", ctx.Err().Error()))
}
