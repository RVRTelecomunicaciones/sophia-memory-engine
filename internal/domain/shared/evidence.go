package shared

// EvidenceRef represents a reference to evidence supporting a decision.
type EvidenceRef struct {
	RecordID *RecordID
	URI      *string
	Excerpt  *string
	Type     EvidenceType
}

// NewEvidenceRef creates an EvidenceRef with type-specific validation.
//   - memory_ref → recordID required
//   - external_link → uri required and non-empty
//   - inline_excerpt → excerpt required and non-empty
func NewEvidenceRef(evidenceType EvidenceType, recordID *RecordID, uri *string, excerpt *string) (EvidenceRef, error) {
	if !evidenceType.IsValid() {
		return EvidenceRef{}, NewValidationError(FieldError{
			Field:   "type",
			Message: "invalid evidence type",
			Value:   evidenceType,
		})
	}

	switch evidenceType {
	case EvidenceTypeMemoryRef:
		if recordID == nil {
			return EvidenceRef{}, NewValidationError(FieldError{
				Field:   "record_id",
				Message: "required for memory_ref evidence",
				Value:   nil,
			})
		}
	case EvidenceTypeExternalLink:
		if uri == nil || *uri == "" {
			return EvidenceRef{}, NewValidationError(FieldError{
				Field:   "uri",
				Message: "required and non-empty for external_link evidence",
				Value:   uri,
			})
		}
	case EvidenceTypeInlineExcerpt:
		if excerpt == nil || *excerpt == "" {
			return EvidenceRef{}, NewValidationError(FieldError{
				Field:   "excerpt",
				Message: "required and non-empty for inline_excerpt evidence",
				Value:   excerpt,
			})
		}
	}

	return EvidenceRef{
		RecordID: recordID,
		URI:      uri,
		Excerpt:  excerpt,
		Type:     evidenceType,
	}, nil
}
