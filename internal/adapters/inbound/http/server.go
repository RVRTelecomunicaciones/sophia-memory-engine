package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/sophia-engine/memory-engine/internal/adapters/inbound/http/middleware"
	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// NewRouter creates the HTTP router with all routes wired.
//
// db is used only by the /ready endpoint to probe Postgres connectivity;
// it reuses the application's pgxpool rather than opening a new connection.
//
// authSvc gates every route under /api/v1; /health and /ready stay public.
func NewRouter(
	memorySvc inbound.MemoryService,
	decisionSvc inbound.DecisionService,
	heuristicSvc inbound.HeuristicService,
	relationSvc inbound.RelationService,
	searchSvc *retrieval.SearchService,
	ctxBuilder *retrieval.ContextBuilder,
	purgeSvc inbound.PurgeService,
	feedbackSvc inbound.FeedbackService,
	authSvc inbound.AuthService,
	db DBPinger,
) chi.Router {
	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Recoverer)
	r.Use(RequestLogger)

	r.Route("/api/v1", func(r chi.Router) {
		// All /api/v1/* routes require a valid API key.
		r.Use(middleware.APIKey(authSvc))
		memHandler := NewMemoryHandler(memorySvc)
		r.Post("/memories", memHandler.Ingest)
		r.Get("/memories/by-topic-key", memHandler.GetByTopicKey)
		r.Get("/memories/{id}", memHandler.Get)
		r.Post("/memories/{id}/archive", memHandler.Archive)

		decHandler := NewDecisionHandler(decisionSvc)
		r.Post("/decisions", decHandler.Record)
		r.Get("/decisions/{id}", decHandler.Get)
		r.Get("/decisions/history/{key}", decHandler.GetHistory)
		r.Post("/decisions/{id}/contradict", decHandler.Contradict)

		heurHandler := NewHeuristicHandler(heuristicSvc)
		r.Post("/heuristics", heurHandler.Create)
		r.Get("/heuristics/active/{key}", heurHandler.GetActive)
		r.Get("/heuristics", heurHandler.ListByScope)
		r.Post("/heuristics/{id}/toggle", heurHandler.Toggle)

		relHandler := NewRelationHandler(relationSvc)
		r.Post("/relations", relHandler.Create)
		r.Get("/relations/from/{id}", relHandler.GetFrom)
		r.Get("/relations/to/{id}", relHandler.GetTo)

		retHandler := NewRetrievalHandler(searchSvc, ctxBuilder)
		r.Post("/search", retHandler.Search)
		r.Post("/search/context", retHandler.BuildContext)

		purgeHandler := NewPurgeHandler(purgeSvc)
		r.Post("/purge/request", purgeHandler.Request)
		r.Post("/purge/{id}/execute", purgeHandler.Execute)

		feedbackHandler := NewFeedbackHandler(feedbackSvc)
		r.Post("/feedback", feedbackHandler.Submit)
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	readyHandler := NewReadyHandler(db)
	r.Get("/ready", readyHandler.Ready)

	return r
}
