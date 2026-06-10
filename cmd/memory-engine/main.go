package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	gohttp "net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	apphttp "github.com/sophia-engine/memory-engine/internal/adapters/inbound/http"
	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/search"
	"github.com/sophia-engine/memory-engine/internal/application/authsvc"
	"github.com/sophia-engine/memory-engine/internal/application/decisions"
	"github.com/sophia-engine/memory-engine/internal/application/feedback"
	"github.com/sophia-engine/memory-engine/internal/application/heuristics"
	"github.com/sophia-engine/memory-engine/internal/application/ingest"
	"github.com/sophia-engine/memory-engine/internal/application/relations"
	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/application/security"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/database"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/events"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/logging"
)

func main() {
	logger := logging.DefaultLogger()
	slog.SetDefault(logger)

	cfg := config.DefaultConfig()

	// Override from environment.
	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Database.Port = p
		}
	}
	if user := os.Getenv("DB_USER"); user != "" {
		cfg.Database.User = user
	}
	if pass := os.Getenv("DB_PASSWORD"); pass != "" {
		cfg.Database.Password = pass
	}
	if name := os.Getenv("DB_NAME"); name != "" {
		cfg.Database.DBName = name
	}
	if sslMode := os.Getenv("DB_SSLMODE"); sslMode != "" {
		cfg.Database.SSLMode = sslMode
	}
	if port := os.Getenv("PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	}

	// Use signal.NotifyContext for graceful shutdown (Go 1.26).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Database.
	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Infrastructure.
	clock := shared.RealClock{}
	eventPub := events.NewInProcessEventPublisher()

	// Repositories.
	memRepo := persistence.NewMemoryPgRepository(pool)
	decRepo := persistence.NewDecisionPgRepository(pool)
	heurRepo := persistence.NewHeuristicPgRepository(pool)
	relRepo := persistence.NewRelationPgRepository(pool)
	searchIdx := search.NewPostgresFTSIndex(pool)
	apiKeyRepo := persistence.NewAPIKeyPgRepository(pool)

	// Transaction manager.
	txMgr := persistence.NewPgTxManager(pool)

	// Application services.
	memorySvc := ingest.NewService(memRepo, searchIdx, eventPub, clock)
	decisionSvc := decisions.NewService(decRepo, relRepo, txMgr, eventPub, clock)
	heuristicSvc := heuristics.NewService(heurRepo, txMgr, eventPub, clock)
	relationSvc := relations.NewService(relRepo, eventPub, clock)
	searchSvc := retrieval.NewSearchService(searchIdx, memRepo, cfg.Retrieval, clock)
	ctxBuilder := retrieval.NewContextBuilder(searchIdx, memRepo, decRepo, heurRepo, relRepo, cfg.Retrieval, clock)
	purgeRepo := persistence.NewPurgePgRepository(pool)
	purgeSvc := security.NewService(purgeRepo, memRepo, relRepo, searchIdx, txMgr, eventPub, clock)
	feedbackRepo := persistence.NewFeedbackPgRepository(pool)
	feedbackSvc := feedback.NewService(feedbackRepo, eventPub, clock)
	authSvc := authsvc.NewService(apiKeyRepo, clock)

	// HTTP.
	// workerPipeline is nil here — the consolidation worker pipeline is wired
	// when SOPHIA_MEMORY_WORKER_ENABLED is set (see cmd/workers or future
	// bootstrap config). The webhook receiver registers automatically when
	// a non-nil pipeline is provided. Per locked decision N.5 the endpoint
	// lives in this main HTTP server; cmd/workers stays minimal.
	router := apphttp.NewRouter(memorySvc, decisionSvc, heuristicSvc, relationSvc, searchSvc, ctxBuilder, purgeSvc, feedbackSvc, authSvc, pool, nil)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &gohttp.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	// Start server in goroutine.
	go func() {
		slog.Info("starting memory-engine", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != gohttp.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for signal.
	<-ctx.Done()
	slog.Info("shutting down...", "cause", ctx.Err())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	slog.Info("server stopped")
}
