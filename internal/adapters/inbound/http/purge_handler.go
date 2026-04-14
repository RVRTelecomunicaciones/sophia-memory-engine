package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// PurgeHandler handles HTTP requests for security purge operations.
type PurgeHandler struct {
	svc inbound.PurgeService
}

// NewPurgeHandler creates a new PurgeHandler.
func NewPurgeHandler(svc inbound.PurgeService) *PurgeHandler {
	return &PurgeHandler{svc: svc}
}

// --- Request DTOs ---

type purgeRequest struct {
	TargetID    string `json:"target_id"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	AuditNote   string `json:"audit_note"`
}

// --- Response DTOs ---

type purgeRespDTO struct {
	ID              string              `json:"id"`
	TargetID        string              `json:"target_id"`
	TargetType      string              `json:"target_type"`
	Reason          string              `json:"reason"`
	RequestedBy     string              `json:"requested_by"`
	AuditNote       string              `json:"audit_note"`
	Status          string              `json:"status"`
	ArtifactsPurged *purgedArtifactsDTO `json:"artifacts_purged,omitempty"`
	ExecutedAt      *time.Time          `json:"executed_at,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
}

type purgedArtifactsDTO struct {
	FTSInvalidated       bool     `json:"fts_invalidated"`
	EmbeddingInvalidated bool     `json:"embedding_invalidated"`
	CacheInvalidated     bool     `json:"cache_invalidated"`
	RelationsRemoved     int      `json:"relations_removed"`
	DerivedInvalidated   []string `json:"derived_invalidated"`
}

func toPurgeResponse(rec *purge.PurgeRecord) purgeRespDTO {
	resp := purgeRespDTO{
		ID:          rec.ID.String(),
		TargetID:    rec.TargetID.String(),
		TargetType:  rec.TargetType,
		Reason:      string(rec.Reason),
		RequestedBy: rec.RequestedBy,
		AuditNote:   rec.AuditNote,
		Status:      string(rec.Status),
		ExecutedAt:  rec.ExecutedAt,
		CreatedAt:   rec.CreatedAt,
	}

	if rec.Status == shared.PurgeStatusExecuted || rec.ArtifactsPurged.FTSInvalidated || rec.ArtifactsPurged.RelationsRemoved > 0 {
		derived := make([]string, 0, len(rec.ArtifactsPurged.DerivedInvalidated))
		for _, id := range rec.ArtifactsPurged.DerivedInvalidated {
			derived = append(derived, id.String())
		}
		resp.ArtifactsPurged = &purgedArtifactsDTO{
			FTSInvalidated:       rec.ArtifactsPurged.FTSInvalidated,
			EmbeddingInvalidated: rec.ArtifactsPurged.EmbeddingInvalidated,
			CacheInvalidated:     rec.ArtifactsPurged.CacheInvalidated,
			RelationsRemoved:     rec.ArtifactsPurged.RelationsRemoved,
			DerivedInvalidated:   derived,
		}
	}

	return resp
}

// Request handles POST /api/v1/purge/request.
func (h *PurgeHandler) Request(w http.ResponseWriter, r *http.Request) {
	var req purgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	targetID, err := shared.RecordIDFromString(req.TargetID)
	if err != nil {
		writeError(w, err)
		return
	}

	cmd := inbound.RequestPurgeCmd{
		TargetID:    targetID,
		Reason:      shared.PurgeReason(req.Reason),
		RequestedBy: req.RequestedBy,
		AuditNote:   req.AuditNote,
	}

	result, err := h.svc.Request(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toPurgeResponse(result))
}

// Execute handles POST /api/v1/purge/{id}/execute.
func (h *PurgeHandler) Execute(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	purgeID, err := shared.RecordIDFromString(idStr)
	if err != nil {
		writeError(w, err)
		return
	}

	cmd := inbound.ExecutePurgeCmd{
		PurgeID: purgeID,
	}

	result, err := h.svc.Execute(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toPurgeResponse(result))
}
