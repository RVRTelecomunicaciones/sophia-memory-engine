package shared

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for the memory engine domain.
var (
	ErrNotFound          = errors.New("not found")
	ErrPurged            = errors.New("purged")
	ErrValidation        = errors.New("validation error")
	ErrAlreadyArchived   = errors.New("already archived")
	ErrNotActive         = errors.New("not active")
	ErrConflict          = errors.New("conflict")
	ErrDuplicateRelation = errors.New("duplicate relation")
	ErrSourceNotFound    = errors.New("source not found")
	ErrTargetNotFound    = errors.New("target not found")
	ErrSourcePurged      = errors.New("source purged")
	ErrTargetPurged      = errors.New("target purged")
	ErrParentNotFound    = errors.New("parent not found")
	ErrEvidenceRefNotFound = errors.New("evidence reference not found")
	ErrAlreadyExecuted   = errors.New("already executed")
	// ErrScopeForbidden is returned when a request's scope does not match the
	// authenticated scope. Maps to HTTP 403. The response body must never echo
	// which field mismatched — doing so leaks information.
	ErrScopeForbidden    = errors.New("forbidden")
)

// FieldError describes a validation failure for a specific field.
type FieldError struct {
	Field   string
	Message string
	Value   any
}

// ValidationError aggregates one or more field-level validation failures.
type ValidationError struct {
	Fields []FieldError
}

// Error returns a human-readable description of all field errors.
func (v *ValidationError) Error() string {
	if len(v.Fields) == 0 {
		return "validation error"
	}

	msgs := make([]string, 0, len(v.Fields))
	for _, f := range v.Fields {
		msgs = append(msgs, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}

	return fmt.Sprintf("validation error: %s", strings.Join(msgs, "; "))
}

// Is reports whether the target matches ErrValidation.
func (v *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// Unwrap returns ErrValidation so errors.Is works up the chain.
func (v *ValidationError) Unwrap() error {
	return ErrValidation
}

// NewValidationError creates a ValidationError from one or more FieldErrors.
func NewValidationError(fields ...FieldError) error {
	return &ValidationError{Fields: fields}
}
