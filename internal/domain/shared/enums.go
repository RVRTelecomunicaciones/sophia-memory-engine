package shared

// MemoryType distinguishes episodic from semantic memories.
type MemoryType string

const (
	MemoryTypeEpisodic MemoryType = "episodic"
	MemoryTypeSemantic MemoryType = "semantic"
)

// IsValid returns true if the MemoryType is a known value.
func (m MemoryType) IsValid() bool {
	switch m {
	case MemoryTypeEpisodic, MemoryTypeSemantic:
		return true
	}
	return false
}

// MemoryStatus represents the lifecycle state of a memory.
type MemoryStatus string

const (
	MemoryStatusActive   MemoryStatus = "active"
	MemoryStatusArchived MemoryStatus = "archived"
	MemoryStatusPurged   MemoryStatus = "purged"
)

// IsValid returns true if the MemoryStatus is a known value.
func (m MemoryStatus) IsValid() bool {
	switch m {
	case MemoryStatusActive, MemoryStatusArchived, MemoryStatusPurged:
		return true
	}
	return false
}

// DecisionStatus represents the lifecycle state of a decision.
type DecisionStatus string

const (
	DecisionStatusActive       DecisionStatus = "active"
	DecisionStatusSuperseded   DecisionStatus = "superseded"
	DecisionStatusContradicted DecisionStatus = "contradicted"
	DecisionStatusArchived     DecisionStatus = "archived"
)

// IsValid returns true if the DecisionStatus is a known value.
func (d DecisionStatus) IsValid() bool {
	switch d {
	case DecisionStatusActive, DecisionStatusSuperseded, DecisionStatusContradicted, DecisionStatusArchived:
		return true
	}
	return false
}

// IngestMethod describes how a memory was created.
type IngestMethod string

const (
	IngestMethodDirect          IngestMethod = "direct"
	IngestMethodDerived         IngestMethod = "derived"
	IngestMethodImported        IngestMethod = "imported"
	IngestMethodWorkerGenerated IngestMethod = "worker_generated"
)

// IsValid returns true if the IngestMethod is a known value.
func (i IngestMethod) IsValid() bool {
	switch i {
	case IngestMethodDirect, IngestMethodDerived, IngestMethodImported, IngestMethodWorkerGenerated:
		return true
	}
	return false
}

// ConfidenceSource describes the origin of a confidence score.
type ConfidenceSource string

const (
	ConfidenceSourceHuman    ConfidenceSource = "human"
	ConfidenceSourceAgent    ConfidenceSource = "agent"
	ConfidenceSourceDerived  ConfidenceSource = "derived"
	ConfidenceSourceComputed ConfidenceSource = "computed"
)

// IsValid returns true if the ConfidenceSource is a known value.
func (c ConfidenceSource) IsValid() bool {
	switch c {
	case ConfidenceSourceHuman, ConfidenceSourceAgent, ConfidenceSourceDerived, ConfidenceSourceComputed:
		return true
	}
	return false
}

// FreshnessLevel represents the temporal freshness of a memory.
type FreshnessLevel string

const (
	FreshnessLevelFresh   FreshnessLevel = "fresh"
	FreshnessLevelAging   FreshnessLevel = "aging"
	FreshnessLevelStale   FreshnessLevel = "stale"
	FreshnessLevelExpired FreshnessLevel = "expired"
)

// IsValid returns true if the FreshnessLevel is a known value.
func (f FreshnessLevel) IsValid() bool {
	switch f {
	case FreshnessLevelFresh, FreshnessLevelAging, FreshnessLevelStale, FreshnessLevelExpired:
		return true
	}
	return false
}

// EvidenceType describes the kind of evidence supporting a decision.
type EvidenceType string

const (
	EvidenceTypeMemoryRef      EvidenceType = "memory_ref"
	EvidenceTypeExternalLink   EvidenceType = "external_link"
	EvidenceTypeInlineExcerpt  EvidenceType = "inline_excerpt"
)

// IsValid returns true if the EvidenceType is a known value.
func (e EvidenceType) IsValid() bool {
	switch e {
	case EvidenceTypeMemoryRef, EvidenceTypeExternalLink, EvidenceTypeInlineExcerpt:
		return true
	}
	return false
}

// RelationType describes the kind of relationship between two entities.
type RelationType string

const (
	RelationTypeRelatesTo   RelationType = "relates_to"
	RelationTypeDependsOn   RelationType = "depends_on"
	RelationTypeSupersedes  RelationType = "supersedes"
	RelationTypeContradicts RelationType = "contradicts"
	RelationTypeReferences  RelationType = "references"
	RelationTypeDerivedFrom RelationType = "derived_from"
	RelationTypeResolves    RelationType = "resolves"
	RelationTypeExtends     RelationType = "extends"
)

// IsValid returns true if the RelationType is a known value.
func (r RelationType) IsValid() bool {
	switch r {
	case RelationTypeRelatesTo, RelationTypeDependsOn, RelationTypeSupersedes,
		RelationTypeContradicts, RelationTypeReferences, RelationTypeDerivedFrom,
		RelationTypeResolves, RelationTypeExtends:
		return true
	}
	return false
}

// PurgeReason describes why a hard purge was requested.
type PurgeReason string

const (
	PurgeReasonSecretLeak       PurgeReason = "secret_leak"
	PurgeReasonCompliance       PurgeReason = "compliance"
	PurgeReasonSensitiveContent PurgeReason = "sensitive_content"
)

// IsValid returns true if the PurgeReason is a known value.
func (p PurgeReason) IsValid() bool {
	switch p {
	case PurgeReasonSecretLeak, PurgeReasonCompliance, PurgeReasonSensitiveContent:
		return true
	}
	return false
}

// PurgeStatus represents the execution state of a purge request.
type PurgeStatus string

const (
	PurgeStatusPending   PurgeStatus = "pending"
	PurgeStatusExecuting PurgeStatus = "executing"
	PurgeStatusExecuted  PurgeStatus = "executed"
	PurgeStatusFailed    PurgeStatus = "failed"
)

// IsValid returns true if the PurgeStatus is a known value.
func (p PurgeStatus) IsValid() bool {
	switch p {
	case PurgeStatusPending, PurgeStatusExecuting, PurgeStatusExecuted, PurgeStatusFailed:
		return true
	}
	return false
}

// EventType represents domain events emitted by the memory engine.
type EventType string

const (
	EventTypeMemoryIngested       EventType = "memory.ingested"
	EventTypeMemoryArchived       EventType = "memory.archived"
	EventTypeDecisionRecorded     EventType = "decision.recorded"
	EventTypeDecisionSuperseded   EventType = "decision.superseded"
	EventTypeDecisionContradicted EventType = "decision.contradicted"
	EventTypeHeuristicCreated     EventType = "heuristic.created"
	EventTypeHeuristicToggled     EventType = "heuristic.toggled"
	EventTypeRelationCreated      EventType = "relation.created"
	EventTypePurgeRequested       EventType = "purge.requested"
	EventTypePurgeExecuted        EventType = "purge.executed"
	EventTypeProfileGenerated     EventType = "profile.generated"
	EventTypeMemoryHelpful        EventType = "memory.helpful"
	EventTypeMemoryIrrelevant     EventType = "memory.irrelevant"
	EventTypeContextHelpful       EventType = "context.helpful"
	EventTypeContextIrrelevant    EventType = "context.irrelevant"
)

// IsValid returns true if the EventType is a known value.
func (e EventType) IsValid() bool {
	switch e {
	case EventTypeMemoryIngested, EventTypeMemoryArchived,
		EventTypeDecisionRecorded, EventTypeDecisionSuperseded, EventTypeDecisionContradicted,
		EventTypeHeuristicCreated, EventTypeHeuristicToggled,
		EventTypeRelationCreated,
		EventTypePurgeRequested, EventTypePurgeExecuted,
		EventTypeProfileGenerated,
		EventTypeMemoryHelpful, EventTypeMemoryIrrelevant,
		EventTypeContextHelpful, EventTypeContextIrrelevant:
		return true
	}
	return false
}
