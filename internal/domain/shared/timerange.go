package shared

import "time"

// TimeRange represents a bounded interval of time.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// NewTimeRange creates a TimeRange, failing if From is after To.
func NewTimeRange(from, to time.Time) (TimeRange, error) {
	if from.After(to) {
		return TimeRange{}, NewValidationError(FieldError{
			Field:   "from",
			Message: "must not be after to",
			Value:   from,
		})
	}
	return TimeRange{From: from, To: to}, nil
}

// Contains returns true if t falls within the range [From, To] inclusive.
func (tr TimeRange) Contains(t time.Time) bool {
	return !t.Before(tr.From) && !t.After(tr.To)
}
