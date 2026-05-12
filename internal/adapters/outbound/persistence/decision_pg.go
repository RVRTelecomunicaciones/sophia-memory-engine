package persistence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that DecisionPgRepository implements DecisionRepository.
var _ outbound.DecisionRepository = (*DecisionPgRepository)(nil)

// DecisionPgRepository is the PostgreSQL implementation of the DecisionRepository port.
type DecisionPgRepository struct {
	pool *pgxpool.Pool
}

// NewDecisionPgRepository creates a new DecisionPgRepository backed by the given connection pool.
func NewDecisionPgRepository(pool *pgxpool.Pool) *DecisionPgRepository {
	return &DecisionPgRepository{pool: pool}
}

// evidenceJSON is the JSONB serialization format for EvidenceRef.
type evidenceJSON struct {
	RecordID *string `json:"record_id,omitempty"`
	URI      *string `json:"uri,omitempty"`
	Excerpt  *string `json:"excerpt,omitempty"`
	Type     string  `json:"type"`
}

func marshalEvidence(refs []shared.EvidenceRef) ([]byte, error) {
	items := make([]evidenceJSON, 0, len(refs))
	for _, ref := range refs {
		ej := evidenceJSON{
			URI:     ref.URI,
			Excerpt: ref.Excerpt,
			Type:    string(ref.Type),
		}
		if ref.RecordID != nil {
			s := ref.RecordID.String()
			ej.RecordID = &s
		}
		items = append(items, ej)
	}
	return json.Marshal(items)
}

func unmarshalEvidence(data []byte) ([]shared.EvidenceRef, error) {
	if data == nil {
		return nil, nil
	}

	var items []evidenceJSON
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}

	refs := make([]shared.EvidenceRef, 0, len(items))
	for _, ej := range items {
		ref := shared.EvidenceRef{
			URI:     ej.URI,
			Excerpt: ej.Excerpt,
			Type:    shared.EvidenceType(ej.Type),
		}
		if ej.RecordID != nil {
			rid, err := shared.RecordIDFromString(*ej.RecordID)
			if err != nil {
				return nil, err
			}
			ref.RecordID = &rid
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func ptrRecordIDStr(id *shared.RecordID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

// Save inserts a Decision into the decisions table.
func (r *DecisionPgRepository) Save(ctx context.Context, d *decision.Decision) error {
	conn := getConn(ctx, r.pool)

	evidenceBytes, err := marshalEvidence(d.Evidence)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO decisions (
			id, decision_key, version, title, description, rationale,
			evidence, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method,
			valid_from, valid_until, last_accessed, freshness,
			confidence_score, confidence_source,
			status, superseded_by,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8::regconfig,
			$9, $10, $11, $12, $13, $14,
			$15, $16, $17,
			$18, $19, $20, $21,
			$22, $23,
			$24, $25,
			$26, $27
		)`

	_, err = conn.Exec(ctx, query,
		d.ID.String(),                    // $1
		d.DecisionKey,                    // $2
		d.Version,                        // $3
		d.Title,                          // $4
		d.Description,                    // $5
		d.Rationale,                      // $6
		evidenceBytes,                    // $7
		d.FTSLanguage,                    // $8
		ptrStr(d.Scope.TenantID),         // $9
		d.Scope.ProjectID,                // $10
		ptrStr(d.Scope.RepoID),           // $11
		ptrStr(d.Scope.AgentID),          // $12
		ptrStr(d.Scope.SessionID),        // $13
		ptrStr(d.Scope.Environment),      // $14
		d.Provenance.Source,              // $15
		ptrStr(d.Provenance.SourceURI),   // $16
		string(d.Provenance.Method),      // $17
		ptrTime(d.Temporal.ValidFrom),    // $18
		ptrTime(d.Temporal.ValidUntil),   // $19
		ptrTime(d.Temporal.LastAccessed), // $20
		string(d.Temporal.Freshness),     // $21
		d.Confidence.Score,               // $22
		string(d.Confidence.Source),      // $23
		string(d.Status),                 // $24
		ptrRecordIDStr(d.SupersededBy),   // $25
		d.CreatedAt,                      // $26
		d.UpdatedAt,                      // $27
	)

	return err
}

// decisionColumns is the SELECT column list shared by all read queries.
const decisionColumns = `
	id, decision_key, version, title, description, rationale,
	evidence, fts_language,
	tenant_id, project_id, repo_id, agent_id, session_id, environment,
	source, source_uri, ingest_method,
	valid_from, valid_until, last_accessed, freshness,
	confidence_score, confidence_source,
	status, superseded_by,
	created_at, updated_at`

// scanner is satisfied by both pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanDecisionRow scans a row into a Decision struct.
func scanDecisionRow(row scanner) (*decision.Decision, error) {
	var (
		idStr        string
		decisionKey  string
		version      int
		title        string
		description  string
		rationale    string
		evidenceData []byte
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
		validFrom    *time.Time
		validUntil   *time.Time
		lastAccessed *time.Time
		freshness    string
		confScore    float64
		confSource   string
		status       string
		supersededBy *string
		createdAt    time.Time
		updatedAt    time.Time
	)

	err := row.Scan(
		&idStr, &decisionKey, &version, &title, &description, &rationale,
		&evidenceData, &ftsLanguage,
		&tenantID, &projectID, &repoID, &agentID, &sessionID, &environment,
		&source, &sourceURI, &ingestMethod,
		&validFrom, &validUntil, &lastAccessed, &freshness,
		&confScore, &confSource,
		&status, &supersededBy,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	recordID, err := shared.RecordIDFromString(idStr)
	if err != nil {
		return nil, err
	}

	evidence, err := unmarshalEvidence(evidenceData)
	if err != nil {
		return nil, err
	}

	d := &decision.Decision{
		ID:          recordID,
		DecisionKey: decisionKey,
		Version:     version,
		Title:       title,
		Description: description,
		Rationale:   rationale,
		Evidence:    evidence,
		FTSLanguage: ftsLanguage,
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
			Method:   shared.IngestMethod(ingestMethod),
		},
		Temporal: shared.TemporalMetadata{
			ValidFrom:    validFrom,
			ValidUntil:   validUntil,
			LastAccessed: lastAccessed,
			Freshness:    shared.FreshnessLevel(freshness),
		},
		Confidence: shared.Confidence{
			Score:  confScore,
			Source: shared.ConfidenceSource(confSource),
		},
		Status:    shared.DecisionStatus(status),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	if supersededBy != nil {
		sid, err := shared.RecordIDFromString(*supersededBy)
		if err != nil {
			return nil, err
		}
		d.SupersededBy = &sid
	}

	return d, nil
}

// FindByID retrieves a Decision by its ID within the given scope.
// Returns shared.ErrNotFound if the record does not exist OR belongs to a
// different project (cross-project reads look identical to missing rows).
//
// Scope: scope is provided by the application layer from the auth context.
// Tenant predicate: NULL tenant on the row is visible to any key in the project
// (legacy / cross-tenant rows). A non-NULL tenant must match exactly.
func (r *DecisionPgRepository) FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*decision.Decision, error) {
	conn := getConn(ctx, r.pool)

	query := `SELECT` + decisionColumns + `
		FROM decisions
		WHERE id = $1
		  AND project_id = $2
		  AND (tenant_id IS NULL OR tenant_id = $3)`

	row := conn.QueryRow(ctx, query, id.String(), scope.ProjectID, scope.TenantID)
	d, err := scanDecisionRow(row)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return d, nil
}

// FindActiveByKey retrieves the active decision for a given key and scope.
// Returns shared.ErrNotFound if no active decision exists.
func (r *DecisionPgRepository) FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*decision.Decision, error) {
	conn := getConn(ctx, r.pool)

	query := `SELECT` + decisionColumns + `
		FROM decisions
		WHERE decision_key = $1
		  AND status = 'active'
		  AND project_id = $2
		  AND ($3::text IS NULL OR tenant_id = $3)
		  AND ($4::text IS NULL OR repo_id = $4)
		  AND ($5::text IS NULL OR agent_id = $5)
		  AND ($6::text IS NULL OR session_id = $6)
		  AND ($7::text IS NULL OR environment = $7)`

	row := conn.QueryRow(ctx, query,
		key,
		scope.ProjectID,
		ptrStr(scope.TenantID),
		ptrStr(scope.RepoID),
		ptrStr(scope.AgentID),
		ptrStr(scope.SessionID),
		ptrStr(scope.Environment),
	)

	d, err := scanDecisionRow(row)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return d, nil
}

// FindByKey retrieves all versions of a decision by key and scope, ordered by version descending.
func (r *DecisionPgRepository) FindByKey(ctx context.Context, key string, scope shared.Scope) ([]decision.Decision, error) {
	conn := getConn(ctx, r.pool)

	query := `SELECT` + decisionColumns + `
		FROM decisions
		WHERE decision_key = $1
		  AND project_id = $2
		  AND ($3::text IS NULL OR tenant_id = $3)
		  AND ($4::text IS NULL OR repo_id = $4)
		  AND ($5::text IS NULL OR agent_id = $5)
		  AND ($6::text IS NULL OR session_id = $6)
		  AND ($7::text IS NULL OR environment = $7)
		ORDER BY version DESC`

	rows, err := conn.Query(ctx, query,
		key,
		scope.ProjectID,
		ptrStr(scope.TenantID),
		ptrStr(scope.RepoID),
		ptrStr(scope.AgentID),
		ptrStr(scope.SessionID),
		ptrStr(scope.Environment),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []decision.Decision
	for rows.Next() {
		d, err := scanDecisionRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// FindActiveByScope retrieves all active decisions for a given scope.
func (r *DecisionPgRepository) FindActiveByScope(ctx context.Context, scope shared.Scope) ([]decision.Decision, error) {
	conn := getConn(ctx, r.pool)

	query := `SELECT` + decisionColumns + `
		FROM decisions
		WHERE status = 'active'
		  AND project_id = $1
		  AND ($2::text IS NULL OR tenant_id = $2)
		  AND ($3::text IS NULL OR repo_id = $3)
		  AND ($4::text IS NULL OR agent_id = $4)
		  AND ($5::text IS NULL OR session_id = $5)
		  AND ($6::text IS NULL OR environment = $6)
		ORDER BY decision_key, version DESC`

	rows, err := conn.Query(ctx, query,
		scope.ProjectID,
		ptrStr(scope.TenantID),
		ptrStr(scope.RepoID),
		ptrStr(scope.AgentID),
		ptrStr(scope.SessionID),
		ptrStr(scope.Environment),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []decision.Decision
	for rows.Next() {
		d, err := scanDecisionRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// UpdateStatus changes the status and optional superseded_by of a scoped decision.
// Returns shared.ErrNotFound if no rows are affected within scope
// (including cross-project access — the two cases are indistinguishable).
//
// Scope: scope is provided by the application layer from the auth context.
func (r *DecisionPgRepository) UpdateStatus(ctx context.Context, scope shared.Scope, id shared.RecordID, status shared.DecisionStatus, supersededBy *shared.RecordID) error {
	conn := getConn(ctx, r.pool)

	const query = `
		UPDATE decisions SET status = $1, superseded_by = $2, updated_at = NOW()
		WHERE id = $3
		  AND project_id = $4
		  AND (tenant_id IS NULL OR tenant_id = $5)`

	tag, err := conn.Exec(ctx, query, string(status), ptrRecordIDStr(supersededBy), id.String(), scope.ProjectID, scope.TenantID)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}

	return nil
}
