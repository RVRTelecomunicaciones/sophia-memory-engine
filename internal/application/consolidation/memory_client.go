package consolidation

import "context"

// MemoryClient is the consolidation worker's narrow view of the memory-engine
// storage layer. It only needs to check existence and ingest new records.
// The full inbound.MemoryService is not required here, keeping dependencies minimal.
type MemoryClient interface {
	// HasTopic reports whether an active memory record exists at the given topic_key.
	// A storage error must be returned as a non-nil error; absence returns false, nil.
	HasTopic(ctx context.Context, topicKey string) (bool, error)

	// Ingest creates or upserts a memory record from IngestRequest.
	Ingest(ctx context.Context, req IngestRequest) error
}
