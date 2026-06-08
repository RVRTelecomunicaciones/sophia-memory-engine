package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that PostgresFTSIndex implements SearchIndex.
var _ outbound.SearchIndex = (*PostgresFTSIndex)(nil)

// PostgresFTSIndex is the PostgreSQL full-text search implementation of the SearchIndex port.
type PostgresFTSIndex struct {
	pool *pgxpool.Pool
}

// NewPostgresFTSIndex creates a new PostgresFTSIndex backed by the given connection pool.
func NewPostgresFTSIndex(pool *pgxpool.Pool) *PostgresFTSIndex {
	return &PostgresFTSIndex{pool: pool}
}

// Index is a no-op in phase 1. PostgreSQL triggers maintain the search_vector
// column automatically via a trigger on INSERT/UPDATE, so explicit indexing
// from the application layer is not required.
func (idx *PostgresFTSIndex) Index(_ context.Context, _ outbound.SearchEntry) error {
	return nil
}

// Remove nulls out the search_vector for a given record. Used by the purge flow
// to ensure purged content is no longer discoverable via FTS.
func (idx *PostgresFTSIndex) Remove(ctx context.Context, id shared.RecordID) error {
	const query = `UPDATE memories SET search_vector = NULL WHERE id = $1`
	_, err := idx.pool.Exec(ctx, query, id.String())
	return err
}

// Search performs a full-text search against the memories table using PostgreSQL
// ts_rank, plainto_tsquery, and pg_trgm similarity as a fallback.
// Phase 1 only searches the memories table (decisions and heuristics will be
// added in the search service layer via UNION ALL).
func (idx *PostgresFTSIndex) Search(ctx context.Context, query outbound.FTSQuery) ([]outbound.FTSResult, error) {
	// Default similarity threshold for trigram fallback.
	const similarityThreshold = 0.3

	args := []any{query.Text, query.Scope.ProjectID, similarityThreshold}
	paramIdx := 4 // next parameter index

	var filters []string

	// Scope filters: only add when the filter field is non-nil.
	if query.Scope.TenantID != nil {
		filters = append(filters, fmt.Sprintf("AND tenant_id = $%d", paramIdx))
		args = append(args, *query.Scope.TenantID)
		paramIdx++
	}
	if query.Scope.RepoID != nil {
		filters = append(filters, fmt.Sprintf("AND repo_id = $%d", paramIdx))
		args = append(args, *query.Scope.RepoID)
		paramIdx++
	}
	if query.Scope.AgentID != nil {
		filters = append(filters, fmt.Sprintf("AND agent_id = $%d", paramIdx))
		args = append(args, *query.Scope.AgentID)
		paramIdx++
	}
	if query.Scope.SessionID != nil {
		filters = append(filters, fmt.Sprintf("AND session_id = $%d", paramIdx))
		args = append(args, *query.Scope.SessionID)
		paramIdx++
	}
	if query.Scope.Environment != nil {
		filters = append(filters, fmt.Sprintf("AND environment = $%d", paramIdx))
		args = append(args, *query.Scope.Environment)
		paramIdx++
	}

	// Type filter.
	if len(query.Types) > 0 {
		filters = append(filters, fmt.Sprintf("AND type = ANY($%d)", paramIdx))
		args = append(args, query.Types)
		paramIdx++
	}

	// Time range filter on created_at.
	if query.TimeRange != nil {
		filters = append(filters, fmt.Sprintf("AND created_at >= $%d AND created_at <= $%d", paramIdx, paramIdx+1))
		args = append(args, query.TimeRange.From, query.TimeRange.To)
		paramIdx += 2
	}

	// Limit and offset with sensible defaults.
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}

	filters = append(filters, fmt.Sprintf("ORDER BY rank DESC LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1))
	args = append(args, limit, offset)

	sql := fmt.Sprintf(`
		SELECT id, 'memory' AS record_type,
			ts_rank(search_vector, plainto_tsquery('simple', $1)) AS rank,
			similarity(content, $1) AS trgm_score,
			ts_headline('simple', content, plainto_tsquery('simple', $1),
				'StartSel=**,StopSel=**,MaxWords=35,MinWords=15') AS snippet
		FROM memories
		WHERE project_id = $2
			AND status = 'active'
			AND (search_vector @@ plainto_tsquery('simple', $1)
				OR similarity(content, $1) > $3)
			%s`,
		strings.Join(filters, "\n\t\t\t"))

	rows, err := idx.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []outbound.FTSResult
	for rows.Next() {
		var (
			idStr        string
			recordType   string
			rank         float64
			trigramScore float64
			snippet      string
		)

		if err := rows.Scan(&idStr, &recordType, &rank, &trigramScore, &snippet); err != nil {
			return nil, err
		}

		recordID, err := shared.RecordIDFromString(idStr)
		if err != nil {
			return nil, err
		}

		results = append(results, outbound.FTSResult{
			ID:           recordID,
			RecordType:   recordType,
			Rank:         rank,
			TrigramScore: trigramScore,
			Snippet:      snippet,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
