package memory

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// MemoryRecord represents a single unit of memory in the engine.
// It is typed, scoped, temporal, and tracks provenance and importance.
type MemoryRecord struct {
	ID            shared.RecordID
	Type          shared.MemoryType
	Content       string
	Summary       *string
	Tags          []string
	TopicKey      *string
	Scope         shared.Scope
	Provenance    shared.Provenance
	Temporal      shared.TemporalMetadata
	Importance    shared.ImportanceScore
	Status        shared.MemoryStatus
	ArchivedBy    *string
	ArchiveReason *string
	FTSLanguage   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Option configures optional fields on a MemoryRecord during construction.
type Option func(*MemoryRecord)

// WithValidFrom sets the temporal ValidFrom field.
func WithValidFrom(t time.Time) Option {
	return func(m *MemoryRecord) {
		m.Temporal.ValidFrom = &t
	}
}

// WithValidUntil sets the temporal ValidUntil field.
func WithValidUntil(t time.Time) Option {
	return func(m *MemoryRecord) {
		m.Temporal.ValidUntil = &t
	}
}

// WithSummary sets the summary field.
func WithSummary(s string) Option {
	return func(m *MemoryRecord) {
		m.Summary = &s
	}
}

// WithTags sets the tags field.
func WithTags(tags []string) Option {
	return func(m *MemoryRecord) {
		m.Tags = tags
	}
}

// WithTopicKey sets the topic key field.
func WithTopicKey(key string) Option {
	return func(m *MemoryRecord) {
		m.TopicKey = &key
	}
}

// WithFTSLanguage overrides the default FTS language.
func WithFTSLanguage(lang string) Option {
	return func(m *MemoryRecord) {
		m.FTSLanguage = lang
	}
}

// NewMemoryRecord creates a validated MemoryRecord with sensible defaults.
// Episodic memories require ValidFrom to be set (via WithValidFrom option).
func NewMemoryRecord(
	memType shared.MemoryType,
	content string,
	scope shared.Scope,
	provenance shared.Provenance,
	clock shared.Clock,
	opts ...Option,
) (*MemoryRecord, error) {
	var errs []shared.FieldError

	if content == "" {
		errs = append(errs, shared.FieldError{
			Field:   "content",
			Message: "required",
			Value:   content,
		})
	}

	if !memType.IsValid() {
		errs = append(errs, shared.FieldError{
			Field:   "type",
			Message: "invalid memory type",
			Value:   memType,
		})
	}

	if len(errs) > 0 {
		return nil, shared.NewValidationError(errs...)
	}

	now := clock.Now()

	m := &MemoryRecord{
		ID:          shared.NewRecordID(),
		Type:        memType,
		Content:     content,
		Tags:        []string{},
		Scope:       scope,
		Provenance:  provenance,
		Temporal:    shared.TemporalMetadata{Freshness: shared.FreshnessLevelFresh},
		Importance:  shared.DefaultImportanceScore(now),
		Status:      shared.MemoryStatusActive,
		FTSLanguage: "spanish",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	for _, opt := range opts {
		opt(m)
	}

	// Post-option validation: episodic memories require ValidFrom.
	if memType == shared.MemoryTypeEpisodic && m.Temporal.ValidFrom == nil {
		return nil, shared.NewValidationError(shared.FieldError{
			Field:   "valid_from",
			Message: "required for episodic memories",
			Value:   nil,
		})
	}

	return m, nil
}

// Archive transitions the memory from active to archived.
// Returns ErrPurged if the memory is purged, ErrAlreadyArchived if already archived.
func (m *MemoryRecord) Archive(by, reason string) error {
	if m.Status == shared.MemoryStatusPurged {
		return shared.ErrPurged
	}
	if m.Status == shared.MemoryStatusArchived {
		return shared.ErrAlreadyArchived
	}

	m.Status = shared.MemoryStatusArchived
	m.ArchivedBy = &by
	m.ArchiveReason = &reason

	return nil
}

// MarkPurged transitions the memory to purged state, wiping sensitive content.
func (m *MemoryRecord) MarkPurged() {
	m.Status = shared.MemoryStatusPurged
	m.Content = ""
	m.Summary = nil
	m.Tags = []string{}
}
