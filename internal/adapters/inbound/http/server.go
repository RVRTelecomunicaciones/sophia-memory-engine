package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// NewRouter creates the HTTP router with all routes wired.
func NewRouter(
	memorySvc inbound.MemoryService,
	searchSvc *retrieval.SearchService,
) chi.Router {
	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Recoverer)
	r.Use(RequestLogger)

	r.Route("/api/v1", func(r chi.Router) {
		memHandler := NewMemoryHandler(memorySvc)
		r.Post("/memories", memHandler.Ingest)
		r.Get("/memories/{id}", memHandler.Get)
		r.Post("/memories/{id}/archive", memHandler.Archive)

		retHandler := NewRetrievalHandler(searchSvc)
		r.Post("/search", retHandler.Search)
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return r
}
