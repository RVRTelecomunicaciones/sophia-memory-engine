package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// HeuristicHandler handles HTTP requests for heuristic lifecycle operations.
type HeuristicHandler struct {
	svc inbound.HeuristicService
}

// NewHeuristicHandler creates a new HeuristicHandler.
func NewHeuristicHandler(svc inbound.HeuristicService) *HeuristicHandler {
	return &HeuristicHandler{svc: svc}
}

// --- Request DTOs ---

type createHeuristicRequest struct {
	HeuristicKey string        `json:"heuristic_key"`
	Condition    string        `json:"condition"`
	Action       string        `json:"action"`
	Rationale    string        `json:"rationale"`
	Scope        scopeDTO      `json:"scope"`
	Provenance   provenanceDTO `json:"provenance"`
	Confidence   confidenceDTO `json:"confidence"`
	Enabled      *bool         `json:"enabled,omitempty"`
	ValidUntil   *string       `json:"valid_until,omitempty"`
	FTSLanguage  *string       `json:"fts_language,omitempty"`
}

type confidenceDTO struct {
	Score  float64 `json:"score"`
	Source string  `json:"source"`
}

type toggleHeuristicRequest struct {
	Enabled bool `json:"enabled"`
}

// --- Response DTOs ---

type createHeuristicResponse struct {
	ID               string    `json:"id"`
	Version          int       `json:"version"`
	DisabledPrevious *string   `json:"disabled_previous,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type heuristicResponse struct {
	ID           string              `json:"id"`
	HeuristicKey string              `json:"heuristic_key"`
	Version      int                 `json:"version"`
	Condition    string              `json:"condition"`
	Action       string              `json:"action"`
	Rationale    string              `json:"rationale"`
	Scope        scopeDTO            `json:"scope"`
	Provenance   heuristicProvDTO    `json:"provenance"`
	Confidence   confidenceRespDTO   `json:"confidence"`
	Enabled      bool                `json:"enabled"`
	Temporal     temporalRespDTO     `json:"temporal"`
	FTSLanguage  string              `json:"fts_language"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type heuristicProvDTO struct {
	Source    string  `json:"source"`
	SourceURI *string `json:"source_uri,omitempty"`
	Method   string  `json:"method"`
}

type confidenceRespDTO struct {
	Score  float64 `json:"score"`
	Source string  `json:"source"`
}

func toHeuristicResponse(rule *heuristic.HeuristicRule) heuristicResponse {
	return heuristicResponse{
		ID:           rule.ID.String(),
		HeuristicKey: rule.HeuristicKey,
		Version:      rule.Version,
		Condition:    rule.Condition,
		Action:       rule.Action,
		Rationale:    rule.Rationale,
		Scope: scopeDTO{
			TenantID:    rule.Scope.TenantID,
			ProjectID:   rule.Scope.ProjectID,
			RepoID:      rule.Scope.RepoID,
			AgentID:     rule.Scope.AgentID,
			SessionID:   rule.Scope.SessionID,
			Environment: rule.Scope.Environment,
		},
		Provenance: heuristicProvDTO{
			Source:    rule.Provenance.Source,
			SourceURI: rule.Provenance.SourceURI,
			Method:   string(rule.Provenance.Method),
		},
		Confidence: confidenceRespDTO{
			Score:  rule.Confidence.Score,
			Source: string(rule.Confidence.Source),
		},
		Enabled: rule.Enabled,
		Temporal: temporalRespDTO{
			ValidFrom:    rule.Temporal.ValidFrom,
			ValidUntil:   rule.Temporal.ValidUntil,
			LastAccessed: rule.Temporal.LastAccessed,
			Freshness:    string(rule.Temporal.Freshness),
		},
		FTSLanguage: rule.FTSLanguage,
		CreatedAt:   rule.CreatedAt,
		UpdatedAt:   rule.UpdatedAt,
	}
}

func toHeuristicResponseList(rules []heuristic.HeuristicRule) []heuristicResponse {
	resp := make([]heuristicResponse, 0, len(rules))
	for i := range rules {
		resp = append(resp, toHeuristicResponse(&rules[i]))
	}
	return resp
}

// Create handles POST /api/v1/heuristics.
func (h *HeuristicHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createHeuristicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	scope, err := parseScopeDTO(req.Scope)
	if err != nil {
		writeError(w, err)
		return
	}

	prov, err := parseProvenanceDTO(req.Provenance)
	if err != nil {
		writeError(w, err)
		return
	}

	confidence, err := shared.NewConfidence(req.Confidence.Score, shared.ConfidenceSource(req.Confidence.Source))
	if err != nil {
		writeError(w, err)
		return
	}

	var validUntil *time.Time
	if req.ValidUntil != nil {
		t, err := time.Parse(time.RFC3339, *req.ValidUntil)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_until format, expected RFC3339"})
			return
		}
		validUntil = &t
	}

	cmd := inbound.CreateHeuristicCmd{
		HeuristicKey: req.HeuristicKey,
		Condition:    req.Condition,
		Action:       req.Action,
		Rationale:    req.Rationale,
		FTSLanguage:  req.FTSLanguage,
		Scope:        scope,
		Provenance:   prov,
		Confidence:   confidence,
		Enabled:      req.Enabled,
		ValidUntil:   validUntil,
	}

	result, err := h.svc.Create(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := createHeuristicResponse{
		ID:        result.ID.String(),
		Version:   result.Version,
		CreatedAt: result.CreatedAt,
	}
	if result.DisabledPrevious != nil {
		s := result.DisabledPrevious.String()
		resp.DisabledPrevious = &s
	}

	writeJSON(w, http.StatusCreated, resp)
}

// GetActive handles GET /api/v1/heuristics/active/{key}.
func (h *HeuristicHandler) GetActive(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "heuristic key is required"})
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query parameter is required"})
		return
	}

	var opts []shared.ScopeOption
	if v := r.URL.Query().Get("repo_id"); v != "" {
		opts = append(opts, shared.WithRepoID(v))
	}
	if v := r.URL.Query().Get("agent_id"); v != "" {
		opts = append(opts, shared.WithAgentID(v))
	}
	if v := r.URL.Query().Get("environment"); v != "" {
		opts = append(opts, shared.WithEnvironment(v))
	}

	scope, err := shared.NewScope(projectID, opts...)
	if err != nil {
		writeError(w, err)
		return
	}

	rule, err := h.svc.GetActive(r.Context(), inbound.GetActiveHeuristicQuery{
		HeuristicKey: key,
		Scope:        scope,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toHeuristicResponse(rule))
}

// ListByScope handles GET /api/v1/heuristics.
func (h *HeuristicHandler) ListByScope(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query parameter is required"})
		return
	}

	var opts []shared.ScopeOption
	if v := r.URL.Query().Get("tenant_id"); v != "" {
		opts = append(opts, shared.WithTenantID(v))
	}
	if v := r.URL.Query().Get("repo_id"); v != "" {
		opts = append(opts, shared.WithRepoID(v))
	}
	if v := r.URL.Query().Get("agent_id"); v != "" {
		opts = append(opts, shared.WithAgentID(v))
	}
	if v := r.URL.Query().Get("session_id"); v != "" {
		opts = append(opts, shared.WithSessionID(v))
	}
	if v := r.URL.Query().Get("environment"); v != "" {
		opts = append(opts, shared.WithEnvironment(v))
	}

	scope, err := shared.NewScope(projectID, opts...)
	if err != nil {
		writeError(w, err)
		return
	}

	var enabled *bool
	if v := r.URL.Query().Get("enabled"); v != "" {
		switch v {
		case "true":
			enabled = new(bool)
			*enabled = true
		case "false":
			enabled = new(bool)
			*enabled = false
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled must be 'true' or 'false'"})
			return
		}
	}

	rules, err := h.svc.ListByScope(r.Context(), inbound.ListHeuristicsQuery{
		Scope:   scope,
		Enabled: enabled,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toHeuristicResponseList(rules))
}

// Toggle handles POST /api/v1/heuristics/{id}/toggle.
func (h *HeuristicHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := shared.RecordIDFromString(idStr)
	if err != nil {
		writeError(w, err)
		return
	}

	var req toggleHeuristicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cmd := inbound.ToggleHeuristicCmd{
		ID:      id,
		Enabled: req.Enabled,
	}

	if err := h.svc.Toggle(r.Context(), cmd); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "toggled"})
}
