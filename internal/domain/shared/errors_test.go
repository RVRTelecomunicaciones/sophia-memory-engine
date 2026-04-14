package shared_test

import (
	"errors"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

func TestErrNotFound_IsDistinctFromErrPurged(t *testing.T) {
	if errors.Is(shared.ErrNotFound, shared.ErrPurged) {
		t.Fatal("ErrNotFound should not match ErrPurged")
	}
	if errors.Is(shared.ErrPurged, shared.ErrNotFound) {
		t.Fatal("ErrPurged should not match ErrNotFound")
	}
}

func TestValidationError_ContainsFieldErrors(t *testing.T) {
	err := shared.NewValidationError(
		shared.FieldError{Field: "title", Message: "required", Value: ""},
		shared.FieldError{Field: "type", Message: "invalid", Value: "unknown"},
	)

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("expected error to be *ValidationError")
	}

	if len(ve.Fields) != 2 {
		t.Fatalf("expected 2 field errors, got %d", len(ve.Fields))
	}

	if ve.Fields[0].Field != "title" {
		t.Errorf("expected first field to be 'title', got %q", ve.Fields[0].Field)
	}
	if ve.Fields[1].Field != "type" {
		t.Errorf("expected second field to be 'type', got %q", ve.Fields[1].Field)
	}

	// ValidationError must match ErrValidation via errors.Is
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatal("ValidationError should match ErrValidation via errors.Is")
	}

	// ValidationError must NOT match unrelated sentinels
	if errors.Is(err, shared.ErrNotFound) {
		t.Fatal("ValidationError should not match ErrNotFound")
	}
}

func TestValidationError_ErrorMessage(t *testing.T) {
	err := shared.NewValidationError(
		shared.FieldError{Field: "content", Message: "must not be empty", Value: nil},
	)

	msg := err.Error()

	if msg == "" {
		t.Fatal("error message should not be empty")
	}

	// Must mention "validation error"
	if !contains(msg, "validation error") {
		t.Errorf("expected message to contain 'validation error', got %q", msg)
	}

	// Must mention the field name
	if !contains(msg, "content") {
		t.Errorf("expected message to contain field name 'content', got %q", msg)
	}

	// Must mention the field message
	if !contains(msg, "must not be empty") {
		t.Errorf("expected message to contain field message 'must not be empty', got %q", msg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
