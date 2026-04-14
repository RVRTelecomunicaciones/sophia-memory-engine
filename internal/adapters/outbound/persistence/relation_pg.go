package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that RelationPgRepository implements RelationRepository.
var _ outbound.RelationRepository = (*RelationPgRepository)(nil)

// maxTraverseDepth is the hard ceiling for recursive graph traversal.
const maxTraverseDepth = 3

// RelationPgRepository is the PostgreSQL implementation of the RelationRepository port.
type RelationPgRepository struct {
	pool *pgxpool.Pool
}

// NewRelationPgRepository creates a new RelationPgRepository backed by the given connection pool.
func NewRelationPgRepository(pool *pgxpool.Pool) *RelationPgRepository {
	return &RelationPgRepository{pool: pool}
}

// Save inserts a Relation into the relations table.
func (r *RelationPgRepository) Save(ctx context.Context, rel *relation.Relation) error {
	conn := getConn(ctx, r.pool)

	metadataJSON, err := json.Marshal(rel.Metadata)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO relations (
			id, source_id, target_id, type, metadata,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			valid_from, valid_until, last_accessed, freshness,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15,
			$16, $17
		)`

	_, err = conn.Exec(ctx, query,
		rel.ID.String(),                    // $1
		rel.SourceID.String(),              // $2
		rel.TargetID.String(),              // $3
		string(rel.Type),                   // $4
		metadataJSON,                       // $5
		ptrStr(rel.Scope.TenantID),         // $6
		rel.Scope.ProjectID,                // $7
		ptrStr(rel.Scope.RepoID),           // $8
		ptrStr(rel.Scope.AgentID),          // $9
		ptrStr(rel.Scope.SessionID),        // $10
		ptrStr(rel.Scope.Environment),      // $11
		ptrTime(rel.Temporal.ValidFrom),    // $12
		ptrTime(rel.Temporal.ValidUntil),   // $13
		ptrTime(rel.Temporal.LastAccessed), // $14
		string(rel.Temporal.Freshness),     // $15
		rel.CreatedAt,                      // $16
		rel.UpdatedAt,                      // $17
	)

	return err
}

// scanRelation scans a single row into a relation.Relation.
// The caller must provide a pgx.Row or compatible scanner from a query that
// selects exactly: id, source_id, target_id, relation_type, metadata,
// tenant_id, project_id, repo_id, agent_id, session_id, environment,
// valid_from, valid_until, created_at, updated_at.
func scanRelation(rows pgx.Rows) (relation.Relation, error) {
	var (
		rel         relation.Relation
		id          string
		sourceID    string
		targetID    string
		relType     string
		metadataRaw []byte
		tenantID    *string
		projectID   string
		repoID      *string
		agentID     *string
		sessionID   *string
		environment *string
		validFrom   *time.Time
		validUntil  *time.Time
		createdAt   time.Time
		updatedAt   time.Time
	)

	if err := rows.Scan(
		&id, &sourceID, &targetID, &relType, &metadataRaw,
		&tenantID, &projectID, &repoID, &agentID, &sessionID, &environment,
		&validFrom, &validUntil,
		&createdAt, &updatedAt,
	); err != nil {
		return rel, err
	}

	parsedID, err := shared.RecordIDFromString(id)
	if err != nil {
		return rel, err
	}
	parsedSourceID, err := shared.RecordIDFromString(sourceID)
	if err != nil {
		return rel, err
	}
	parsedTargetID, err := shared.RecordIDFromString(targetID)
	if err != nil {
		return rel, err
	}

	var metadata map[string]any
	if len(metadataRaw) > 0 {
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
			return rel, err
		}
	}

	rel.ID = parsedID
	rel.SourceID = parsedSourceID
	rel.TargetID = parsedTargetID
	rel.Type = shared.RelationType(relType)
	rel.Metadata = metadata
	rel.Scope = shared.Scope{
		TenantID:    tenantID,
		ProjectID:   projectID,
		RepoID:      repoID,
		AgentID:     agentID,
		SessionID:   sessionID,
		Environment: environment,
	}
	rel.Temporal = shared.TemporalMetadata{
		ValidFrom:  validFrom,
		ValidUntil: validUntil,
	}
	rel.CreatedAt = createdAt
	rel.UpdatedAt = updatedAt

	return rel, nil
}

// FindFromSource returns all relations originating from the given source, optionally filtered by type.
func (r *RelationPgRepository) FindFromSource(ctx context.Context, sourceID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error) {
	conn := getConn(ctx, r.pool)

	const query = `
		SELECT id, source_id, target_id, type, metadata,
		       tenant_id, project_id, repo_id, agent_id, session_id, environment,
		       valid_from, valid_until, created_at, updated_at
		FROM relations
		WHERE source_id = $1
		  AND ($2::varchar IS NULL OR type = $2)
		ORDER BY created_at DESC`

	var typeStr *string
	if relType != nil {
		s := string(*relType)
		typeStr = &s
	}

	rows, err := conn.Query(ctx, query, sourceID.String(), typeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []relation.Relation
	for rows.Next() {
		rel, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, rel)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// FindToTarget returns all relations pointing to the given target, optionally filtered by type.
func (r *RelationPgRepository) FindToTarget(ctx context.Context, targetID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error) {
	conn := getConn(ctx, r.pool)

	const query = `
		SELECT id, source_id, target_id, type, metadata,
		       tenant_id, project_id, repo_id, agent_id, session_id, environment,
		       valid_from, valid_until, created_at, updated_at
		FROM relations
		WHERE target_id = $1
		  AND ($2::varchar IS NULL OR type = $2)
		ORDER BY created_at DESC`

	var typeStr *string
	if relType != nil {
		s := string(*relType)
		typeStr = &s
	}

	rows, err := conn.Query(ctx, query, targetID.String(), typeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []relation.Relation
	for rows.Next() {
		rel, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, rel)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// Traverse performs a recursive graph traversal from the given start node.
func (r *RelationPgRepository) Traverse(ctx context.Context, query outbound.TraverseQuery) ([]outbound.TraverseResult, error) {
	conn := getConn(ctx, r.pool)

	// Hard cap depth at 3.
	depth := query.MaxDepth
	if depth > maxTraverseDepth {
		depth = maxTraverseDepth
	}
	if depth < 1 {
		depth = 1
	}

	// Build dynamic SQL with parameter counting.
	paramIdx := 1
	args := make([]any, 0, 8)

	// $1 = start_id
	startParam := fmt.Sprintf("$%d", paramIdx)
	args = append(args, query.StartID.String())
	paramIdx++

	// $2 = project_id (scope is required for traversal)
	projectParam := fmt.Sprintf("$%d", paramIdx)
	var projectID string
	if query.Scope != nil {
		projectID = query.Scope.ProjectID
	}
	args = append(args, projectID)
	paramIdx++

	// $3 = max_depth
	depthParam := fmt.Sprintf("$%d", paramIdx)
	args = append(args, depth)
	paramIdx++

	// Build type filter clause.
	var typeFilter string
	if len(query.Types) > 0 {
		placeholders := make([]string, len(query.Types))
		for i, t := range query.Types {
			placeholders[i] = fmt.Sprintf("$%d", paramIdx)
			args = append(args, string(t))
			paramIdx++
		}
		typeFilter = fmt.Sprintf("AND r.type IN (%s)", strings.Join(placeholders, ", "))
	}

	// Build temporal filter clause.
	var temporalFilter string
	if query.ValidAt != nil {
		validAtParam := fmt.Sprintf("$%d", paramIdx)
		args = append(args, *query.ValidAt)
		paramIdx++
		temporalFilter = fmt.Sprintf(
			"AND (r.valid_from IS NULL OR r.valid_from <= %s) AND (r.valid_until IS NULL OR r.valid_until > %s)",
			validAtParam, validAtParam,
		)
	}

	// Build optional scope filters.
	var scopeFilters string
	if query.Scope != nil {
		if query.Scope.TenantID != nil {
			p := fmt.Sprintf("$%d", paramIdx)
			args = append(args, *query.Scope.TenantID)
			paramIdx++
			scopeFilters += fmt.Sprintf(" AND r.tenant_id = %s", p)
		}
		if query.Scope.RepoID != nil {
			p := fmt.Sprintf("$%d", paramIdx)
			args = append(args, *query.Scope.RepoID)
			paramIdx++
			scopeFilters += fmt.Sprintf(" AND r.repo_id = %s", p)
		}
		if query.Scope.AgentID != nil {
			p := fmt.Sprintf("$%d", paramIdx)
			args = append(args, *query.Scope.AgentID)
			paramIdx++
			scopeFilters += fmt.Sprintf(" AND r.agent_id = %s", p)
		}
		if query.Scope.SessionID != nil {
			p := fmt.Sprintf("$%d", paramIdx)
			args = append(args, *query.Scope.SessionID)
			paramIdx++
			scopeFilters += fmt.Sprintf(" AND r.session_id = %s", p)
		}
		if query.Scope.Environment != nil {
			p := fmt.Sprintf("$%d", paramIdx)
			args = append(args, *query.Scope.Environment)
			paramIdx++
			scopeFilters += fmt.Sprintf(" AND r.environment = %s", p)
		}
	}

	allFilters := typeFilter + " " + temporalFilter + " " + scopeFilters

	// Build the base case and recursive case depending on direction.
	var baseWhere, recursiveJoin, basePath, recursivePath, cyclePrevention string

	switch query.Direction {
	case outbound.TraverseInbound:
		baseWhere = fmt.Sprintf("r.target_id = %s", startParam)
		recursiveJoin = "INNER JOIN graph g ON g.source_id = r.target_id"
		basePath = "ARRAY[r.target_id, r.source_id]::varchar[]"
		recursivePath = "g.path || r.source_id"
		cyclePrevention = "AND NOT r.source_id = ANY(g.path)"
	case outbound.TraverseBoth:
		// For "both" we use two UNIONs in the base case.
		baseWhere = fmt.Sprintf("(r.source_id = %s OR r.target_id = %s)", startParam, startParam)
		// Recursive case: follow edges in both directions.
		recursiveJoin = "INNER JOIN graph g ON (g.target_id = r.source_id OR g.source_id = r.target_id)"
		basePath = "ARRAY[r.source_id, r.target_id]::varchar[]"
		recursivePath = "g.path || CASE WHEN g.target_id = r.source_id THEN r.target_id ELSE r.source_id END"
		cyclePrevention = "AND NOT (CASE WHEN g.target_id = r.source_id THEN r.target_id ELSE r.source_id END) = ANY(g.path)"
	default: // outbound
		baseWhere = fmt.Sprintf("r.source_id = %s", startParam)
		recursiveJoin = "INNER JOIN graph g ON g.target_id = r.source_id"
		basePath = "ARRAY[r.source_id, r.target_id]::varchar[]"
		recursivePath = "g.path || r.target_id"
		cyclePrevention = "AND NOT r.target_id = ANY(g.path)"
	}

	sql := fmt.Sprintf(`
WITH RECURSIVE graph AS (
    SELECT
        r.id, r.source_id, r.target_id, r.type, r.metadata,
        r.tenant_id, r.project_id, r.repo_id, r.agent_id, r.session_id, r.environment,
        r.valid_from, r.valid_until, r.created_at, r.updated_at,
        1 AS depth,
        %s AS path
    FROM relations r
    WHERE %s
      AND r.project_id = %s
      %s

    UNION ALL

    SELECT
        r.id, r.source_id, r.target_id, r.type, r.metadata,
        r.tenant_id, r.project_id, r.repo_id, r.agent_id, r.session_id, r.environment,
        r.valid_from, r.valid_until, r.created_at, r.updated_at,
        g.depth + 1,
        %s
    FROM relations r
    %s
    WHERE g.depth < %s
      %s
      AND r.project_id = %s
      %s
)
SELECT id, source_id, target_id, type, metadata,
       tenant_id, project_id, repo_id, agent_id, session_id, environment,
       valid_from, valid_until, created_at, updated_at,
       depth, path
FROM graph
ORDER BY depth ASC, created_at DESC`,
		basePath, baseWhere, projectParam, allFilters,
		recursivePath, recursiveJoin, depthParam, cyclePrevention, projectParam, allFilters,
	)

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []outbound.TraverseResult
	for rows.Next() {
		var (
			id          string
			sourceID    string
			targetID    string
			relType     string
			metadataRaw []byte
			tenantID    *string
			pID         string
			repoID      *string
			agentID     *string
			sessionID   *string
			environment *string
			validFrom   *time.Time
			validUntil  *time.Time
			createdAt   time.Time
			updatedAt   time.Time
			d           int
			pathRaw     []string
		)

		if err := rows.Scan(
			&id, &sourceID, &targetID, &relType, &metadataRaw,
			&tenantID, &pID, &repoID, &agentID, &sessionID, &environment,
			&validFrom, &validUntil,
			&createdAt, &updatedAt,
			&d, &pathRaw,
		); err != nil {
			return nil, err
		}

		parsedID, err := shared.RecordIDFromString(id)
		if err != nil {
			return nil, err
		}
		parsedSourceID, err := shared.RecordIDFromString(sourceID)
		if err != nil {
			return nil, err
		}
		parsedTargetID, err := shared.RecordIDFromString(targetID)
		if err != nil {
			return nil, err
		}

		var metadata map[string]any
		if len(metadataRaw) > 0 {
			if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
				return nil, err
			}
		}

		path := make([]shared.RecordID, 0, len(pathRaw))
		for _, s := range pathRaw {
			pid, err := shared.RecordIDFromString(s)
			if err != nil {
				return nil, err
			}
			path = append(path, pid)
		}

		results = append(results, outbound.TraverseResult{
			Relation: relation.Relation{
				ID:       parsedID,
				SourceID: parsedSourceID,
				TargetID: parsedTargetID,
				Type:     shared.RelationType(relType),
				Metadata: metadata,
				Scope: shared.Scope{
					TenantID:    tenantID,
					ProjectID:   pID,
					RepoID:      repoID,
					AgentID:     agentID,
					SessionID:   sessionID,
					Environment: environment,
				},
				Temporal: shared.TemporalMetadata{
					ValidFrom:  validFrom,
					ValidUntil: validUntil,
				},
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			Depth: d,
			Path:  path,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// DeleteByTarget removes all relations where the given ID appears as source or target.
func (r *RelationPgRepository) DeleteByTarget(ctx context.Context, targetID shared.RecordID) (int, error) {
	conn := getConn(ctx, r.pool)

	const query = `DELETE FROM relations WHERE source_id = $1 OR target_id = $1`

	tag, err := conn.Exec(ctx, query, targetID.String())
	if err != nil {
		return 0, err
	}

	return int(tag.RowsAffected()), nil
}
