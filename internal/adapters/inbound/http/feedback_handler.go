package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// FeedbackHandler handles HTTP requests for retrieval feedback telemetry.
type FeedbackHandler struct {
	svc inbound.FeedbackService
}

// NewFeedbackHandler creates a new FeedbackHandler.
func NewFeedbackHandler(svc inbound.FeedbackService) *FeedbackHandler {
	return &FeedbackHandler{svc: svc}
}

// --- Request DTOs ---

type feedbackRequest struct {
	Target      string         `json:"target"`
	Type        string         `json:"type"`
	Query       string         `json:"query"`
	Scope       scopeDTO       `json:"scope"`
	ResultIDs   []string       `json:"result_ids"`
	SelectedIDs []string       `json:"selected_ids"`
	Actor       string         `json:"actor"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// --- Response DTOs ---

type feedbackRespDTO struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// Submit handles POST /api/v1/feedback.
func (h *FeedbackHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	scope, err := parseScopeDTO(req.Scope)
	if err != nil {
		writeError(w, err)
		return
	}

	resultIDs, err := parseRecordIDs(req.ResultIDs)
	if err != nil {
		writeError(w, err)
		return
	}

	selectedIDs, err := parseRecordIDs(req.SelectedIDs)
	if err != nil {
		writeError(w, err)
		return
	}

	cmd := inbound.SubmitFeedbackCmd{
		Target:      shared.FeedbackTarget(req.Target),
		Type:        shared.FeedbackType(req.Type),
		Query:       req.Query,
		Scope:       scope,
		ResultIDs:   resultIDs,
		SelectedIDs: selectedIDs,
		Actor:       req.Actor,
		Metadata:    req.Metadata,
	}

	result, err := h.svc.Submit(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, feedbackRespDTO{
		ID:        result.ID.String(),
		CreatedAt: result.CreatedAt,
	})
}

// parseRecordIDs converts a slice of strings to a slice of RecordID.
func parseRecordIDs(ids []string) ([]shared.RecordID, error) {
	out := make([]shared.RecordID, 0, len(ids))
	for _, s := range ids {
		id, err := shared.RecordIDFromString(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
