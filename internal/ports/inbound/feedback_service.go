package inbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// SubmitFeedbackCmd carries the data needed to submit retrieval feedback.
type SubmitFeedbackCmd struct {
	Target      shared.FeedbackTarget
	Type        shared.FeedbackType
	Query       string
	Scope       shared.Scope
	ResultIDs   []shared.RecordID
	SelectedIDs []shared.RecordID
	Actor       string
	Metadata    map[string]any
}

// FeedbackService defines the inbound port for retrieval feedback telemetry.
type FeedbackService interface {
	Submit(ctx context.Context, cmd SubmitFeedbackCmd) (*shared.RetrievalFeedback, error)
}
