package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// RelationHandler handles HTTP requests for relation graph operations.
type RelationHandler struct {
	svc inbound.RelationService
}

// NewRelationHandler creates a new RelationHandler.
func NewRelationHandler(svc inbound.RelationService) *RelationHandler {
	return &RelationHandler{svc: svc}
}

// --- Request DTOs ---

type createRelationRequest struct {
	SourceID  string         `json:"source_id"`
	TargetID  string         `json:"target_id"`
	Type      string         `json:"type"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Scope     scopeDTO       `json:"scope"`
	ValidFrom *string        `json:"valid_from,omitempty"`
}

// --- Response DTOs ---

type createRelationResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type relationResultDTO struct {
	ID        string         `json:"id"`
	SourceID  string         `json:"source_id"`
	TargetID  string         `json:"target_id"`
	Type      string         `json:"type"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Depth     int            `json:"depth"`
	Path      []string       `json:"path"`
	CreatedAt time.Time      `json:"created_at"`
}

func toRelationResultDTOs(results []inbound.RelationResult) []relationResultDTO {
	dtos := make([]relationResultDTO, 0, len(results))
	for _, r := range results {
		path := make([]string, 0, len(r.Path))
		for _, p := range r.Path {
			path = append(path, p.String())
		}
		dtos = append(dtos, relationResultDTO{
			ID:        r.Relation.ID.String(),
			SourceID:  r.Relation.SourceID.String(),
			TargetID:  r.Relation.TargetID.String(),
			Type:      string(r.Relation.Type),
			Metadata:  r.Relation.Metadata,
			Depth:     r.Depth,
			Path:      path,
			CreatedAt: r.Relation.CreatedAt,
		})
	}
	return dtos
}

// Create handles POST /api/v1/relations.
func (h *RelationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRelationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	sourceID, err := shared.RecordIDFromString(req.SourceID)
	if err != nil {
		writeError(w, err)
		return
	}

	targetID, err := shared.RecordIDFromString(req.TargetID)
	if err != nil {
		writeError(w, err)
		return
	}

	scope, err := parseScopeDTO(req.Scope)
	if err != nil {
		writeError(w, err)
		return
	}

	cmd := inbound.CreateRelationCmd{
		SourceID: sourceID,
		TargetID: targetID,
		Type:     shared.RelationType(req.Type),
		Metadata: req.Metadata,
		Scope:    scope,
	}

	if req.ValidFrom != nil {
		t, err := time.Parse(time.RFC3339, *req.ValidFrom)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_from format, expected RFC3339"})
			return
		}
		cmd.ValidFrom = &t
	}

	result, err := h.svc.Create(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, createRelationResponse{
		ID:        result.ID.String(),
		CreatedAt: result.CreatedAt,
	})
}

// GetFrom handles GET /api/v1/relations/from/{id}.
func (h *RelationHandler) GetFrom(w http.ResponseWriter, r *http.Request) {
	h.handleTraverse(w, r)
}

// GetTo handles GET /api/v1/relations/to/{id}.
func (h *RelationHandler) GetTo(w http.ResponseWriter, r *http.Request) {
	h.handleTraverse(w, r)
}

// handleTraverse parses the common query params and delegates to the correct service method.
func (h *RelationHandler) handleTraverse(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	recordID, err := shared.RecordIDFromString(idStr)
	if err != nil {
		writeError(w, err)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query parameter is required"})
		return
	}

	var scopeOpts []shared.ScopeOption
	if v := r.URL.Query().Get("tenant_id"); v != "" {
		scopeOpts = append(scopeOpts, shared.WithTenantID(v))
	}
	if v := r.URL.Query().Get("repo_id"); v != "" {
		scopeOpts = append(scopeOpts, shared.WithRepoID(v))
	}
	if v := r.URL.Query().Get("agent_id"); v != "" {
		scopeOpts = append(scopeOpts, shared.WithAgentID(v))
	}
	if v := r.URL.Query().Get("environment"); v != "" {
		scopeOpts = append(scopeOpts, shared.WithEnvironment(v))
	}

	scope, err := shared.NewScope(projectID, scopeOpts...)
	if err != nil {
		writeError(w, err)
		return
	}

	query := inbound.RelationQuery{
		RecordID: recordID,
		Scope:    &scope,
	}

	if v := r.URL.Query().Get("type"); v != "" {
		rt := shared.RelationType(v)
		query.Type = &rt
	}

	if v := r.URL.Query().Get("max_depth"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_depth, expected integer"})
			return
		}
		query.MaxDepth = &d
	}

	if v := r.URL.Query().Get("valid_at"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_at format, expected RFC3339"})
			return
		}
		query.ValidAt = &t
	}

	// Determine which service method to call based on the route path.
	var results []inbound.RelationResult
	routePattern := chi.RouteContext(r.Context()).RoutePattern()
	if routePattern == "/api/v1/relations/to/{id}" {
		results, err = h.svc.GetTo(r.Context(), query)
	} else {
		results, err = h.svc.GetFrom(r.Context(), query)
	}
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toRelationResultDTOs(results))
}
