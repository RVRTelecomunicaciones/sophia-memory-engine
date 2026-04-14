package outbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// SearchEntry represents a record indexed for full-text search.
// This is a read-model projection, not authoritative storage.
type SearchEntry struct {
	ID         shared.RecordID
	RecordType string
	Content    string
	Title      *string
	Tags       []string
	Scope      shared.Scope
	CreatedAt  time.Time
}

// FTSQuery defines the parameters for a full-text search query.
type FTSQuery struct {
	Text      string
	Scope     shared.Scope
	Types     []string
	TimeRange *shared.TimeRange
	Limit     int
	Offset    int
}

// FTSResult represents a single full-text search hit with relevance ranking.
type FTSResult struct {
	ID         shared.RecordID
	RecordType string
	Rank       float64
	Snippet    string
}

// SearchIndex is a read-model port for full-text search operations.
type SearchIndex interface {
	Index(ctx context.Context, entry SearchEntry) error
	Remove(ctx context.Context, id shared.RecordID) error
	Search(ctx context.Context, query FTSQuery) ([]FTSResult, error)
}
