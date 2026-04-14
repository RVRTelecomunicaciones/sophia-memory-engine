package projectprofile

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// PatternEntry records a detected pattern with frequency and confidence.
type PatternEntry struct {
	Pattern    string
	Frequency  int
	LastSeenAt time.Time
	Confidence shared.Confidence
}

// SourceCounts tracks how many items of each type contributed to a profile.
type SourceCounts struct {
	EpisodicConsidered   int
	SemanticConsidered   int
	DecisionsConsidered  int
	HeuristicsConsidered int
}

// FreshnessState records when and from what sources a profile was generated.
type FreshnessState struct {
	GeneratedAt     time.Time
	SourceTimeRange shared.TimeRange
	StaleAfter      time.Time
	SourceCounts    SourceCounts
}

// ProjectProfile is a generated snapshot of a project's memory landscape.
type ProjectProfile struct {
	ID              shared.RecordID
	ProjectID       string
	Version         int
	Summary         string
	ActiveDecisions []shared.RecordID
	TopHeuristics   []shared.RecordID
	Patterns        []PatternEntry
	ArchSignals     []string
	Freshness       FreshnessState
	Scope           shared.Scope
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewProjectProfile creates a validated ProjectProfile with sensible defaults.
func NewProjectProfile(
	projectID, summary string,
	scope shared.Scope,
	sourceRange shared.TimeRange,
	clock shared.Clock,
	version int,
) (*ProjectProfile, error) {
	if projectID == "" {
		return nil, shared.NewValidationError(shared.FieldError{
			Field:   "project_id",
			Message: "required",
			Value:   projectID,
		})
	}

	now := clock.Now()

	return &ProjectProfile{
		ID:              shared.NewRecordID(),
		ProjectID:       projectID,
		Version:         version,
		Summary:         summary,
		ActiveDecisions: []shared.RecordID{},
		TopHeuristics:   []shared.RecordID{},
		Patterns:        []PatternEntry{},
		ArchSignals:     []string{},
		Freshness: FreshnessState{
			GeneratedAt:     now,
			SourceTimeRange: sourceRange,
			StaleAfter:      now.Add(24 * time.Hour),
			SourceCounts:    SourceCounts{},
		},
		Scope:     scope,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
