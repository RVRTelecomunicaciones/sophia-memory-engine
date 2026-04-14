package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// SearchQuery defines the parameters for a hybrid search operation.
type SearchQuery struct {
	Query     string
	Scope     shared.Scope
	Types     []string
	Freshness *shared.FreshnessLevel
	TimeRange *shared.TimeRange
	MinScore  *float64
	Limit     *int
	Offset    *int
}

// SearchResults is the paginated outcome of a search operation.
type SearchResults struct {
	Results    []SearchResult
	TotalCount int
	Query      string
	Scope      shared.Scope
}

// SearchResult is a single scored item returned by search.
type SearchResult struct {
	ID         shared.RecordID
	RecordType string
	Title      string
	Snippet    string
	Score      float64
	Ranking    RankingExplanation
	Scope      shared.Scope
	Freshness  shared.FreshnessLevel
	CreatedAt  time.Time
}

// RankingExplanation breaks down how the final score was computed.
type RankingExplanation struct {
	FTSScore        float64
	TRGMScore       float64
	RecencyBoost    float64
	ImportanceScore float64
	TypeBoost       float64
	FreshnessBoost  float64
	ScopeExactness  float64
	FinalScore      float64
}

// ContextRequest defines the parameters for building an agent context bundle.
type ContextRequest struct {
	Scope        shared.Scope
	Query        *string
	MaxTokens    *int
	IncludeTypes []string
	ExpandGraph  *bool
}

// ContextBundle is the assembled context delivered to an agent.
type ContextBundle struct {
	Sections    []ContextSection
	TotalTokens int
	Truncated   bool
	GeneratedAt time.Time
	ScopeUsed   shared.Scope
	DebugInfo   *ContextDebugInfo
}

// ContextSection groups records by type within the bundle.
type ContextSection struct {
	Type       string
	Records    []ContextRecord
	TokenCount int
}

// ContextRecord is a single record included in a context section.
type ContextRecord struct {
	ID       shared.RecordID
	Type     string
	Content  string
	Score    float64
	Relation *string
}

// ContextDebugInfo provides observability data for context building.
type ContextDebugInfo struct {
	Strategy        string
	RecordsScanned  int
	RecordsIncluded int
	GraphExpanded   bool
	GraphDepth      int
	TokenBudgetUsed int
	DurationMs      int64
}

// RetrievalService defines the inbound port for search and context assembly.
type RetrievalService interface {
	Search(ctx context.Context, query SearchQuery) (*SearchResults, error)
	BuildContext(ctx context.Context, req ContextRequest) (*ContextBundle, error)
}
