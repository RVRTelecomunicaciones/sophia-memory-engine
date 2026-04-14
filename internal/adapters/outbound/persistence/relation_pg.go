package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that RelationPgRepository implements RelationRepository.
var _ outbound.RelationRepository = (*RelationPgRepository)(nil)

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

// FindFromSource is a stub — full implementation in Lote 6.
func (r *RelationPgRepository) FindFromSource(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, errors.New("RelationPgRepository.FindFromSource: not yet implemented")
}

// FindToTarget is a stub — full implementation in Lote 6.
func (r *RelationPgRepository) FindToTarget(_ context.Context, _ shared.RecordID, _ *shared.RelationType) ([]relation.Relation, error) {
	return nil, errors.New("RelationPgRepository.FindToTarget: not yet implemented")
}

// Traverse is a stub — full implementation in Lote 6.
func (r *RelationPgRepository) Traverse(_ context.Context, _ outbound.TraverseQuery) ([]outbound.TraverseResult, error) {
	return nil, errors.New("RelationPgRepository.Traverse: not yet implemented")
}

// DeleteByTarget is a stub — full implementation in Lote 6.
func (r *RelationPgRepository) DeleteByTarget(_ context.Context, _ shared.RecordID) (int, error) {
	return 0, errors.New("RelationPgRepository.DeleteByTarget: not yet implemented")
}
