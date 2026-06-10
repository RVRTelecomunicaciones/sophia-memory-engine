package consolidation

import "context"

// MemoryClient is the consolidation worker's narrow view of the memory-engine
// storage layer. It only needs to check existence, read content, and ingest records.
// The full inbound.MemoryService is not required here, keeping dependencies minimal.
type MemoryClient interface {
	// HasTopic reports whether an active memory record exists at the given topic_key.
	// A storage error must be returned as a non-nil error; absence returns false, nil.
	HasTopic(ctx context.Context, topicKey string) (bool, error)

	// ReadContent returns the content string of the active record at topic_key.
	// Returns "", nil when no record exists. Returns an error on storage failure.
	ReadContent(ctx context.Context, topicKey string) (string, error)

	// Ingest creates or upserts a memory record from IngestRequest.
	// When a record with the same topic_key already exists, it is overwritten
	// (topic_key acts as an upsert key in memory-engine).
	Ingest(ctx context.Context, req IngestRequest) error
}
