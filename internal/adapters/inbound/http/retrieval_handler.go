package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// RetrievalHandler handles HTTP requests for search operations.
type RetrievalHandler struct {
	searchSvc *retrieval.SearchService
}

// NewRetrievalHandler creates a new RetrievalHandler.
func NewRetrievalHandler(searchSvc *retrieval.SearchService) *RetrievalHandler {
	return &RetrievalHandler{searchSvc: searchSvc}
}

// --- Request/Response DTOs ---

type searchRequest struct {
	Query  string   `json:"query"`
	Scope  scopeDTO `json:"scope"`
	Types  []string `json:"types,omitempty"`
	Limit  *int     `json:"limit,omitempty"`
	Offset *int     `json:"offset,omitempty"`
}

type searchResponse struct {
	Results    []searchResultDTO `json:"results"`
	TotalCount int               `json:"total_count"`
	Query      string            `json:"query"`
}

type searchResultDTO struct {
	ID         string              `json:"id"`
	RecordType string              `json:"record_type"`
	Title      string              `json:"title"`
	Snippet    string              `json:"snippet"`
	Score      float64             `json:"score"`
	Ranking    rankingDTO          `json:"ranking"`
	Freshness  string              `json:"freshness"`
	CreatedAt  time.Time           `json:"created_at"`
}

type rankingDTO struct {
	FTSScore        float64 `json:"fts_score"`
	TRGMScore       float64 `json:"trgm_score"`
	RecencyBoost    float64 `json:"recency_boost"`
	ImportanceScore float64 `json:"importance_score"`
	TypeBoost       float64 `json:"type_boost"`
	FreshnessBoost  float64 `json:"freshness_boost"`
	ScopeExactness  float64 `json:"scope_exactness"`
	FinalScore      float64 `json:"final_score"`
}

func toSearchResponse(sr *inbound.SearchResults) searchResponse {
	results := make([]searchResultDTO, 0, len(sr.Results))
	for _, r := range sr.Results {
		results = append(results, searchResultDTO{
			ID:         r.ID.String(),
			RecordType: r.RecordType,
			Title:      r.Title,
			Snippet:    r.Snippet,
			Score:      r.Score,
			Ranking: rankingDTO{
				FTSScore:        r.Ranking.FTSScore,
				TRGMScore:       r.Ranking.TRGMScore,
				RecencyBoost:    r.Ranking.RecencyBoost,
				ImportanceScore: r.Ranking.ImportanceScore,
				TypeBoost:       r.Ranking.TypeBoost,
				FreshnessBoost:  r.Ranking.FreshnessBoost,
				ScopeExactness:  r.Ranking.ScopeExactness,
				FinalScore:      r.Ranking.FinalScore,
			},
			Freshness: string(r.Freshness),
			CreatedAt: r.CreatedAt,
		})
	}

	return searchResponse{
		Results:    results,
		TotalCount: sr.TotalCount,
		Query:      sr.Query,
	}
}

// Search handles POST /api/v1/search.
func (h *RetrievalHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	scope, err := parseScopeDTO(req.Scope)
	if err != nil {
		writeError(w, err)
		return
	}

	query := inbound.SearchQuery{
		Query:  req.Query,
		Scope:  scope,
		Types:  req.Types,
		Limit:  req.Limit,
		Offset: req.Offset,
	}

	results, err := h.searchSvc.Search(r.Context(), query)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toSearchResponse(results))
}
