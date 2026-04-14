package shared

// Confidence represents a validated confidence score with its source.
type Confidence struct {
	Score  float64
	Source ConfidenceSource
}

// NewConfidence creates a Confidence, failing if score is outside [0.0, 1.0] or source is invalid.
func NewConfidence(score float64, source ConfidenceSource) (Confidence, error) {
	var errs []FieldError

	if score < 0.0 || score > 1.0 {
		errs = append(errs, FieldError{
			Field:   "score",
			Message: "must be between 0.0 and 1.0",
			Value:   score,
		})
	}

	if !source.IsValid() {
		errs = append(errs, FieldError{
			Field:   "source",
			Message: "invalid confidence source",
			Value:   source,
		})
	}

	if len(errs) > 0 {
		return Confidence{}, NewValidationError(errs...)
	}

	return Confidence{Score: score, Source: source}, nil
}
