package shared

// Provenance tracks the origin and lineage of a memory record.
type Provenance struct {
	Source    string
	SourceURI *string
	Method    IngestMethod
	ParentID  *RecordID
}

// NewProvenance creates a Provenance with validated fields.
// source is required, method must be valid, and parentID is required when method is derived.
func NewProvenance(source string, method IngestMethod, parentID *RecordID) (Provenance, error) {
	var errs []FieldError

	if source == "" {
		errs = append(errs, FieldError{
			Field:   "source",
			Message: "required",
			Value:   source,
		})
	}

	if !method.IsValid() {
		errs = append(errs, FieldError{
			Field:   "method",
			Message: "invalid ingest method",
			Value:   method,
		})
	}

	if method == IngestMethodDerived && parentID == nil {
		errs = append(errs, FieldError{
			Field:   "parent_id",
			Message: "required when method is derived",
			Value:   nil,
		})
	}

	if len(errs) > 0 {
		return Provenance{}, NewValidationError(errs...)
	}

	return Provenance{
		Source:   source,
		Method:   method,
		ParentID: parentID,
	}, nil
}

// WithSourceURI returns a copy of the Provenance with the given URI set.
func (p Provenance) WithSourceURI(uri string) Provenance {
	p.SourceURI = &uri
	return p
}
