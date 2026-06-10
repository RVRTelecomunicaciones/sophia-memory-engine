package consolidation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// MemoryServiceClient wraps the inbound.MemoryService to implement MemoryClient
// for the consolidation worker pipeline. It uses a fixed scope (projectID)
// derived from the worker's operational context — no per-request auth needed
// because the pipeline runs as a trusted internal component.
type MemoryServiceClient struct {
	svc       inbound.MemoryService
	projectID string
	clock     shared.Clock
}

// NewMemoryServiceClient constructs a MemoryServiceClient.
// projectID is the memory-engine scope project_id used for all reads/writes.
func NewMemoryServiceClient(svc inbound.MemoryService, projectID string, clock shared.Clock) *MemoryServiceClient {
	return &MemoryServiceClient{svc: svc, projectID: projectID, clock: clock}
}

// HasTopic implements MemoryClient.
// Returns (true, nil) when an active record exists at topic_key;
// (false, nil) when not found; (false, err) on storage error.
func (m *MemoryServiceClient) HasTopic(ctx context.Context, topicKey string) (bool, error) {
	query := inbound.GetByTopicKeyQuery{
		ProjectID: m.projectID,
		TopicKey:  topicKey,
	}
	_, err := m.svc.GetByTopicKey(ctx, query)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, shared.ErrNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("consolidation.HasTopic: %w", err)
}

// ReadContent implements MemoryClient.
// Returns the content of the active record at topic_key, or "" when absent.
func (m *MemoryServiceClient) ReadContent(ctx context.Context, topicKey string) (string, error) {
	query := inbound.GetByTopicKeyQuery{
		ProjectID: m.projectID,
		TopicKey:  topicKey,
	}
	rec, err := m.svc.GetByTopicKey(ctx, query)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("consolidation.ReadContent: %w", err)
	}
	return rec.Content, nil
}

// Ingest implements MemoryClient.
// Writes or upserts a record via the memory service. The provenance source
// is "archive_worker"; the method is "computed" (no direct user action).
func (m *MemoryServiceClient) Ingest(ctx context.Context, req IngestRequest) error {
	scope, err := shared.NewScope(m.projectID)
	if err != nil {
		return fmt.Errorf("consolidation.Ingest: build scope: %w", err)
	}

	prov, err := shared.NewProvenance("archive_worker", shared.IngestMethodWorkerGenerated, nil)
	if err != nil {
		return fmt.Errorf("consolidation.Ingest: build provenance: %w", err)
	}

	memType := shared.MemoryType(req.Type)
	topicKey := req.TopicKey

	cmd := inbound.IngestMemoryCmd{
		Type:     memType,
		Content:  req.Content,
		Tags:     req.Tags,
		TopicKey: &topicKey,
		Scope:    scope,
		Provenance: prov,
	}

	result, err := m.svc.Ingest(ctx, cmd)
	if err != nil {
		return fmt.Errorf("consolidation.Ingest: %w", err)
	}

	slog.InfoContext(ctx, "consolidation: memory ingested",
		"id", result.ID.String(),
		"topic_key", topicKey,
	)
	return nil
}

