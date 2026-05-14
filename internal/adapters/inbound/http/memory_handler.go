package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// MemoryHandler handles HTTP requests for memory lifecycle operations.
type MemoryHandler struct {
	svc inbound.MemoryService
}

// NewMemoryHandler creates a new MemoryHandler.
func NewMemoryHandler(svc inbound.MemoryService) *MemoryHandler {
	return &MemoryHandler{svc: svc}
}

// --- Request DTOs ---

type ingestRequest struct {
	Type        string        `json:"type"`
	Content     string        `json:"content"`
	Summary     *string       `json:"summary,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	TopicKey    *string       `json:"topic_key,omitempty"`
	FTSLanguage *string       `json:"fts_language,omitempty"`
	Scope       scopeDTO      `json:"scope"`
	Provenance  provenanceDTO `json:"provenance"`
	ValidFrom   *string       `json:"valid_from,omitempty"`
	ValidUntil  *string       `json:"valid_until,omitempty"`
}

type archiveRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
}

// --- Response DTOs ---

type ingestResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type memoryResponse struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Content       string             `json:"content"`
	Summary       *string            `json:"summary,omitempty"`
	Tags          []string           `json:"tags"`
	TopicKey      *string            `json:"topic_key,omitempty"`
	Scope         scopeDTO           `json:"scope"`
	Provenance    provenanceRespDTO  `json:"provenance"`
	Temporal      temporalRespDTO    `json:"temporal"`
	Importance    importanceRespDTO  `json:"importance"`
	Status        string             `json:"status"`
	ArchivedBy    *string            `json:"archived_by,omitempty"`
	ArchiveReason *string            `json:"archive_reason,omitempty"`
	FTSLanguage   string             `json:"fts_language"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

type provenanceRespDTO struct {
	Source    string  `json:"source"`
	SourceURI *string `json:"source_uri,omitempty"`
	Method   string  `json:"method"`
	ParentID *string `json:"parent_id,omitempty"`
}

type temporalRespDTO struct {
	ValidFrom    *time.Time `json:"valid_from,omitempty"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	LastAccessed *time.Time `json:"last_accessed,omitempty"`
	Freshness    string     `json:"freshness"`
}

type importanceRespDTO struct {
	Score      float64                `json:"score"`
	ComputedAt time.Time              `json:"computed_at"`
	Factors    []importanceFactorDTO  `json:"factors"`
}

type importanceFactorDTO struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Value  float64 `json:"value"`
}

func toMemoryResponse(rec *memory.MemoryRecord) memoryResponse {
	var parentID *string
	if rec.Provenance.ParentID != nil {
		s := rec.Provenance.ParentID.String()
		parentID = &s
	}

	factors := make([]importanceFactorDTO, 0, len(rec.Importance.Factors))
	for _, f := range rec.Importance.Factors {
		factors = append(factors, importanceFactorDTO{
			Name:   f.Name,
			Weight: f.Weight,
			Value:  f.Value,
		})
	}

	return memoryResponse{
		ID:      rec.ID.String(),
		Type:    string(rec.Type),
		Content: rec.Content,
		Summary: rec.Summary,
		Tags:    rec.Tags,
		TopicKey: rec.TopicKey,
		Scope: scopeDTO{
			TenantID:    rec.Scope.TenantID,
			ProjectID:   rec.Scope.ProjectID,
			RepoID:      rec.Scope.RepoID,
			AgentID:     rec.Scope.AgentID,
			SessionID:   rec.Scope.SessionID,
			Environment: rec.Scope.Environment,
		},
		Provenance: provenanceRespDTO{
			Source:    rec.Provenance.Source,
			SourceURI: rec.Provenance.SourceURI,
			Method:   string(rec.Provenance.Method),
			ParentID: parentID,
		},
		Temporal: temporalRespDTO{
			ValidFrom:    rec.Temporal.ValidFrom,
			ValidUntil:   rec.Temporal.ValidUntil,
			LastAccessed: rec.Temporal.LastAccessed,
			Freshness:    string(rec.Temporal.Freshness),
		},
		Importance: importanceRespDTO{
			Score:      rec.Importance.Score,
			ComputedAt: rec.Importance.ComputedAt,
			Factors:    factors,
		},
		Status:        string(rec.Status),
		ArchivedBy:    rec.ArchivedBy,
		ArchiveReason: rec.ArchiveReason,
		FTSLanguage:   rec.FTSLanguage,
		CreatedAt:     rec.CreatedAt,
		UpdatedAt:     rec.UpdatedAt,
	}
}

// Ingest handles POST /api/v1/memories.
func (h *MemoryHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var req ingestRequest
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

	validFrom, err := parseTime(req.ValidFrom)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_from format, expected RFC3339"})
		return
	}

	validUntil, err := parseTime(req.ValidUntil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_until format, expected RFC3339"})
		return
	}

	cmd := inbound.IngestMemoryCmd{
		Type:        shared.MemoryType(req.Type),
		Content:     req.Content,
		Summary:     req.Summary,
		Tags:        req.Tags,
		TopicKey:    req.TopicKey,
		FTSLanguage: req.FTSLanguage,
		Scope:       scope,
		Provenance:  prov,
		ValidFrom:   validFrom,
		ValidUntil:  validUntil,
	}

	result, err := h.svc.Ingest(r.Context(), cmd)
	if err != nil {
		slog.WarnContext(r.Context(), "memory: ingest failed",
			slog.String("error", err.Error()),
		)
		writeError(w, err)
		return
	}

	slog.InfoContext(r.Context(), "memory: ingested",
		slog.String("id", result.ID.String()),
		slog.String("type", string(cmd.Type)),
	)
	writeJSON(w, http.StatusCreated, ingestResponse{
		ID:        result.ID.String(),
		CreatedAt: result.CreatedAt,
	})
}

// Get handles GET /api/v1/memories/{id}.
func (h *MemoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := shared.RecordIDFromString(idStr)
	if err != nil {
		writeError(w, err)
		return
	}

	record, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toMemoryResponse(record))
}

// GetByTopicKey handles GET /api/v1/memories/by-topic-key.
//
// Required query params: project_id, topic_key.
// Optional scope refinements: tenant_id, repo_id, agent_id, session_id, environment.
//
// Returns the active memory record matching the criteria, or 404 when no
// active record matches. Cross-project requests masquerade as 404 (ADR-0005
// §P1.5 existence leak prevention). Post-migration-004 the
// (project_id, tenant_id, topic_key) tuple is unique among active rows.
func (h *MemoryHandler) GetByTopicKey(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	projectID := q.Get("project_id")
	topicKey := q.Get("topic_key")
	if projectID == "" || topicKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "project_id and topic_key are required query params",
		})
		return
	}

	query := inbound.GetByTopicKeyQuery{
		ProjectID: projectID,
		TopicKey:  topicKey,
	}
	if v := q.Get("tenant_id"); v != "" {
		query.TenantID = &v
	}
	if v := q.Get("repo_id"); v != "" {
		query.RepoID = &v
	}
	if v := q.Get("agent_id"); v != "" {
		query.AgentID = &v
	}
	if v := q.Get("session_id"); v != "" {
		query.SessionID = &v
	}
	if v := q.Get("environment"); v != "" {
		query.Environment = &v
	}

	record, err := h.svc.GetByTopicKey(r.Context(), query)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toMemoryResponse(record))
}

// Archive handles POST /api/v1/memories/{id}/archive.
func (h *MemoryHandler) Archive(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := shared.RecordIDFromString(idStr)
	if err != nil {
		writeError(w, err)
		return
	}

	var req archiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cmd := inbound.ArchiveMemoryCmd{
		ID:          id,
		Reason:      req.Reason,
		RequestedBy: req.RequestedBy,
	}

	if err := h.svc.Archive(r.Context(), cmd); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}
