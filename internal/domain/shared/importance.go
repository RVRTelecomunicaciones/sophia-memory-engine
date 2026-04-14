package shared

import "time"

// ImportanceFactor represents a named weighted factor contributing to an importance score.
type ImportanceFactor struct {
	Name   string
	Weight float64
	Value  float64
}

// ImportanceScore represents a computed importance rating with its contributing factors.
type ImportanceScore struct {
	Score      float64
	ComputedAt time.Time
	Factors    []ImportanceFactor
}

// NewImportanceScore creates an ImportanceScore, failing if score is outside [0.0, 1.0].
func NewImportanceScore(score float64, computedAt time.Time, factors []ImportanceFactor) (ImportanceScore, error) {
	if score < 0.0 || score > 1.0 {
		return ImportanceScore{}, NewValidationError(FieldError{
			Field:   "score",
			Message: "must be between 0.0 and 1.0",
			Value:   score,
		})
	}

	return ImportanceScore{
		Score:      score,
		ComputedAt: computedAt,
		Factors:    factors,
	}, nil
}

// DefaultImportanceScore returns a neutral importance score (0.5) with a single default factor.
func DefaultImportanceScore(now time.Time) ImportanceScore {
	return ImportanceScore{
		Score:      0.5,
		ComputedAt: now,
		Factors: []ImportanceFactor{
			{Name: "default", Weight: 1.0, Value: 0.5},
		},
	}
}
