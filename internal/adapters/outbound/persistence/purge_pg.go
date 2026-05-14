package persistence

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that PurgePgRepository implements PurgeRepository.
var _ outbound.PurgeRepository = (*PurgePgRepository)(nil)

// PurgePgRepository is the PostgreSQL implementation of the PurgeRepository port.
type PurgePgRepository struct {
	pool *pgxpool.Pool
}

// NewPurgePgRepository creates a new PurgePgRepository backed by the given connection pool.
func NewPurgePgRepository(pool *pgxpool.Pool) *PurgePgRepository {
	return &PurgePgRepository{pool: pool}
}

// Save inserts a PurgeRecord into the purge_records table.
func (r *PurgePgRepository) Save(ctx context.Context, record *purge.PurgeRecord) error {
	conn := getConn(ctx, r.pool)

	artifactsJSON, err := json.Marshal(record.ArtifactsPurged)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO purge_records (
			id, target_id, target_type, reason, requested_by, audit_note,
			tenant_id, project_id,
			status, artifacts_purged,
			executed_at, error_detail,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8,
			$9, $10,
			$11, $12,
			$13, $14
		)`

	_, err = conn.Exec(ctx, query,
		record.ID.String(),              // $1
		record.TargetID.String(),        // $2
		record.TargetType,               // $3
		string(record.Reason),           // $4
		record.RequestedBy,              // $5
		record.AuditNote,                // $6
		ptrStr(record.Scope.TenantID),   // $7
		record.Scope.ProjectID,          // $8
		string(record.Status),           // $9
		artifactsJSON,                   // $10
		ptrTime(record.ExecutedAt),      // $11
		ptrStr(record.ErrorDetail),      // $12
		record.CreatedAt,                // $13
		record.UpdatedAt,                // $14
	)

	return err
}

// FindByID retrieves a PurgeRecord by its ID within the given scope.
// Returns shared.ErrNotFound if the record does not exist OR belongs to a
// different project (cross-project reads look identical to missing rows).
//
// Scope: scope is provided by the application layer from the auth context.
// Tenant predicate: NULL tenant on the row is visible to any key in the project.
func (r *PurgePgRepository) FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*purge.PurgeRecord, error) {
	conn := getConn(ctx, r.pool)

	const query = `
		SELECT
			id, target_id, target_type, reason, requested_by, audit_note,
			tenant_id, project_id,
			status, artifacts_purged,
			executed_at, error_detail,
			created_at, updated_at
		FROM purge_records
		WHERE id = $1
		  AND project_id = $2
		  AND (tenant_id IS NULL OR tenant_id = $3)`

	row := conn.QueryRow(ctx, query, id.String(), scope.ProjectID, scope.TenantID)

	var (
		idStr        string
		targetIDStr  string
		targetType   string
		reason       string
		requestedBy  string
		auditNote    string
		tenantID     *string
		projectID    string
		status       string
		artifactsRaw []byte
		errorDetail  *string
	)

	var rec purge.PurgeRecord

	err := row.Scan(
		&idStr, &targetIDStr, &targetType, &reason, &requestedBy, &auditNote,
		&tenantID, &projectID,
		&status, &artifactsRaw,
		&rec.ExecutedAt, &errorDetail,
		&rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}

	recordID, err := shared.RecordIDFromString(idStr)
	if err != nil {
		return nil, err
	}

	targetID, err := shared.RecordIDFromString(targetIDStr)
	if err != nil {
		return nil, err
	}

	var artifacts purge.PurgedArtifacts
	if artifactsRaw != nil {
		if err := json.Unmarshal(artifactsRaw, &artifacts); err != nil {
			return nil, err
		}
	}

	rec.ID = recordID
	rec.TargetID = targetID
	rec.TargetType = targetType
	rec.Reason = shared.PurgeReason(reason)
	rec.RequestedBy = requestedBy
	rec.AuditNote = auditNote
	rec.Scope = shared.Scope{
		TenantID:  tenantID,
		ProjectID: projectID,
	}
	rec.Status = shared.PurgeStatus(status)
	rec.ArtifactsPurged = artifacts
	rec.ErrorDetail = errorDetail

	return &rec, nil
}

// UpdateStatus updates the status and artifacts of a scoped purge record.
// Returns shared.ErrNotFound if the record does not exist within scope
// (including cross-project access — the two cases are indistinguishable).
//
// Scope: scope is provided by the application layer from the auth context.
func (r *PurgePgRepository) UpdateStatus(ctx context.Context, scope shared.Scope, id shared.RecordID, status shared.PurgeStatus, artifacts *purge.PurgedArtifacts) error {
	conn := getConn(ctx, r.pool)

	var artifactsArg any
	if artifacts != nil {
		b, err := json.Marshal(artifacts)
		if err != nil {
			return err
		}
		artifactsArg = b
	}

	const query = `
		UPDATE purge_records SET
			status = $1,
			artifacts_purged = $2,
			executed_at = CASE WHEN $1 = 'executed' THEN NOW() ELSE executed_at END,
			updated_at = NOW()
		WHERE id = $3
		  AND project_id = $4
		  AND (tenant_id IS NULL OR tenant_id = $5)`

	tag, err := conn.Exec(ctx, query, string(status), artifactsArg, id.String(), scope.ProjectID, scope.TenantID)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}

	return nil
}
