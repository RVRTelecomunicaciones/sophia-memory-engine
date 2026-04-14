package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// DecisionHandler handles HTTP requests for decision lifecycle operations.
type DecisionHandler struct {
	svc inbound.DecisionService
}

// NewDecisionHandler creates a new DecisionHandler.
func NewDecisionHandler(svc inbound.DecisionService) *DecisionHandler {
	return &DecisionHandler{svc: svc}
}

// --- Request DTOs ---

type recordDecisionRequest struct {
	DecisionKey string             `json:"decision_key"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Rationale   string             `json:"rationale"`
	Evidence    []evidenceReqDTO   `json:"evidence"`
	Scope       scopeDTO           `json:"scope"`
	Provenance  provenanceDTO      `json:"provenance"`
	Confidence  confidenceReqDTO   `json:"confidence"`
	ValidFrom   *string            `json:"valid_from,omitempty"`
}

type evidenceReqDTO struct {
	Type     string  `json:"type"`
	RecordID *string `json:"record_id,omitempty"`
	URI      *string `json:"uri,omitempty"`
	Excerpt  *string `json:"excerpt,omitempty"`
}

type confidenceReqDTO struct {
	Score  float64 `json:"score"`
	Source string  `json:"source"`
}

type contradictRequest struct {
	ContradictedBy string `json:"contradicted_by"`
	Reason         string `json:"reason"`
}

// --- Response DTOs ---

type recordDecisionResponse struct {
	ID         string    `json:"id"`
	Version    int       `json:"version"`
	Superseded *string   `json:"superseded,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type decisionResponse struct {
	ID          string            `json:"id"`
	DecisionKey string            `json:"decision_key"`
	Version     int               `json:"version"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Rationale   string            `json:"rationale"`
	Evidence    []evidenceRespDTO `json:"evidence"`
	Scope       scopeDTO          `json:"scope"`
	Provenance  provenanceRespDTO `json:"provenance"`
	Temporal    temporalRespDTO   `json:"temporal"`
	Confidence  confidenceRespDTO `json:"confidence"`
	Status      string            `json:"status"`
	SupersededBy *string          `json:"superseded_by,omitempty"`
	FTSLanguage string            `json:"fts_language"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type evidenceRespDTO struct {
	Type     string  `json:"type"`
	RecordID *string `json:"record_id,omitempty"`
	URI      *string `json:"uri,omitempty"`
	Excerpt  *string `json:"excerpt,omitempty"`
}

func toDecisionResponse(d *decision.Decision) decisionResponse {
	evidence := make([]evidenceRespDTO, 0, len(d.Evidence))
	for _, e := range d.Evidence {
		dto := evidenceRespDTO{
			Type:    string(e.Type),
			URI:     e.URI,
			Excerpt: e.Excerpt,
		}
		if e.RecordID != nil {
			s := e.RecordID.String()
			dto.RecordID = &s
		}
		evidence = append(evidence, dto)
	}

	var supersededBy *string
	if d.SupersededBy != nil {
		s := d.SupersededBy.String()
		supersededBy = &s
	}

	var provParentID *string
	if d.Provenance.ParentID != nil {
		s := d.Provenance.ParentID.String()
		provParentID = &s
	}

	return decisionResponse{
		ID:          d.ID.String(),
		DecisionKey: d.DecisionKey,
		Version:     d.Version,
		Title:       d.Title,
		Description: d.Description,
		Rationale:   d.Rationale,
		Evidence:    evidence,
		Scope: scopeDTO{
			TenantID:    d.Scope.TenantID,
			ProjectID:   d.Scope.ProjectID,
			RepoID:      d.Scope.RepoID,
			AgentID:     d.Scope.AgentID,
			SessionID:   d.Scope.SessionID,
			Environment: d.Scope.Environment,
		},
		Provenance: provenanceRespDTO{
			Source:    d.Provenance.Source,
			SourceURI: d.Provenance.SourceURI,
			Method:   string(d.Provenance.Method),
			ParentID: provParentID,
		},
		Temporal: temporalRespDTO{
			ValidFrom:    d.Temporal.ValidFrom,
			ValidUntil:   d.Temporal.ValidUntil,
			LastAccessed: d.Temporal.LastAccessed,
			Freshness:    string(d.Temporal.Freshness),
		},
		Confidence: confidenceRespDTO{
			Score:  d.Confidence.Score,
			Source: string(d.Confidence.Source),
		},
		Status:       string(d.Status),
		SupersededBy: supersededBy,
		FTSLanguage:  d.FTSLanguage,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
}

// Record handles POST /api/v1/decisions.
func (h *DecisionHandler) Record(w http.ResponseWriter, r *http.Request) {
	var req recordDecisionRequest
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

	evidence := make([]shared.EvidenceRef, 0, len(req.Evidence))
	for _, e := range req.Evidence {
		var recordID *shared.RecordID
		if e.RecordID != nil {
			rid, err := shared.RecordIDFromString(*e.RecordID)
			if err != nil {
				writeError(w, err)
				return
			}
			recordID = &rid
		}

		ref, err := shared.NewEvidenceRef(shared.EvidenceType(e.Type), recordID, e.URI, e.Excerpt)
		if err != nil {
			writeError(w, err)
			return
		}
		evidence = append(evidence, ref)
	}

	cmd := inbound.RecordDecisionCmd{
		DecisionKey: req.DecisionKey,
		Title:       req.Title,
		Description: req.Description,
		Rationale:   req.Rationale,
		Evidence:    evidence,
		Scope:       scope,
		Provenance:  prov,
		Confidence:  confidence,
	}

	if req.ValidFrom != nil {
		t, err := time.Parse(time.RFC3339, *req.ValidFrom)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_from format, expected RFC3339"})
			return
		}
		cmd.ValidFrom = &t
	}

	result, err := h.svc.Record(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := recordDecisionResponse{
		ID:        result.ID.String(),
		Version:   result.Version,
		CreatedAt: result.CreatedAt,
	}
	if result.Superseded != nil {
		s := result.Superseded.String()
		resp.Superseded = &s
	}

	writeJSON(w, http.StatusCreated, resp)
}

// Get handles GET /api/v1/decisions/{id}.
func (h *DecisionHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := shared.RecordIDFromString(idStr)
	if err != nil {
		writeError(w, err)
		return
	}

	d, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toDecisionResponse(d))
}

// GetHistory handles GET /api/v1/decisions/history/{key}.
func (h *DecisionHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

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

	results, err := h.svc.GetHistory(r.Context(), inbound.DecisionHistoryQuery{
		DecisionKey: key,
		Scope:       scope,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	resp := make([]decisionResponse, 0, len(results))
	for i := range results {
		resp = append(resp, toDecisionResponse(&results[i]))
	}

	writeJSON(w, http.StatusOK, resp)
}

// Contradict handles POST /api/v1/decisions/{id}/contradict.
func (h *DecisionHandler) Contradict(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	targetID, err := shared.RecordIDFromString(idStr)
	if err != nil {
		writeError(w, err)
		return
	}

	var req contradictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	contradictedBy, err := shared.RecordIDFromString(req.ContradictedBy)
	if err != nil {
		writeError(w, err)
		return
	}

	// Build a minimal provenance for the contradict operation.
	prov, err := shared.NewProvenance("contradict-api", shared.IngestMethodDirect, nil)
	if err != nil {
		writeError(w, err)
		return
	}

	cmd := inbound.ContradictDecisionCmd{
		TargetID:       targetID,
		ContradictedBy: contradictedBy,
		Reason:         req.Reason,
		Provenance:     prov,
	}

	if err := h.svc.Contradict(r.Context(), cmd); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "contradicted"})
}
