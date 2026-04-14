package persistence

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

// Compile-time check that FeedbackPgRepository implements FeedbackRepository.
var _ outbound.FeedbackRepository = (*FeedbackPgRepository)(nil)

// FeedbackPgRepository is the PostgreSQL implementation of the FeedbackRepository port.
type FeedbackPgRepository struct {
	pool *pgxpool.Pool
}

// NewFeedbackPgRepository creates a new FeedbackPgRepository backed by the given connection pool.
func NewFeedbackPgRepository(pool *pgxpool.Pool) *FeedbackPgRepository {
	return &FeedbackPgRepository{pool: pool}
}

// Save inserts a RetrievalFeedback record into the retrieval_feedback table.
func (r *FeedbackPgRepository) Save(ctx context.Context, feedback *shared.RetrievalFeedback) error {
	conn := getConn(ctx, r.pool)

	resultIDs := recordIDsToStrings(feedback.ResultIDs)
	selectedIDs := recordIDsToStrings(feedback.SelectedIDs)

	metadataJSON, err := json.Marshal(feedback.Metadata)
	if err != nil {
		return err
	}
	if feedback.Metadata == nil {
		metadataJSON = []byte("{}")
	}

	const query = `
		INSERT INTO retrieval_feedback (
			id, target, feedback_type, query,
			tenant_id, project_id, repo_id, agent_id, session_id, environment,
			result_ids, selected_ids,
			actor, metadata, created_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10,
			$11, $12,
			$13, $14, $15
		)`

	_, err = conn.Exec(ctx, query,
		feedback.ID.String(),                  // $1
		string(feedback.Target),               // $2
		string(feedback.FeedbackType),         // $3
		feedback.Query,                        // $4
		ptrStr(feedback.Scope.TenantID),       // $5
		feedback.Scope.ProjectID,              // $6
		ptrStr(feedback.Scope.RepoID),         // $7
		ptrStr(feedback.Scope.AgentID),        // $8
		ptrStr(feedback.Scope.SessionID),      // $9
		ptrStr(feedback.Scope.Environment),    // $10
		resultIDs,                             // $11
		selectedIDs,                           // $12
		feedback.Actor,                        // $13
		metadataJSON,                          // $14
		feedback.CreatedAt,                    // $15
	)

	return err
}

// recordIDsToStrings converts a slice of RecordID to a slice of strings for pgx array encoding.
func recordIDsToStrings(ids []RecordID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

// RecordID is a local alias to avoid repeating the full import path.
type RecordID = shared.RecordID
