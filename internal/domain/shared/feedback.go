package shared

import "time"

// FeedbackType categorizes retrieval feedback.
type FeedbackType string

const (
	FeedbackTypeHelpful    FeedbackType = "helpful"
	FeedbackTypeIrrelevant FeedbackType = "irrelevant"
)

// IsValid returns true if the FeedbackType is a known value.
func (f FeedbackType) IsValid() bool {
	return f == FeedbackTypeHelpful || f == FeedbackTypeIrrelevant
}

// FeedbackTarget indicates what was evaluated.
type FeedbackTarget string

const (
	FeedbackTargetMemory  FeedbackTarget = "memory"
	FeedbackTargetContext FeedbackTarget = "context"
)

// IsValid returns true if the FeedbackTarget is a known value.
func (f FeedbackTarget) IsValid() bool {
	return f == FeedbackTargetMemory || f == FeedbackTargetContext
}

// RetrievalFeedback records an agent's or user's evaluation of retrieval results.
type RetrievalFeedback struct {
	ID           RecordID
	Target       FeedbackTarget
	FeedbackType FeedbackType
	Query        string         // the query that produced the results
	Scope        Scope
	ResultIDs    []RecordID     // IDs that were returned
	SelectedIDs  []RecordID     // IDs that were actually used/referenced
	Actor        string         // who gave the feedback (e.g. "agent:coder-1")
	Metadata     map[string]any // optional extra context
	CreatedAt    time.Time
}

// NewRetrievalFeedback creates a validated RetrievalFeedback.
func NewRetrievalFeedback(
	target FeedbackTarget,
	feedbackType FeedbackType,
	query string,
	scope Scope,
	resultIDs, selectedIDs []RecordID,
	actor string,
	clock Clock,
) (*RetrievalFeedback, error) {
	var errs []FieldError

	if !target.IsValid() {
		errs = append(errs, FieldError{Field: "target", Message: "invalid feedback target", Value: string(target)})
	}
	if !feedbackType.IsValid() {
		errs = append(errs, FieldError{Field: "feedback_type", Message: "invalid feedback type", Value: string(feedbackType)})
	}
	if actor == "" {
		errs = append(errs, FieldError{Field: "actor", Message: "required", Value: actor})
	}

	if len(errs) > 0 {
		return nil, NewValidationError(errs...)
	}

	return &RetrievalFeedback{
		ID:           NewRecordID(),
		Target:       target,
		FeedbackType: feedbackType,
		Query:        query,
		Scope:        scope,
		ResultIDs:    resultIDs,
		SelectedIDs:  selectedIDs,
		Actor:        actor,
		Metadata:     nil,
		CreatedAt:    clock.Now(),
	}, nil
}
