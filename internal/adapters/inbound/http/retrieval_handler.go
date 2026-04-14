package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/retrieval"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// RetrievalHandler handles HTTP requests for search and context operations.
type RetrievalHandler struct {
	searchSvc      *retrieval.SearchService
	contextBuilder *retrieval.ContextBuilder
}

// NewRetrievalHandler creates a new RetrievalHandler.
func NewRetrievalHandler(searchSvc *retrieval.SearchService, ctxBuilder *retrieval.ContextBuilder) *RetrievalHandler {
	return &RetrievalHandler{searchSvc: searchSvc, contextBuilder: ctxBuilder}
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

// --- Context Builder DTOs ---

type contextRequest struct {
	Scope        scopeDTO `json:"scope"`
	Query        *string  `json:"query,omitempty"`
	MaxTokens    *int     `json:"max_tokens,omitempty"`
	IncludeTypes []string `json:"include_types,omitempty"`
	ExpandGraph  *bool    `json:"expand_graph,omitempty"`
}

type contextBundleResponse struct {
	Sections    []contextSectionDTO `json:"sections"`
	TotalTokens int                 `json:"total_tokens"`
	Truncated   bool                `json:"truncated"`
	GeneratedAt time.Time           `json:"generated_at"`
	DebugInfo   *contextDebugDTO    `json:"debug_info,omitempty"`
}

type contextSectionDTO struct {
	Type       string             `json:"type"`
	Records    []contextRecordDTO `json:"records"`
	TokenCount int                `json:"token_count"`
}

type contextRecordDTO struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
	Relation *string `json:"relation,omitempty"`
}

type contextDebugDTO struct {
	Strategy        string `json:"strategy"`
	RecordsScanned  int    `json:"records_scanned"`
	RecordsIncluded int    `json:"records_included"`
	GraphExpanded   bool   `json:"graph_expanded"`
	GraphDepth      int    `json:"graph_depth"`
	TokenBudgetUsed int    `json:"token_budget_used"`
	DurationMs      int64  `json:"duration_ms"`
}

func toContextBundleResponse(cb *inbound.ContextBundle) contextBundleResponse {
	sections := make([]contextSectionDTO, 0, len(cb.Sections))
	for _, s := range cb.Sections {
		records := make([]contextRecordDTO, 0, len(s.Records))
		for _, r := range s.Records {
			records = append(records, contextRecordDTO{
				ID:       r.ID.String(),
				Type:     r.Type,
				Content:  r.Content,
				Score:    r.Score,
				Relation: r.Relation,
			})
		}
		sections = append(sections, contextSectionDTO{
			Type:       s.Type,
			Records:    records,
			TokenCount: s.TokenCount,
		})
	}

	resp := contextBundleResponse{
		Sections:    sections,
		TotalTokens: cb.TotalTokens,
		Truncated:   cb.Truncated,
		GeneratedAt: cb.GeneratedAt,
	}

	if cb.DebugInfo != nil {
		resp.DebugInfo = &contextDebugDTO{
			Strategy:        cb.DebugInfo.Strategy,
			RecordsScanned:  cb.DebugInfo.RecordsScanned,
			RecordsIncluded: cb.DebugInfo.RecordsIncluded,
			GraphExpanded:   cb.DebugInfo.GraphExpanded,
			GraphDepth:      cb.DebugInfo.GraphDepth,
			TokenBudgetUsed: cb.DebugInfo.TokenBudgetUsed,
			DurationMs:      cb.DebugInfo.DurationMs,
		}
	}

	return resp
}

// BuildContext handles POST /api/v1/search/context.
func (h *RetrievalHandler) BuildContext(w http.ResponseWriter, r *http.Request) {
	var req contextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	scope, err := parseScopeDTO(req.Scope)
	if err != nil {
		writeError(w, err)
		return
	}

	ctxReq := inbound.ContextRequest{
		Scope:        scope,
		Query:        req.Query,
		MaxTokens:    req.MaxTokens,
		IncludeTypes: req.IncludeTypes,
		ExpandGraph:  req.ExpandGraph,
	}

	bundle, err := h.contextBuilder.BuildContext(r.Context(), ctxReq)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toContextBundleResponse(bundle))
}
