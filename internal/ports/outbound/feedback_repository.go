package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// FeedbackRepository defines the outbound port for persisting retrieval feedback.
type FeedbackRepository interface {
	Save(ctx context.Context, feedback *shared.RetrievalFeedback) error
}
