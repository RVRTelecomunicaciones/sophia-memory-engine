package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that MemoryPgRepository implements MemoryRepository.
var _ outbound.MemoryRepository = (*MemoryPgRepository)(nil)

// MemoryPgRepository is the PostgreSQL implementation of the MemoryRepository port.
type MemoryPgRepository struct {
	pool *pgxpool.Pool
}

// NewMemoryPgRepository creates a new MemoryPgRepository backed by the given connection pool.
func NewMemoryPgRepository(pool *pgxpool.Pool) *MemoryPgRepository {
	return &MemoryPgRepository{pool: pool}
}

// Save inserts a MemoryRecord into the memories table.
func (r *MemoryPgRepository) Save(ctx context.Context, record *memory.MemoryRecord) error {
	conn := getConn(ctx, r.pool)

	factorsJSON, err := json.Marshal(record.Importance.Factors)
	if err != nil {
		return err
	}

	var parentID any
	if record.Provenance.ParentID != nil {
		parentID = record.Provenance.ParentID.String()
	}

	const query = `
		INSERT INTO memories (
			id, type, content, summary, tags, topic_key, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method, parent_id,
			valid_from, valid_until, last_accessed, freshness,
			importance_score, importance_computed_at, importance_factors,
			status, archived_by, archive_reason,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7::regconfig,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19, $20, $21,
			$22, $23, $24,
			$25, $26, $27,
			$28, $29
		)`

	_, err = conn.Exec(ctx, query,
		record.ID.String(),                       // $1
		string(record.Type),                      // $2
		record.Content,                           // $3
		ptrStr(record.Summary),                   // $4
		record.Tags,                              // $5
		ptrStr(record.TopicKey),                  // $6
		record.FTSLanguage,                       // $7
		ptrStr(record.Scope.TenantID),            // $8
		record.Scope.ProjectID,                   // $9
		ptrStr(record.Scope.RepoID),              // $10
		ptrStr(record.Scope.AgentID),             // $11
		ptrStr(record.Scope.SessionID),           // $12
		ptrStr(record.Scope.Environment),         // $13
		record.Provenance.Source,                  // $14
		ptrStr(record.Provenance.SourceURI),      // $15
		string(record.Provenance.Method),         // $16
		parentID,                                 // $17
		ptrTime(record.Temporal.ValidFrom),       // $18
		ptrTime(record.Temporal.ValidUntil),      // $19
		ptrTime(record.Temporal.LastAccessed),    // $20
		string(record.Temporal.Freshness),        // $21
		record.Importance.Score,                  // $22
		record.Importance.ComputedAt,             // $23
		factorsJSON,                              // $24
		string(record.Status),                    // $25
		ptrStr(record.ArchivedBy),                // $26
		ptrStr(record.ArchiveReason),             // $27
		record.CreatedAt,                         // $28
		record.UpdatedAt,                         // $29
	)

	if err != nil {
		return fmt.Errorf("memory_pg: save: %w", err)
	}
	return nil
}

// UpsertByTopicKey inserts a new memory record, or — when an active row with
// the same (project_id, tenant_id, topic_key) already exists — updates that
// row's content, summary, tags, provenance and temporal fields in place. The
// returned id is the existing row's id on update; `inserted` is true iff a
// new row was created.
//
// Concurrency: two concurrent calls with the same (scope, topic_key) converge
// on exactly ONE active row thanks to the partial unique index
// idx_memories_topic_key_active_unique (migration 004 — ADR-0005 P1.3). Both
// callers receive the same id; the winner of the race decides the final
// content (last-write-wins).
//
// Preconditions:
//   - record.TopicKey MUST be non-nil and non-empty. The caller MUST branch
//     on TopicKey presence and use Save for the no-topic-key path.
//   - Application-layer scope assertion (P1.5) MUST have been performed.
//
// Fields preserved on UPDATE: id (from existing row), created_at, tenant_id,
// project_id, repo_id, agent_id, session_id, environment.
// Fields overwritten on UPDATE: content, summary, tags, fts_language,
// valid_from, valid_until, last_accessed, freshness ('fresh'), source,
// source_uri, ingest_method, parent_id, importance, status ('active'),
// archived_by (NULL), archive_reason (NULL), updated_at (NOW()).
func (r *MemoryPgRepository) UpsertByTopicKey(
	ctx context.Context,
	record *memory.MemoryRecord,
) (shared.RecordID, bool, error) {
	if record.TopicKey == nil || *record.TopicKey == "" {
		return shared.RecordID{}, false, fmt.Errorf("memory_pg: upsert_by_topic_key: %w", &shared.ValidationError{Fields: []shared.FieldError{{Field: "topic_key", Message: "required for upsert"}}})
	}

	conn := getConn(ctx, r.pool)

	factorsJSON, err := json.Marshal(record.Importance.Factors)
	if err != nil {
		return shared.RecordID{}, false, fmt.Errorf("memory_pg: marshal importance factors: %w", err)
	}

	var parentID any
	if record.Provenance.ParentID != nil {
		parentID = record.Provenance.ParentID.String()
	}

	// ON CONFLICT targets the partial unique index columns. PostgreSQL
	// matches a partial unique index when the conflict target's column list
	// + predicate are a (strict) subset of the index's, so we restate the
	// WHERE clause here. We also exclude rows that are NOT active from
	// updates as an extra safety net — the partial index already filters
	// archived/purged rows but the explicit predicate documents intent.
	const query = `
		INSERT INTO memories (
			id, type, content, summary, tags, topic_key, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method, parent_id,
			valid_from, valid_until, last_accessed, freshness,
			importance_score, importance_computed_at, importance_factors,
			status, archived_by, archive_reason,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7::regconfig,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19, $20, $21,
			$22, $23, $24,
			$25, $26, $27,
			$28, $29
		)
		ON CONFLICT (project_id, COALESCE(tenant_id, ''), topic_key)
			WHERE topic_key IS NOT NULL AND status = 'active'
		DO UPDATE SET
			type                    = EXCLUDED.type,
			content                 = EXCLUDED.content,
			summary                 = EXCLUDED.summary,
			tags                    = EXCLUDED.tags,
			fts_language            = EXCLUDED.fts_language,
			source                  = EXCLUDED.source,
			source_uri              = EXCLUDED.source_uri,
			ingest_method           = EXCLUDED.ingest_method,
			parent_id               = EXCLUDED.parent_id,
			valid_from              = EXCLUDED.valid_from,
			valid_until             = EXCLUDED.valid_until,
			last_accessed           = EXCLUDED.last_accessed,
			freshness               = 'fresh',
			importance_score        = EXCLUDED.importance_score,
			importance_computed_at  = EXCLUDED.importance_computed_at,
			importance_factors      = EXCLUDED.importance_factors,
			status                  = 'active',
			archived_by             = NULL,
			archive_reason          = NULL,
			updated_at              = NOW()
		RETURNING id, (xmax = 0) AS inserted`

	row := conn.QueryRow(ctx, query,
		record.ID.String(),                       // $1
		string(record.Type),                      // $2
		record.Content,                           // $3
		ptrStr(record.Summary),                   // $4
		record.Tags,                              // $5
		ptrStr(record.TopicKey),                  // $6
		record.FTSLanguage,                       // $7
		ptrStr(record.Scope.TenantID),            // $8
		record.Scope.ProjectID,                   // $9
		ptrStr(record.Scope.RepoID),              // $10
		ptrStr(record.Scope.AgentID),             // $11
		ptrStr(record.Scope.SessionID),           // $12
		ptrStr(record.Scope.Environment),         // $13
		record.Provenance.Source,                 // $14
		ptrStr(record.Provenance.SourceURI),      // $15
		string(record.Provenance.Method),         // $16
		parentID,                                 // $17
		ptrTime(record.Temporal.ValidFrom),       // $18
		ptrTime(record.Temporal.ValidUntil),      // $19
		ptrTime(record.Temporal.LastAccessed),    // $20
		string(record.Temporal.Freshness),        // $21
		record.Importance.Score,                  // $22
		record.Importance.ComputedAt,             // $23
		factorsJSON,                              // $24
		string(record.Status),                    // $25
		ptrStr(record.ArchivedBy),                // $26
		ptrStr(record.ArchiveReason),             // $27
		record.CreatedAt,                         // $28
		record.UpdatedAt,                         // $29
	)

	var (
		returnedID string
		inserted   bool
	)
	if err := row.Scan(&returnedID, &inserted); err != nil {
		return shared.RecordID{}, false, fmt.Errorf("memory_pg: upsert_by_topic_key scan: %w", err)
	}

	rid, err := shared.RecordIDFromString(returnedID)
	if err != nil {
		return shared.RecordID{}, false, fmt.Errorf("memory_pg: upsert_by_topic_key parse id: %w", err)
	}
	return rid, inserted, nil
}

// FindActiveByTopicKey retrieves the unique active memory record matching the
// supplied (scope, topic_key). Scope filtering follows the same NULL-tenant
// visibility rule as FindByID: a NULL tenant_id on the row is visible to any
// key in the same project; a non-NULL tenant must match exactly.
func (r *MemoryPgRepository) FindActiveByTopicKey(
	ctx context.Context,
	scope shared.Scope,
	topicKey string,
) (*memory.MemoryRecord, error) {
	conn := getConn(ctx, r.pool)

	const query = `
		SELECT
			id, type, content, summary, tags, topic_key, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method, parent_id,
			valid_from, valid_until, last_accessed, freshness,
			importance_score, importance_computed_at, importance_factors,
			status, archived_by, archive_reason,
			created_at, updated_at
		FROM memories
		WHERE topic_key = $1
		  AND status = 'active'
		  AND project_id = $2
		  AND (tenant_id IS NULL OR tenant_id = $3)
		LIMIT 1`

	row := conn.QueryRow(ctx, query, topicKey, scope.ProjectID, scope.TenantID)

	rec, err := scanMemoryRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("memory_pg: find_active_by_topic_key: %w", err)
	}
	return rec, nil
}

// FindByID retrieves a MemoryRecord by its ID within the given scope.
// Returns shared.ErrNotFound if the record does not exist OR belongs to a
// different project — the two cases are intentionally indistinguishable to
// prevent existence leaks across project boundaries (see ADR-0005 §P1.5).
// Returns shared.ErrPurged if the record has been purged.
//
// Scope: scope is provided by the application layer from the auth context.
// The record's scope is NOT used here; only the project_id and tenant_id from
// the auth context are used as row filters.
func (r *MemoryPgRepository) FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*memory.MemoryRecord, error) {
	conn := getConn(ctx, r.pool)

	// Tenant predicate: NULL tenant on the row is visible to any key in the project
	// (legacy / cross-tenant rows). A non-NULL tenant must match exactly.
	const query = `
		SELECT
			id, type, content, summary, tags, topic_key, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method, parent_id,
			valid_from, valid_until, last_accessed, freshness,
			importance_score, importance_computed_at, importance_factors,
			status, archived_by, archive_reason,
			created_at, updated_at
		FROM memories
		WHERE id = $1
		  AND project_id = $2
		  AND (tenant_id IS NULL OR tenant_id = $3)`

	row := conn.QueryRow(ctx, query, id.String(), scope.ProjectID, scope.TenantID)

	rec, err := scanMemoryRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("memory_pg: find_by_id: %w", err)
	}

	if rec.Status == shared.MemoryStatusPurged {
		return nil, shared.ErrPurged
	}
	return rec, nil
}

// scanMemoryRow scans a single memories row into a MemoryRecord. The caller
// is responsible for handling status-derived semantics (e.g. ErrPurged) and
// for translating pgx.ErrNoRows into the appropriate domain error.
func scanMemoryRow(row pgx.Row) (*memory.MemoryRecord, error) {
	var (
		idStr        string
		memType      string
		content      string
		summary      *string
		tags         []string
		topicKey     *string
		ftsLanguage  string
		tenantID     *string
		projectID    string
		repoID       *string
		agentID      *string
		sessionID    *string
		environment  *string
		source       string
		sourceURI    *string
		ingestMethod string
		parentIDStr  *string
		validFrom    *time.Time
		validUntil   *time.Time
		lastAccessed *time.Time
		freshness    string
		impScore     float64
		impComputed  time.Time
		impFactors   []byte
		status       string
		archivedBy   *string
		archiveRsn   *string
		createdAt    time.Time
		updatedAt    time.Time
	)

	if err := row.Scan(
		&idStr, &memType, &content, &summary, &tags, &topicKey, &ftsLanguage,
		&tenantID, &projectID, &repoID, &agentID, &sessionID, &environment,
		&source, &sourceURI, &ingestMethod, &parentIDStr,
		&validFrom, &validUntil, &lastAccessed, &freshness,
		&impScore, &impComputed, &impFactors,
		&status, &archivedBy, &archiveRsn,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err //nolint:wrapcheck // pgx.ErrNoRows sentinel propagated upstream
	}

	recordID, err := shared.RecordIDFromString(idStr)
	if err != nil {
		return nil, fmt.Errorf("memory_pg: parse id: %w", err)
	}

	var parentID *shared.RecordID
	if parentIDStr != nil {
		pid, err := shared.RecordIDFromString(*parentIDStr)
		if err != nil {
			return nil, fmt.Errorf("memory_pg: parse parent id: %w", err)
		}
		parentID = &pid
	}

	var factors []shared.ImportanceFactor
	if impFactors != nil {
		if err := json.Unmarshal(impFactors, &factors); err != nil {
			return nil, fmt.Errorf("memory_pg: unmarshal importance factors: %w", err)
		}
	}

	return &memory.MemoryRecord{
		ID:       recordID,
		Type:     shared.MemoryType(memType),
		Content:  content,
		Summary:  summary,
		Tags:     tags,
		TopicKey: topicKey,
		Scope: shared.Scope{
			TenantID:    tenantID,
			ProjectID:   projectID,
			RepoID:      repoID,
			AgentID:     agentID,
			SessionID:   sessionID,
			Environment: environment,
		},
		Provenance: shared.Provenance{
			Source:    source,
			SourceURI: sourceURI,
			Method:    shared.IngestMethod(ingestMethod),
			ParentID:  parentID,
		},
		Temporal: shared.TemporalMetadata{
			ValidFrom:    validFrom,
			ValidUntil:   validUntil,
			LastAccessed: lastAccessed,
			Freshness:    shared.FreshnessLevel(freshness),
		},
		Importance: shared.ImportanceScore{
			Score:      impScore,
			ComputedAt: impComputed,
			Factors:    factors,
		},
		Status:        shared.MemoryStatus(status),
		ArchivedBy:    archivedBy,
		ArchiveReason: archiveRsn,
		FTSLanguage:   ftsLanguage,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

// UpdateStatus changes the status of a scoped memory record.
// Returns shared.ErrNotFound if the record does not exist within scope
// (including cross-project access — the two cases are indistinguishable).
//
// Scope: scope is provided by the application layer from the auth context.
func (r *MemoryPgRepository) UpdateStatus(ctx context.Context, scope shared.Scope, id shared.RecordID, status shared.MemoryStatus) error {
	conn := getConn(ctx, r.pool)

	const query = `
		UPDATE memories SET status = $1, updated_at = NOW()
		WHERE id = $2
		  AND project_id = $3
		  AND (tenant_id IS NULL OR tenant_id = $4)`

	tag, err := conn.Exec(ctx, query, string(status), id.String(), scope.ProjectID, scope.TenantID)
	if err != nil {
		return fmt.Errorf("memory_pg: update_status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}

	return nil
}

// WipeContent performs a hard purge on a scoped memory record, clearing sensitive data.
// Returns shared.ErrNotFound if the record does not exist within scope
// (including cross-project access — the two cases are indistinguishable).
//
// Scope: scope is provided by the application layer from the auth context.
// Scope: scope is provided by the application layer from the auth context.
// NOTE: scope comes from the purge record's scope (set when the purge was
// requested), which in turn was validated against the auth context at request
// time. This preserves the audit chain: the purge record's scope is the
// authoritative boundary for the wipe operation.
func (r *MemoryPgRepository) WipeContent(ctx context.Context, scope shared.Scope, id shared.RecordID) error {
	conn := getConn(ctx, r.pool)

	const query = `
		UPDATE memories SET
			content = '', summary = NULL, tags = '{}',
			search_vector = NULL, importance_score = 0,
			importance_factors = NULL, status = 'purged',
			updated_at = NOW()
		WHERE id = $1
		  AND project_id = $2
		  AND (tenant_id IS NULL OR tenant_id = $3)`

	tag, err := conn.Exec(ctx, query, id.String(), scope.ProjectID, scope.TenantID)
	if err != nil {
		return fmt.Errorf("memory_pg: wipe_content: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}

	return nil
}
