package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that HeuristicPgRepository implements HeuristicRepository.
var _ outbound.HeuristicRepository = (*HeuristicPgRepository)(nil)

// HeuristicPgRepository is the PostgreSQL implementation of the HeuristicRepository port.
type HeuristicPgRepository struct {
	pool *pgxpool.Pool
}

// NewHeuristicPgRepository creates a new HeuristicPgRepository backed by the given connection pool.
func NewHeuristicPgRepository(pool *pgxpool.Pool) *HeuristicPgRepository {
	return &HeuristicPgRepository{pool: pool}
}

// Save inserts a HeuristicRule into the heuristics table.
func (r *HeuristicPgRepository) Save(ctx context.Context, rule *heuristic.HeuristicRule) error {
	conn := getConn(ctx, r.pool)

	const query = `
		INSERT INTO heuristics (
			id, heuristic_key, version, condition, action, rationale, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method,
			confidence_score, confidence_source,
			enabled,
			valid_from, valid_until, last_accessed, freshness,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7::regconfig,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16,
			$17, $18,
			$19,
			$20, $21, $22, $23,
			$24, $25
		)`

	_, err := conn.Exec(ctx, query,
		rule.ID.String(),                        // $1
		rule.HeuristicKey,                       // $2
		rule.Version,                            // $3
		rule.Condition,                          // $4
		rule.Action,                             // $5
		rule.Rationale,                          // $6
		rule.FTSLanguage,                        // $7
		ptrStr(rule.Scope.TenantID),             // $8
		rule.Scope.ProjectID,                    // $9
		ptrStr(rule.Scope.RepoID),               // $10
		ptrStr(rule.Scope.AgentID),              // $11
		ptrStr(rule.Scope.SessionID),            // $12
		ptrStr(rule.Scope.Environment),          // $13
		rule.Provenance.Source,                   // $14
		ptrStr(rule.Provenance.SourceURI),       // $15
		string(rule.Provenance.Method),          // $16
		rule.Confidence.Score,                   // $17
		string(rule.Confidence.Source),           // $18
		rule.Enabled,                            // $19
		ptrTime(rule.Temporal.ValidFrom),        // $20
		ptrTime(rule.Temporal.ValidUntil),       // $21
		ptrTime(rule.Temporal.LastAccessed),     // $22
		string(rule.Temporal.Freshness),         // $23
		rule.CreatedAt,                          // $24
		rule.UpdatedAt,                          // $25
	)

	return err
}

// FindByID retrieves a HeuristicRule by its ID within the given scope.
// Returns shared.ErrNotFound if the record does not exist OR belongs to a
// different project (cross-project reads look identical to missing rows).
//
// Scope: scope is provided by the application layer from the auth context.
// Tenant predicate: NULL tenant on the row is visible to any key in the project
// (legacy / cross-tenant rows). A non-NULL tenant must match exactly.
func (r *HeuristicPgRepository) FindByID(ctx context.Context, scope shared.Scope, id shared.RecordID) (*heuristic.HeuristicRule, error) {
	conn := getConn(ctx, r.pool)

	const query = `
		SELECT
			id, heuristic_key, version, condition, action, rationale, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method,
			confidence_score, confidence_source,
			enabled,
			valid_from, valid_until, last_accessed, freshness,
			created_at, updated_at
		FROM heuristics
		WHERE id = $1
		  AND project_id = $2
		  AND (tenant_id IS NULL OR tenant_id = $3)`

	row := conn.QueryRow(ctx, query, id.String(), scope.ProjectID, scope.TenantID)
	return scanHeuristicRule(row)
}

// FindActiveByKey retrieves the latest active version of a heuristic by key and scope.
// Returns shared.ErrNotFound if no active rule matches.
func (r *HeuristicPgRepository) FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*heuristic.HeuristicRule, error) {
	conn := getConn(ctx, r.pool)

	args := []any{key, scope.ProjectID}
	conditions := []string{
		"heuristic_key = $1",
		"enabled = true",
		"(valid_until IS NULL OR valid_until > NOW())",
		"project_id = $2",
	}
	idx := 3

	if scope.TenantID != nil {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", idx))
		args = append(args, *scope.TenantID)
		idx++
	}
	if scope.RepoID != nil {
		conditions = append(conditions, fmt.Sprintf("repo_id = $%d", idx))
		args = append(args, *scope.RepoID)
		idx++
	}
	if scope.AgentID != nil {
		conditions = append(conditions, fmt.Sprintf("agent_id = $%d", idx))
		args = append(args, *scope.AgentID)
		idx++
	}
	if scope.SessionID != nil {
		conditions = append(conditions, fmt.Sprintf("session_id = $%d", idx))
		args = append(args, *scope.SessionID)
		idx++
	}
	if scope.Environment != nil {
		conditions = append(conditions, fmt.Sprintf("environment = $%d", idx))
		args = append(args, *scope.Environment)
		idx++
	}

	query := fmt.Sprintf(`
		SELECT
			id, heuristic_key, version, condition, action, rationale, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method,
			confidence_score, confidence_source,
			enabled,
			valid_from, valid_until, last_accessed, freshness,
			created_at, updated_at
		FROM heuristics
		WHERE %s
		ORDER BY version DESC
		LIMIT 1`, strings.Join(conditions, " AND "))

	row := conn.QueryRow(ctx, query, args...)
	return scanHeuristicRule(row)
}

// FindByScope retrieves heuristic rules matching the given scope and optional enabled filter.
func (r *HeuristicPgRepository) FindByScope(ctx context.Context, scope shared.Scope, enabled *bool) ([]heuristic.HeuristicRule, error) {
	conn := getConn(ctx, r.pool)

	args := []any{scope.ProjectID}
	conditions := []string{"project_id = $1"}
	idx := 2

	if scope.TenantID != nil {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", idx))
		args = append(args, *scope.TenantID)
		idx++
	}
	if scope.RepoID != nil {
		conditions = append(conditions, fmt.Sprintf("repo_id = $%d", idx))
		args = append(args, *scope.RepoID)
		idx++
	}
	if scope.AgentID != nil {
		conditions = append(conditions, fmt.Sprintf("agent_id = $%d", idx))
		args = append(args, *scope.AgentID)
		idx++
	}
	if scope.SessionID != nil {
		conditions = append(conditions, fmt.Sprintf("session_id = $%d", idx))
		args = append(args, *scope.SessionID)
		idx++
	}
	if scope.Environment != nil {
		conditions = append(conditions, fmt.Sprintf("environment = $%d", idx))
		args = append(args, *scope.Environment)
		idx++
	}
	if enabled != nil {
		conditions = append(conditions, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *enabled)
		idx++
	}

	query := fmt.Sprintf(`
		SELECT
			id, heuristic_key, version, condition, action, rationale, fts_language,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			source, source_uri, ingest_method,
			confidence_score, confidence_source,
			enabled,
			valid_from, valid_until, last_accessed, freshness,
			created_at, updated_at
		FROM heuristics
		WHERE %s
		ORDER BY heuristic_key, version DESC`, strings.Join(conditions, " AND "))

	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []heuristic.HeuristicRule
	for rows.Next() {
		var (
			idStr        string
			hKey         string
			version      int
			condition    string
			action       string
			rationale    string
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
			confScore    float64
			confSource   string
			enabledVal   bool
			validFrom    *time.Time
			validUntil   *time.Time
			lastAccessed *time.Time
			freshness    string
			createdAt    time.Time
			updatedAt    time.Time
		)

		err := rows.Scan(
			&idStr, &hKey, &version, &condition, &action, &rationale, &ftsLanguage,
			&tenantID, &projectID, &repoID, &agentID, &sessionID, &environment,
			&source, &sourceURI, &ingestMethod,
			&confScore, &confSource,
			&enabledVal,
			&validFrom, &validUntil, &lastAccessed, &freshness,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		recordID, err := shared.RecordIDFromString(idStr)
		if err != nil {
			return nil, err
		}

		rules = append(rules, heuristic.HeuristicRule{
			ID:           recordID,
			HeuristicKey: hKey,
			Version:      version,
			Condition:    condition,
			Action:       action,
			Rationale:    rationale,
			FTSLanguage:  ftsLanguage,
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
			Confidence: shared.Confidence{
				Score:  confScore,
				Source: shared.ConfidenceSource(confSource),
			},
			Enabled: enabledVal,
			Temporal: shared.TemporalMetadata{
				ValidFrom:    validFrom,
				ValidUntil:   validUntil,
				LastAccessed: lastAccessed,
				Freshness:    shared.FreshnessLevel(freshness),
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rules, nil
}

// UpdateEnabled sets the enabled flag on a scoped heuristic rule.
// Returns shared.ErrNotFound if the record does not exist within scope
// (including cross-project access — the two cases are indistinguishable).
//
// Scope: scope is provided by the application layer from the auth context.
func (r *HeuristicPgRepository) UpdateEnabled(ctx context.Context, scope shared.Scope, id shared.RecordID, enabled bool) error {
	conn := getConn(ctx, r.pool)

	const query = `
		UPDATE heuristics SET enabled = $1, updated_at = NOW()
		WHERE id = $2
		  AND project_id = $3
		  AND (tenant_id IS NULL OR tenant_id = $4)`

	tag, err := conn.Exec(ctx, query, enabled, id.String(), scope.ProjectID, scope.TenantID)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return shared.ErrNotFound
	}

	return nil
}

// scanHeuristicRule is a helper for scanning a single row into a HeuristicRule.
type scannable interface {
	Scan(dest ...any) error
}

func scanHeuristicRule(row scannable) (*heuristic.HeuristicRule, error) {
	var (
		idStr        string
		hKey         string
		version      int
		condition    string
		action       string
		rationale    string
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
		confScore    float64
		confSource   string
		enabledVal   bool
		validFrom    *time.Time
		validUntil   *time.Time
		lastAccessed *time.Time
		freshness    string
		createdAt    time.Time
		updatedAt    time.Time
	)

	err := row.Scan(
		&idStr, &hKey, &version, &condition, &action, &rationale, &ftsLanguage,
		&tenantID, &projectID, &repoID, &agentID, &sessionID, &environment,
		&source, &sourceURI, &ingestMethod,
		&confScore, &confSource,
		&enabledVal,
		&validFrom, &validUntil, &lastAccessed, &freshness,
		&createdAt, &updatedAt,
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

	return &heuristic.HeuristicRule{
		ID:           recordID,
		HeuristicKey: hKey,
		Version:      version,
		Condition:    condition,
		Action:       action,
		Rationale:    rationale,
		FTSLanguage:  ftsLanguage,
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
		Confidence: shared.Confidence{
			Score:  confScore,
			Source: shared.ConfidenceSource(confSource),
		},
		Enabled: enabledVal,
		Temporal: shared.TemporalMetadata{
			ValidFrom:    validFrom,
			ValidUntil:   validUntil,
			LastAccessed: lastAccessed,
			Freshness:    shared.FreshnessLevel(freshness),
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
