package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, err error) {
	if ve, ok := errors.AsType[*shared.ValidationError](err); ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "validation_error", "fields": ve.Fields,
		})
		return
	}

	switch {
	case errors.Is(err, shared.ErrScopeForbidden):
		// Cross-project write attempt detected at the application layer.
		// The body intentionally omits which field mismatched to prevent
		// information leakage (OWASP A01:2021 — Broken Access Control).
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, shared.ErrNotFound):
		// Cross-project reads return ErrNotFound (not 403) to prevent
		// existence leaks (ADR-0005 §P1.5 — locked decision 3).
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrPurged):
		writeJSON(w, http.StatusGone, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrAlreadyArchived):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrNotActive):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrDuplicateRelation):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrAlreadyExecuted):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
