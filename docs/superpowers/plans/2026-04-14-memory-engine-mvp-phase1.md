# Memory Engine MVP Phase 1 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the memory-engine MVP phase 1 — a reusable memory engine for AI development systems with episodic/semantic memory, decisions, heuristics, graph relations, hybrid retrieval, and context building.

**Architecture:** Hexagonal/clean architecture in Go. Domain layer (entities + value objects) → Application layer (use cases) → Ports (inbound/outbound interfaces) → Adapters (PostgreSQL, HTTP REST). Inside-out implementation: domain first, then ports, then adapters, then wiring.

**Tech Stack:** Go 1.22+, PostgreSQL 16+, pgx v5, chi router, testify, testcontainers-go, oklog/ulid, slog

**Spec:** `docs/superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md`

---

## File Structure

### Domain Layer (`internal/domain/`)

```
internal/domain/shared/
├── record_id.go          — ULID-based RecordID value object
├── record_id_test.go
├── scope.go              — Scope value object with ProjectID required
├── scope_test.go
├── provenance.go         — Provenance value object with IngestMethod enum
├── provenance_test.go
├── temporal.go           — TemporalMetadata, FreshnessLevel, FreshnessConfig
├── temporal_test.go
├── confidence.go         — Confidence value object with ConfidenceSource
├── confidence_test.go
├── importance.go         — ImportanceScore with explainable factors
├── importance_test.go
├── evidence.go           — EvidenceRef value object with EvidenceType
├── evidence_test.go
├── timerange.go          — TimeRange value object
├── timerange_test.go
├── enums.go              — All domain enums as typed string constants
├── errors.go             — Domain error types (ErrNotFound, ErrPurged, etc.)
├── errors_test.go

internal/domain/memory/
├── memory.go             — MemoryRecord entity, MemoryType, MemoryStatus
├── memory_test.go

internal/domain/decision/
├── decision.go           — Decision entity, DecisionStatus, transitions
├── decision_test.go

internal/domain/heuristic/
├── heuristic.go          — HeuristicRule entity, version semantics
├── heuristic_test.go

internal/domain/relation/
├── relation.go           — Relation entity, RelationType enum
├── relation_test.go

internal/domain/purge/
├── purge.go              — PurgeRecord, PurgeReason, PurgeStatus, PurgedArtifacts
├── purge_test.go

internal/domain/projectprofile/
├── profile.go            — ProjectProfile, PatternEntry, FreshnessState, SourceCounts
├── profile_test.go
```

### Ports Layer (`internal/ports/`)

```
internal/ports/inbound/
├── memory_service.go     — MemoryService interface + Cmd/Result/Query types
├── decision_service.go   — DecisionService interface + Cmd/Result/Query types
├── heuristic_service.go  — HeuristicService interface + Cmd/Result/Query types
├── relation_service.go   — RelationService interface + Cmd/Result/Query types
├── retrieval_service.go  — RetrievalService interface + SearchQuery/ContextRequest types
├── purge_service.go      — PurgeService interface + Cmd types
├── profile_service.go    — ProjectProfileService interface + Cmd/Query types

internal/ports/outbound/
├── memory_repository.go      — MemoryRepository interface
├── decision_repository.go    — DecisionRepository interface
├── heuristic_repository.go   — HeuristicRepository interface
├── relation_repository.go    — RelationRepository interface + TraverseQuery/Result
├── purge_repository.go       — PurgeRepository interface
├── profile_repository.go     — ProjectProfileRepository interface
├── search_index.go           — SearchIndex interface + SearchEntry/FTSQuery/FTSResult
├── embedding_generator.go    — EmbeddingGenerator interface (stub)
├── tx_manager.go             — TransactionManager interface
├── clock.go                  — Clock interface + RealClock + FixedClock
├── event_publisher.go        — EventPublisher interface + DomainEvent + EventType constants
```

### Infrastructure Layer (`internal/infrastructure/`)

```
internal/infrastructure/config/
├── config.go             — AppConfig, DatabaseConfig, RetrievalConfig, ServerConfig

internal/infrastructure/database/
├── postgres.go           — PG connection pool setup, context-aware tx helper

internal/infrastructure/logging/
├── logger.go             — slog setup

internal/infrastructure/events/
├── inprocess.go          — InProcessEventPublisher implementation
├── inprocess_test.go
```

### Adapters Layer (`internal/adapters/`)

```
internal/adapters/outbound/persistence/
├── memory_pg.go          — PostgreSQL MemoryRepository
├── memory_pg_test.go     — Unit tests with mocked DB (query construction)
├── decision_pg.go        — PostgreSQL DecisionRepository
├── decision_pg_test.go
├── heuristic_pg.go       — PostgreSQL HeuristicRepository
├── heuristic_pg_test.go
├── relation_pg.go        — PostgreSQL RelationRepository (includes CTE)
├── relation_pg_test.go
├── purge_pg.go           — PostgreSQL PurgeRepository
├── purge_pg_test.go
├── profile_pg.go         — PostgreSQL ProjectProfileRepository
├── profile_pg_test.go
├── tx_manager_pg.go      — PostgreSQL TransactionManager
├── helpers.go            — Shared scan helpers, scope filter builder

internal/adapters/outbound/search/
├── postgres_fts.go       — PostgreSQL FTS SearchIndex implementation
├── postgres_fts_test.go

internal/adapters/outbound/embeddings/
├── noop.go               — NoopEmbeddingGenerator stub

internal/adapters/inbound/http/
├── server.go             — chi router setup, middleware
├── memory_handler.go     — HTTP handlers for memory endpoints
├── decision_handler.go   — HTTP handlers for decision endpoints
├── heuristic_handler.go  — HTTP handlers for heuristic endpoints
├── relation_handler.go   — HTTP handlers for relation endpoints
├── retrieval_handler.go  — HTTP handlers for search + context
├── purge_handler.go      — HTTP handlers for purge endpoints
├── profile_handler.go    — HTTP handlers for profile endpoints
├── responses.go          — JSON response helpers, error mapping
├── middleware.go         — Request logging, error recovery
```

### Application Layer (`internal/application/`)

```
internal/application/ingest/
├── service.go            — IngestService (MemoryService implementation)
├── service_test.go

internal/application/decisions/
├── service.go            — DecisionAppService (DecisionService implementation)
├── service_test.go

internal/application/heuristics/
├── service.go            — HeuristicAppService (HeuristicService implementation)
├── service_test.go

internal/application/relations/
├── service.go            — RelationAppService (RelationService implementation)
├── service_test.go

internal/application/retrieval/
├── search.go             — SearchService (Search use case)
├── search_test.go
├── ranking.go            — Ranking computation with configurable weights
├── ranking_test.go
├── context_builder.go    — ContextBuilder (BuildContext use case)
├── context_builder_test.go

internal/application/security/
├── service.go            — PurgeAppService (PurgeService implementation)
├── service_test.go

internal/application/projectdna/
├── service.go            — ProfileAppService (ProjectProfileService implementation)
├── service_test.go
```

### Migrations & Tests

```
migrations/postgres/
├── 001_initial_schema.up.sql
├── 001_initial_schema.down.sql

test/
├── integration/
│   ├── testhelper/
│   │   └── pg.go              — testcontainers PG setup + migration runner
│   ├── memory_repository_test.go
│   ├── decision_repository_test.go
│   ├── heuristic_repository_test.go
│   ├── relation_repository_test.go
│   ├── search_index_test.go
│   └── usecases/
│       ├── ingest_memory_test.go
│       ├── record_decision_test.go
│       ├── search_test.go
│       └── build_context_test.go
├── fixtures/
│   ├── memories.go
│   ├── decisions.go
│   ├── heuristics.go
│   ├── relations.go
│   └── scopes.go

cmd/
├── memory-engine/
│   └── main.go            — HTTP server entrypoint
```

---

## Task 1: Project Bootstrap

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `internal/infrastructure/config/config.go`
- Create: `internal/infrastructure/logging/logger.go`

- [ ] **Step 1: Initialize Go module and install core dependencies**

```bash
cd /Users/russell/Documents/2026/sophia-memory-engine
go mod init github.com/sophia-engine/memory-engine
go get github.com/oklog/ulid/v2@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/go-chi/chi/v5@latest
go get github.com/stretchr/testify@latest
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
```

- [ ] **Step 2: Create Makefile**

```makefile
.PHONY: test test-unit test-integration lint

test:
	go test ./...

test-unit:
	go test ./internal/domain/... ./internal/application/...

test-integration:
	go test ./test/integration/... -tags=integration -count=1

lint:
	golangci-lint run ./...

migrate-up:
	@echo "Run: psql $$DATABASE_URL -f migrations/postgres/001_initial_schema.up.sql"

migrate-down:
	@echo "Run: psql $$DATABASE_URL -f migrations/postgres/001_initial_schema.down.sql"
```

- [ ] **Step 3: Create config struct**

```go
// internal/infrastructure/config/config.go
package config

import "time"

type AppConfig struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Retrieval RetrievalConfig
}

type ServerConfig struct {
	Port         int    `default:"8080"`
	ReadTimeout  time.Duration `default:"10s"`
	WriteTimeout time.Duration `default:"30s"`
}

type DatabaseConfig struct {
	URL             string `required:"true"`
	MaxConns        int    `default:"25"`
	MinConns        int    `default:"5"`
	MaxConnLifetime time.Duration `default:"1h"`
}

type RetrievalConfig struct {
	Weights       RankingWeights
	Thresholds    RetrievalThresholds
	ContextBudget ContextBudgetConfig
}

type RankingWeights struct {
	FTS            float64 `default:"0.25"`
	Trigram        float64 `default:"0.15"`
	Recency        float64 `default:"0.12"`
	Importance     float64 `default:"0.13"`
	TypeBoost      float64 `default:"0.10"`
	Freshness      float64 `default:"0.10"`
	ScopeExactness float64 `default:"0.15"`
}

type RetrievalThresholds struct {
	MinTrigramSimilarity float64 `default:"0.3"`
	MinFinalScore        float64 `default:"0.1"`
	MaxResults           int     `default:"100"`
	DefaultResults       int     `default:"20"`
}

type ContextBudgetConfig struct {
	DefaultMaxTokens    int     `default:"4000"`
	DecisionsPct        float64 `default:"0.25"`
	HeuristicsPct       float64 `default:"0.20"`
	RecentEpisodicPct   float64 `default:"0.30"`
	SemanticPct         float64 `default:"0.15"`
	RelatedPct          float64 `default:"0.10"`
	TokenEstimateRatio  float64 `default:"0.25"`
	MaxGraphExpandDepth int     `default:"1"`
	MaxGraphExpandCount int     `default:"5"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		Server: ServerConfig{
			Port:         8080,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			MaxConns:        25,
			MinConns:        5,
			MaxConnLifetime: time.Hour,
		},
		Retrieval: RetrievalConfig{
			Weights: RankingWeights{
				FTS: 0.25, Trigram: 0.15, Recency: 0.12,
				Importance: 0.13, TypeBoost: 0.10, Freshness: 0.10,
				ScopeExactness: 0.15,
			},
			Thresholds: RetrievalThresholds{
				MinTrigramSimilarity: 0.3,
				MinFinalScore:        0.1,
				MaxResults:           100,
				DefaultResults:       20,
			},
			ContextBudget: ContextBudgetConfig{
				DefaultMaxTokens: 4000,
				DecisionsPct: 0.25, HeuristicsPct: 0.20,
				RecentEpisodicPct: 0.30, SemanticPct: 0.15,
				RelatedPct: 0.10, TokenEstimateRatio: 0.25,
				MaxGraphExpandDepth: 1, MaxGraphExpandCount: 5,
			},
		},
	}
}
```

- [ ] **Step 4: Create logger setup**

```go
// internal/infrastructure/logging/logger.go
package logging

import (
	"log/slog"
	"os"
)

func NewLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/russell/Documents/2026/sophia-memory-engine && go build ./...`
Expected: Clean compilation, no errors.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum Makefile internal/infrastructure/config/config.go internal/infrastructure/logging/logger.go
git commit -m "feat: bootstrap project with go.mod, config, and logging"
```

---

## Task 2: Domain Shared — Enums and Errors

**Files:**
- Create: `internal/domain/shared/enums.go`
- Create: `internal/domain/shared/errors.go`
- Create: `internal/domain/shared/errors_test.go`

- [ ] **Step 1: Write error type tests**

```go
// internal/domain/shared/errors_test.go
package shared_test

import (
	"errors"
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrNotFound_IsDistinctFromErrPurged(t *testing.T) {
	assert.False(t, errors.Is(shared.ErrNotFound, shared.ErrPurged))
	assert.False(t, errors.Is(shared.ErrPurged, shared.ErrNotFound))
}

func TestValidationError_ContainsFieldErrors(t *testing.T) {
	err := shared.NewValidationError(
		shared.FieldError{Field: "content", Message: "required"},
		shared.FieldError{Field: "project_id", Message: "required"},
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shared.ErrValidation))

	var ve *shared.ValidationError
	require.True(t, errors.As(err, &ve))
	assert.Len(t, ve.Fields, 2)
	assert.Equal(t, "content", ve.Fields[0].Field)
}

func TestValidationError_ErrorMessage(t *testing.T) {
	err := shared.NewValidationError(
		shared.FieldError{Field: "content", Message: "required"},
	)
	assert.Contains(t, err.Error(), "content")
	assert.Contains(t, err.Error(), "required")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/russell/Documents/2026/sophia-memory-engine && go test ./internal/domain/shared/...`
Expected: FAIL — package doesn't compile.

- [ ] **Step 3: Implement enums**

```go
// internal/domain/shared/enums.go
package shared

// MemoryType distinguishes episodic from semantic memories.
type MemoryType string

const (
	MemoryTypeEpisodic MemoryType = "episodic"
	MemoryTypeSemantic MemoryType = "semantic"
)

func (m MemoryType) IsValid() bool {
	return m == MemoryTypeEpisodic || m == MemoryTypeSemantic
}

// MemoryStatus represents the lifecycle state of a memory record.
type MemoryStatus string

const (
	MemoryStatusActive   MemoryStatus = "active"
	MemoryStatusArchived MemoryStatus = "archived"
	MemoryStatusPurged   MemoryStatus = "purged"
)

func (s MemoryStatus) IsValid() bool {
	return s == MemoryStatusActive || s == MemoryStatusArchived || s == MemoryStatusPurged
}

// DecisionStatus represents mutually exclusive decision states.
type DecisionStatus string

const (
	DecisionStatusActive       DecisionStatus = "active"
	DecisionStatusSuperseded   DecisionStatus = "superseded"
	DecisionStatusContradicted DecisionStatus = "contradicted"
	DecisionStatusArchived     DecisionStatus = "archived"
)

func (s DecisionStatus) IsValid() bool {
	switch s {
	case DecisionStatusActive, DecisionStatusSuperseded,
		DecisionStatusContradicted, DecisionStatusArchived:
		return true
	}
	return false
}

// IngestMethod describes how a record entered the system.
type IngestMethod string

const (
	IngestMethodDirect          IngestMethod = "direct"
	IngestMethodDerived         IngestMethod = "derived"
	IngestMethodImported        IngestMethod = "imported"
	IngestMethodWorkerGenerated IngestMethod = "worker_generated"
)

func (m IngestMethod) IsValid() bool {
	switch m {
	case IngestMethodDirect, IngestMethodDerived,
		IngestMethodImported, IngestMethodWorkerGenerated:
		return true
	}
	return false
}

// ConfidenceSource describes who/what assigned the confidence score.
type ConfidenceSource string

const (
	ConfidenceSourceHuman    ConfidenceSource = "human"
	ConfidenceSourceAgent    ConfidenceSource = "agent"
	ConfidenceSourceDerived  ConfidenceSource = "derived"
	ConfidenceSourceComputed ConfidenceSource = "computed"
)

func (s ConfidenceSource) IsValid() bool {
	switch s {
	case ConfidenceSourceHuman, ConfidenceSourceAgent,
		ConfidenceSourceDerived, ConfidenceSourceComputed:
		return true
	}
	return false
}

// FreshnessLevel represents how fresh a record is.
type FreshnessLevel string

const (
	FreshnessLevelFresh   FreshnessLevel = "fresh"
	FreshnessLevelAging   FreshnessLevel = "aging"
	FreshnessLevelStale   FreshnessLevel = "stale"
	FreshnessLevelExpired FreshnessLevel = "expired"
)

func (f FreshnessLevel) IsValid() bool {
	switch f {
	case FreshnessLevelFresh, FreshnessLevelAging,
		FreshnessLevelStale, FreshnessLevelExpired:
		return true
	}
	return false
}

// EvidenceType describes what kind of evidence is referenced.
type EvidenceType string

const (
	EvidenceTypeMemoryRef     EvidenceType = "memory_ref"
	EvidenceTypeExternalLink  EvidenceType = "external_link"
	EvidenceTypeInlineExcerpt EvidenceType = "inline_excerpt"
)

func (e EvidenceType) IsValid() bool {
	switch e {
	case EvidenceTypeMemoryRef, EvidenceTypeExternalLink, EvidenceTypeInlineExcerpt:
		return true
	}
	return false
}

// RelationType defines valid relation types between records.
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

func (r PurgeReason) IsValid() bool {
	switch r {
	case PurgeReasonSecretLeak, PurgeReasonCompliance, PurgeReasonSensitiveContent:
		return true
	}
	return false
}

// PurgeStatus represents the lifecycle of a purge operation.
type PurgeStatus string

const (
	PurgeStatusPending   PurgeStatus = "pending"
	PurgeStatusExecuting PurgeStatus = "executing"
	PurgeStatusExecuted  PurgeStatus = "executed"
	PurgeStatusFailed    PurgeStatus = "failed"
)

func (s PurgeStatus) IsValid() bool {
	switch s {
	case PurgeStatusPending, PurgeStatusExecuting,
		PurgeStatusExecuted, PurgeStatusFailed:
		return true
	}
	return false
}

// EventType for domain events.
type EventType string

const (
	EventMemoryIngested       EventType = "memory.ingested"
	EventMemoryArchived       EventType = "memory.archived"
	EventDecisionRecorded     EventType = "decision.recorded"
	EventDecisionSuperseded   EventType = "decision.superseded"
	EventDecisionContradicted EventType = "decision.contradicted"
	EventHeuristicCreated     EventType = "heuristic.created"
	EventHeuristicToggled     EventType = "heuristic.toggled"
	EventRelationCreated      EventType = "relation.created"
	EventPurgeRequested       EventType = "purge.requested"
	EventPurgeExecuted        EventType = "purge.executed"
	EventProfileGenerated     EventType = "profile.generated"
)
```

- [ ] **Step 4: Implement errors**

```go
// internal/domain/shared/errors.go
package shared

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound          = errors.New("record not found")
	ErrPurged            = errors.New("record has been purged")
	ErrValidation        = errors.New("validation error")
	ErrAlreadyArchived   = errors.New("record already archived")
	ErrNotActive         = errors.New("record is not active")
	ErrConflict          = errors.New("business rule conflict")
	ErrDuplicateRelation = errors.New("relation already exists")
	ErrSourceNotFound    = errors.New("relation source not found")
	ErrTargetNotFound    = errors.New("relation target not found")
	ErrSourcePurged      = errors.New("relation source was purged")
	ErrTargetPurged      = errors.New("relation target was purged")
	ErrParentNotFound    = errors.New("parent record not found")
	ErrEvidenceRefNotFound = errors.New("evidence reference not found")
	ErrAlreadyExecuted   = errors.New("purge already executed")
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   any    `json:"value,omitempty"`
}

type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = fmt.Sprintf("%s: %s", f.Field, f.Message)
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(msgs, "; "))
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

func NewValidationError(fields ...FieldError) error {
	return &ValidationError{Fields: fields}
}
```

- [ ] **Step 5: Run tests**

Run: `cd /Users/russell/Documents/2026/sophia-memory-engine && go test ./internal/domain/shared/... -v`
Expected: PASS — all 3 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/shared/enums.go internal/domain/shared/errors.go internal/domain/shared/errors_test.go
git commit -m "feat: add domain enums and error types with structured validation"
```

---

## Task 3: Domain Shared — RecordID Value Object

**Files:**
- Create: `internal/domain/shared/record_id.go`
- Create: `internal/domain/shared/record_id_test.go`

- [ ] **Step 1: Write tests**

```go
// internal/domain/shared/record_id_test.go
package shared_test

import (
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRecordID_GeneratesValidULID(t *testing.T) {
	id := shared.NewRecordID()
	assert.Len(t, id.String(), 26)
	assert.True(t, id.IsValid())
}

func TestNewRecordID_IsUnique(t *testing.T) {
	id1 := shared.NewRecordID()
	id2 := shared.NewRecordID()
	assert.NotEqual(t, id1, id2)
}

func TestRecordIDFromString_Valid(t *testing.T) {
	original := shared.NewRecordID()
	parsed, err := shared.RecordIDFromString(original.String())
	require.NoError(t, err)
	assert.Equal(t, original, parsed)
}

func TestRecordIDFromString_Invalid(t *testing.T) {
	_, err := shared.RecordIDFromString("")
	assert.Error(t, err)

	_, err = shared.RecordIDFromString("not-a-ulid")
	assert.Error(t, err)
}

func TestRecordID_IsTimeSortable(t *testing.T) {
	id1 := shared.NewRecordID()
	id2 := shared.NewRecordID()
	// ULIDs created sequentially should sort in order
	assert.True(t, id1.String() <= id2.String())
}

func TestRecordID_ZeroValue_IsInvalid(t *testing.T) {
	var id shared.RecordID
	assert.False(t, id.IsValid())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/shared/... -run TestNewRecordID`
Expected: FAIL — types not defined.

- [ ] **Step 3: Implement RecordID**

```go
// internal/domain/shared/record_id.go
package shared

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// RecordID is a ULID-based identifier. Time-sortable, 26 chars, Crockford base32.
type RecordID struct {
	value ulid.ULID
}

func NewRecordID() RecordID {
	return RecordID{value: ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)}
}

func RecordIDFromString(s string) (RecordID, error) {
	if s == "" {
		return RecordID{}, fmt.Errorf("record id cannot be empty")
	}
	parsed, err := ulid.Parse(s)
	if err != nil {
		return RecordID{}, fmt.Errorf("invalid record id %q: %w", s, err)
	}
	return RecordID{value: parsed}, nil
}

func (r RecordID) String() string {
	return r.value.String()
}

func (r RecordID) IsValid() bool {
	return r.value.Time() > 0
}

func (r RecordID) IsZero() bool {
	return r.value.Time() == 0
}

func (r RecordID) Time() time.Time {
	return ulid.Time(r.value.Time())
}

// Equal compares two RecordIDs by value.
func (r RecordID) Equal(other RecordID) bool {
	return r.value == other.value
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/shared/... -run TestRecordID -v && go test ./internal/domain/shared/... -run TestNewRecordID -v`
Expected: PASS — all 6 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/shared/record_id.go internal/domain/shared/record_id_test.go
git commit -m "feat: add RecordID value object with ULID generation"
```

---

## Task 4: Domain Shared — Scope, Provenance, TimeRange

**Files:**
- Create: `internal/domain/shared/scope.go`
- Create: `internal/domain/shared/scope_test.go`
- Create: `internal/domain/shared/provenance.go`
- Create: `internal/domain/shared/provenance_test.go`
- Create: `internal/domain/shared/timerange.go`
- Create: `internal/domain/shared/timerange_test.go`

- [ ] **Step 1: Write Scope tests**

```go
// internal/domain/shared/scope_test.go
package shared_test

import (
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestNewScope_ProjectIDRequired(t *testing.T) {
	_, err := shared.NewScope("")
	assert.Error(t, err)
}

func TestNewScope_MinimalValid(t *testing.T) {
	s, err := shared.NewScope("proj-1")
	assert.NoError(t, err)
	assert.Equal(t, "proj-1", s.ProjectID)
	assert.Nil(t, s.TenantID)
	assert.Nil(t, s.RepoID)
}

func TestScope_WithOptionalFields(t *testing.T) {
	s, _ := shared.NewScope("proj-1",
		shared.WithRepoID("repo-a"),
		shared.WithAgentID("coder-1"),
		shared.WithEnvironment("production"),
	)
	assert.Equal(t, "repo-a", *s.RepoID)
	assert.Equal(t, "coder-1", *s.AgentID)
	assert.Equal(t, "production", *s.Environment)
}

func TestScope_Matches_NilFieldMatchesAll(t *testing.T) {
	filter, _ := shared.NewScope("proj-1")
	record, _ := shared.NewScope("proj-1", shared.WithRepoID("repo-a"))
	assert.True(t, filter.Matches(record))
}

func TestScope_Matches_ExactFieldRequired(t *testing.T) {
	filter, _ := shared.NewScope("proj-1", shared.WithRepoID("repo-a"))
	recordMatch, _ := shared.NewScope("proj-1", shared.WithRepoID("repo-a"))
	recordNoMatch, _ := shared.NewScope("proj-1", shared.WithRepoID("repo-b"))
	assert.True(t, filter.Matches(recordMatch))
	assert.False(t, filter.Matches(recordNoMatch))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/shared/... -run TestScope`
Expected: FAIL.

- [ ] **Step 3: Implement Scope**

```go
// internal/domain/shared/scope.go
package shared

import "fmt"

type Scope struct {
	TenantID    *string
	ProjectID   string
	RepoID      *string
	AgentID     *string
	SessionID   *string
	Environment *string
}

type ScopeOption func(*Scope)

func WithTenantID(id string) ScopeOption    { return func(s *Scope) { s.TenantID = &id } }
func WithRepoID(id string) ScopeOption      { return func(s *Scope) { s.RepoID = &id } }
func WithAgentID(id string) ScopeOption     { return func(s *Scope) { s.AgentID = &id } }
func WithSessionID(id string) ScopeOption   { return func(s *Scope) { s.SessionID = &id } }
func WithEnvironment(env string) ScopeOption { return func(s *Scope) { s.Environment = &env } }

func NewScope(projectID string, opts ...ScopeOption) (Scope, error) {
	if projectID == "" {
		return Scope{}, fmt.Errorf("project_id is required")
	}
	s := Scope{ProjectID: projectID}
	for _, opt := range opts {
		opt(&s)
	}
	return s, nil
}

// Matches returns true if this scope (as a filter) matches the given record scope.
// Nil filter fields match everything. Present filter fields require exact match.
func (s Scope) Matches(record Scope) bool {
	if s.ProjectID != record.ProjectID {
		return false
	}
	if s.TenantID != nil && (record.TenantID == nil || *s.TenantID != *record.TenantID) {
		return false
	}
	if s.RepoID != nil && (record.RepoID == nil || *s.RepoID != *record.RepoID) {
		return false
	}
	if s.AgentID != nil && (record.AgentID == nil || *s.AgentID != *record.AgentID) {
		return false
	}
	if s.SessionID != nil && (record.SessionID == nil || *s.SessionID != *record.SessionID) {
		return false
	}
	if s.Environment != nil && (record.Environment == nil || *s.Environment != *record.Environment) {
		return false
	}
	return true
}
```

- [ ] **Step 4: Run Scope tests**

Run: `go test ./internal/domain/shared/... -run TestScope -v`
Expected: PASS.

- [ ] **Step 5: Write Provenance tests**

```go
// internal/domain/shared/provenance_test.go
package shared_test

import (
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestNewProvenance_SourceRequired(t *testing.T) {
	_, err := shared.NewProvenance("", shared.IngestMethodDirect, nil)
	assert.Error(t, err)
}

func TestNewProvenance_DerivedRequiresParent(t *testing.T) {
	_, err := shared.NewProvenance("agent:coder-1", shared.IngestMethodDerived, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parent_id")
}

func TestNewProvenance_DerivedWithParent(t *testing.T) {
	parentID := shared.NewRecordID()
	p, err := shared.NewProvenance("agent:coder-1", shared.IngestMethodDerived, &parentID)
	assert.NoError(t, err)
	assert.Equal(t, &parentID, p.ParentID)
}

func TestNewProvenance_DirectNoParent(t *testing.T) {
	p, err := shared.NewProvenance("user:russell", shared.IngestMethodDirect, nil)
	assert.NoError(t, err)
	assert.Equal(t, "user:russell", p.Source)
	assert.Equal(t, shared.IngestMethodDirect, p.Method)
}

func TestNewProvenance_InvalidMethod(t *testing.T) {
	_, err := shared.NewProvenance("agent:x", shared.IngestMethod("invalid"), nil)
	assert.Error(t, err)
}
```

- [ ] **Step 6: Implement Provenance**

```go
// internal/domain/shared/provenance.go
package shared

import "fmt"

type Provenance struct {
	Source    string
	SourceURI *string
	Method   IngestMethod
	ParentID *RecordID
}

func NewProvenance(source string, method IngestMethod, parentID *RecordID) (Provenance, error) {
	if source == "" {
		return Provenance{}, fmt.Errorf("source is required")
	}
	if !method.IsValid() {
		return Provenance{}, fmt.Errorf("invalid ingest method: %s", method)
	}
	if method == IngestMethodDerived && parentID == nil {
		return Provenance{}, fmt.Errorf("parent_id is required when method is derived")
	}
	return Provenance{
		Source:   source,
		Method:   method,
		ParentID: parentID,
	}, nil
}

func (p Provenance) WithSourceURI(uri string) Provenance {
	p.SourceURI = &uri
	return p
}
```

- [ ] **Step 7: Write TimeRange tests and implement**

```go
// internal/domain/shared/timerange_test.go
package shared_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestNewTimeRange_Valid(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	tr, err := shared.NewTimeRange(from, to)
	assert.NoError(t, err)
	assert.Equal(t, from, tr.From)
	assert.Equal(t, to, tr.To)
}

func TestNewTimeRange_FromAfterTo(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := shared.NewTimeRange(from, to)
	assert.Error(t, err)
}

func TestNewTimeRange_Equal(t *testing.T) {
	now := time.Now()
	tr, err := shared.NewTimeRange(now, now)
	assert.NoError(t, err)
	assert.Equal(t, now, tr.From)
}
```

```go
// internal/domain/shared/timerange.go
package shared

import (
	"fmt"
	"time"
)

type TimeRange struct {
	From time.Time
	To   time.Time
}

func NewTimeRange(from, to time.Time) (TimeRange, error) {
	if from.After(to) {
		return TimeRange{}, fmt.Errorf("from (%v) must be before or equal to to (%v)", from, to)
	}
	return TimeRange{From: from, To: to}, nil
}

func (r TimeRange) Contains(t time.Time) bool {
	return !t.Before(r.From) && !t.After(r.To)
}
```

- [ ] **Step 8: Run all tests**

Run: `go test ./internal/domain/shared/... -v`
Expected: PASS — all Scope, Provenance, TimeRange tests.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/shared/scope.go internal/domain/shared/scope_test.go \
  internal/domain/shared/provenance.go internal/domain/shared/provenance_test.go \
  internal/domain/shared/timerange.go internal/domain/shared/timerange_test.go
git commit -m "feat: add Scope, Provenance, and TimeRange value objects"
```

---

## Task 5: Domain Shared — TemporalMetadata, Confidence, ImportanceScore, EvidenceRef

**Files:**
- Create: `internal/domain/shared/temporal.go`
- Create: `internal/domain/shared/temporal_test.go`
- Create: `internal/domain/shared/confidence.go`
- Create: `internal/domain/shared/confidence_test.go`
- Create: `internal/domain/shared/importance.go`
- Create: `internal/domain/shared/importance_test.go`
- Create: `internal/domain/shared/evidence.go`
- Create: `internal/domain/shared/evidence_test.go`

- [ ] **Step 1: Write TemporalMetadata tests**

```go
// internal/domain/shared/temporal_test.go
package shared_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestTemporalMetadata_ComputeFreshness_Expired(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	past := clock.Now().Add(-1 * time.Hour)
	tm := shared.TemporalMetadata{ValidUntil: &past}
	cfg := shared.DefaultFreshnessConfig()
	assert.Equal(t, shared.FreshnessLevelExpired, tm.ComputeFreshness(cfg, clock))
}

func TestTemporalMetadata_ComputeFreshness_Stale(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	accessed := clock.Now().Add(-45 * 24 * time.Hour)
	tm := shared.TemporalMetadata{LastAccessed: &accessed}
	cfg := shared.DefaultFreshnessConfig()
	assert.Equal(t, shared.FreshnessLevelStale, tm.ComputeFreshness(cfg, clock))
}

func TestTemporalMetadata_ComputeFreshness_Aging(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	accessed := clock.Now().Add(-10 * 24 * time.Hour)
	tm := shared.TemporalMetadata{LastAccessed: &accessed}
	cfg := shared.DefaultFreshnessConfig()
	assert.Equal(t, shared.FreshnessLevelAging, tm.ComputeFreshness(cfg, clock))
}

func TestTemporalMetadata_ComputeFreshness_Fresh(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	accessed := clock.Now().Add(-1 * 24 * time.Hour)
	tm := shared.TemporalMetadata{LastAccessed: &accessed}
	cfg := shared.DefaultFreshnessConfig()
	assert.Equal(t, shared.FreshnessLevelFresh, tm.ComputeFreshness(cfg, clock))
}

func TestTemporalMetadata_ComputeFreshness_Timeless(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	tm := shared.TemporalMetadata{} // no ValidUntil, no LastAccessed
	cfg := shared.DefaultFreshnessConfig()
	assert.Equal(t, shared.FreshnessLevelFresh, tm.ComputeFreshness(cfg, clock))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/shared/... -run TestTemporalMetadata`
Expected: FAIL.

- [ ] **Step 3: Implement TemporalMetadata with Clock**

```go
// internal/domain/shared/temporal.go
package shared

import "time"

// Clock is the port for time — enables deterministic testing.
type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type FixedClock struct {
	FixedTime time.Time
}

func NewFixedClock(t time.Time) *FixedClock {
	return &FixedClock{FixedTime: t}
}

func (c *FixedClock) Now() time.Time { return c.FixedTime }

func (c *FixedClock) Advance(d time.Duration) {
	c.FixedTime = c.FixedTime.Add(d)
}

// TemporalMetadata captures validity windows and freshness state.
type TemporalMetadata struct {
	ValidFrom    *time.Time
	ValidUntil   *time.Time
	LastAccessed *time.Time
	Freshness    FreshnessLevel
}

type FreshnessConfig struct {
	AgingThreshold time.Duration
	StaleThreshold time.Duration
}

func DefaultFreshnessConfig() FreshnessConfig {
	return FreshnessConfig{
		AgingThreshold: 7 * 24 * time.Hour,
		StaleThreshold: 30 * 24 * time.Hour,
	}
}

func (tm TemporalMetadata) ComputeFreshness(cfg FreshnessConfig, clock Clock) FreshnessLevel {
	now := clock.Now()

	// Expired takes priority: ValidUntil in the past
	if tm.ValidUntil != nil && tm.ValidUntil.Before(now) {
		return FreshnessLevelExpired
	}

	// If never accessed and no expiry, it's fresh
	if tm.LastAccessed == nil {
		return FreshnessLevelFresh
	}

	age := now.Sub(*tm.LastAccessed)
	if age > cfg.StaleThreshold {
		return FreshnessLevelStale
	}
	if age > cfg.AgingThreshold {
		return FreshnessLevelAging
	}
	return FreshnessLevelFresh
}

func (tm TemporalMetadata) IsExpired(clock Clock) bool {
	return tm.ValidUntil != nil && tm.ValidUntil.Before(clock.Now())
}
```

- [ ] **Step 4: Run TemporalMetadata tests**

Run: `go test ./internal/domain/shared/... -run TestTemporalMetadata -v`
Expected: PASS — all 5 tests.

- [ ] **Step 5: Write and implement Confidence**

```go
// internal/domain/shared/confidence_test.go
package shared_test

import (
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestNewConfidence_Valid(t *testing.T) {
	c, err := shared.NewConfidence(0.85, shared.ConfidenceSourceHuman)
	assert.NoError(t, err)
	assert.Equal(t, 0.85, c.Score)
}

func TestNewConfidence_ClampsAboveOne(t *testing.T) {
	_, err := shared.NewConfidence(1.5, shared.ConfidenceSourceAgent)
	assert.Error(t, err)
}

func TestNewConfidence_ClampsBelowZero(t *testing.T) {
	_, err := shared.NewConfidence(-0.1, shared.ConfidenceSourceAgent)
	assert.Error(t, err)
}

func TestNewConfidence_InvalidSource(t *testing.T) {
	_, err := shared.NewConfidence(0.5, shared.ConfidenceSource("invalid"))
	assert.Error(t, err)
}

func TestNewConfidence_BoundaryValues(t *testing.T) {
	c0, err := shared.NewConfidence(0.0, shared.ConfidenceSourceComputed)
	assert.NoError(t, err)
	assert.Equal(t, 0.0, c0.Score)

	c1, err := shared.NewConfidence(1.0, shared.ConfidenceSourceComputed)
	assert.NoError(t, err)
	assert.Equal(t, 1.0, c1.Score)
}
```

```go
// internal/domain/shared/confidence.go
package shared

import "fmt"

type Confidence struct {
	Score  float64
	Source ConfidenceSource
}

func NewConfidence(score float64, source ConfidenceSource) (Confidence, error) {
	if score < 0.0 || score > 1.0 {
		return Confidence{}, fmt.Errorf("confidence score must be between 0.0 and 1.0, got %f", score)
	}
	if !source.IsValid() {
		return Confidence{}, fmt.Errorf("invalid confidence source: %s", source)
	}
	return Confidence{Score: score, Source: source}, nil
}
```

- [ ] **Step 6: Write and implement ImportanceScore**

```go
// internal/domain/shared/importance_test.go
package shared_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestNewImportanceScore_Valid(t *testing.T) {
	factors := []shared.ImportanceFactor{
		{Name: "recency", Weight: 0.5, Value: 0.8},
		{Name: "access_count", Weight: 0.5, Value: 0.6},
	}
	s, err := shared.NewImportanceScore(0.7, time.Now(), factors)
	assert.NoError(t, err)
	assert.Equal(t, 0.7, s.Score)
	assert.Len(t, s.Factors, 2)
}

func TestNewImportanceScore_OutOfRange(t *testing.T) {
	_, err := shared.NewImportanceScore(1.5, time.Now(), nil)
	assert.Error(t, err)
}

func TestDefaultImportanceScore(t *testing.T) {
	s := shared.DefaultImportanceScore(time.Now())
	assert.Equal(t, 0.5, s.Score)
}
```

```go
// internal/domain/shared/importance.go
package shared

import (
	"fmt"
	"time"
)

type ImportanceFactor struct {
	Name   string
	Weight float64
	Value  float64
}

type ImportanceScore struct {
	Score      float64
	ComputedAt time.Time
	Factors    []ImportanceFactor
}

func NewImportanceScore(score float64, computedAt time.Time, factors []ImportanceFactor) (ImportanceScore, error) {
	if score < 0.0 || score > 1.0 {
		return ImportanceScore{}, fmt.Errorf("importance score must be between 0.0 and 1.0, got %f", score)
	}
	return ImportanceScore{
		Score:      score,
		ComputedAt: computedAt,
		Factors:    factors,
	}, nil
}

func DefaultImportanceScore(now time.Time) ImportanceScore {
	return ImportanceScore{
		Score:      0.5,
		ComputedAt: now,
		Factors: []ImportanceFactor{
			{Name: "default", Weight: 1.0, Value: 0.5},
		},
	}
}
```

- [ ] **Step 7: Write and implement EvidenceRef**

```go
// internal/domain/shared/evidence_test.go
package shared_test

import (
	"testing"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

func TestNewEvidenceRef_MemoryRef(t *testing.T) {
	id := shared.NewRecordID()
	ref, err := shared.NewEvidenceRef(shared.EvidenceTypeMemoryRef, &id, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, &id, ref.RecordID)
}

func TestNewEvidenceRef_MemoryRef_RequiresRecordID(t *testing.T) {
	_, err := shared.NewEvidenceRef(shared.EvidenceTypeMemoryRef, nil, nil, nil)
	assert.Error(t, err)
}

func TestNewEvidenceRef_ExternalLink_RequiresURI(t *testing.T) {
	_, err := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, nil, nil)
	assert.Error(t, err)
}

func TestNewEvidenceRef_ExternalLink_Valid(t *testing.T) {
	uri := "https://github.com/org/repo/pull/42"
	ref, err := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, &uri, nil)
	assert.NoError(t, err)
	assert.Equal(t, &uri, ref.URI)
}

func TestNewEvidenceRef_InlineExcerpt_RequiresExcerpt(t *testing.T) {
	_, err := shared.NewEvidenceRef(shared.EvidenceTypeInlineExcerpt, nil, nil, nil)
	assert.Error(t, err)
}
```

```go
// internal/domain/shared/evidence.go
package shared

import "fmt"

type EvidenceRef struct {
	RecordID *RecordID
	URI      *string
	Excerpt  *string
	Type     EvidenceType
}

func NewEvidenceRef(evidenceType EvidenceType, recordID *RecordID, uri *string, excerpt *string) (EvidenceRef, error) {
	if !evidenceType.IsValid() {
		return EvidenceRef{}, fmt.Errorf("invalid evidence type: %s", evidenceType)
	}
	switch evidenceType {
	case EvidenceTypeMemoryRef:
		if recordID == nil {
			return EvidenceRef{}, fmt.Errorf("record_id required for memory_ref evidence")
		}
	case EvidenceTypeExternalLink:
		if uri == nil || *uri == "" {
			return EvidenceRef{}, fmt.Errorf("uri required for external_link evidence")
		}
	case EvidenceTypeInlineExcerpt:
		if excerpt == nil || *excerpt == "" {
			return EvidenceRef{}, fmt.Errorf("excerpt required for inline_excerpt evidence")
		}
	}
	return EvidenceRef{
		RecordID: recordID,
		URI:      uri,
		Excerpt:  excerpt,
		Type:     evidenceType,
	}, nil
}
```

- [ ] **Step 8: Run all shared tests**

Run: `go test ./internal/domain/shared/... -v -count=1`
Expected: PASS — all value object tests (approx 30+ tests).

- [ ] **Step 9: Commit**

```bash
git add internal/domain/shared/temporal.go internal/domain/shared/temporal_test.go \
  internal/domain/shared/confidence.go internal/domain/shared/confidence_test.go \
  internal/domain/shared/importance.go internal/domain/shared/importance_test.go \
  internal/domain/shared/evidence.go internal/domain/shared/evidence_test.go
git commit -m "feat: add TemporalMetadata, Confidence, ImportanceScore, EvidenceRef value objects"
```

---

## Task 6: Domain Entities — MemoryRecord

**Files:**
- Create: `internal/domain/memory/memory.go`
- Create: `internal/domain/memory/memory_test.go`

- [ ] **Step 1: Write tests**

```go
// internal/domain/memory/memory_test.go
package memory_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validScope() shared.Scope {
	s, _ := shared.NewScope("proj-1")
	return s
}

func validProvenance() shared.Provenance {
	p, _ := shared.NewProvenance("agent:coder-1", shared.IngestMethodDirect, nil)
	return p
}

func TestNewMemoryRecord_Episodic_RequiresValidFrom(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	_, err := memory.NewMemoryRecord(
		shared.MemoryTypeEpisodic, "some content",
		validScope(), validProvenance(), clock,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "valid_from")
}

func TestNewMemoryRecord_Episodic_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	validFrom := clock.Now()
	m, err := memory.NewMemoryRecord(
		shared.MemoryTypeEpisodic, "bug found in auth",
		validScope(), validProvenance(), clock,
		memory.WithValidFrom(validFrom),
	)
	require.NoError(t, err)
	assert.Equal(t, shared.MemoryTypeEpisodic, m.Type)
	assert.Equal(t, shared.MemoryStatusActive, m.Status)
	assert.Equal(t, "bug found in auth", m.Content)
	assert.True(t, m.ID.IsValid())
}

func TestNewMemoryRecord_Semantic_ValidFromOptional(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	m, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "JWT tokens should be rotated",
		validScope(), validProvenance(), clock,
	)
	require.NoError(t, err)
	assert.Equal(t, shared.MemoryTypeSemantic, m.Type)
	assert.Nil(t, m.Temporal.ValidFrom)
}

func TestNewMemoryRecord_EmptyContent_Fails(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	_, err := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "",
		validScope(), validProvenance(), clock,
	)
	assert.Error(t, err)
}

func TestMemoryRecord_Archive(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "old pattern",
		validScope(), validProvenance(), clock,
	)
	err := m.Archive("user:russell", "no longer relevant")
	assert.NoError(t, err)
	assert.Equal(t, shared.MemoryStatusArchived, m.Status)
	assert.Equal(t, "user:russell", *m.ArchivedBy)
}

func TestMemoryRecord_Archive_AlreadyArchived(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "old pattern",
		validScope(), validProvenance(), clock,
	)
	_ = m.Archive("user:russell", "reason")
	err := m.Archive("user:russell", "reason again")
	assert.ErrorIs(t, err, shared.ErrAlreadyArchived)
}

func TestMemoryRecord_Archive_Purged(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "secret content",
		validScope(), validProvenance(), clock,
	)
	m.MarkPurged()
	err := m.Archive("user:russell", "reason")
	assert.ErrorIs(t, err, shared.ErrPurged)
}

func TestMemoryRecord_MarkPurged_WipesContent(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "secret content",
		validScope(), validProvenance(), clock,
		memory.WithSummary("secret summary"),
		memory.WithTags([]string{"auth", "secret"}),
	)
	m.MarkPurged()
	assert.Equal(t, shared.MemoryStatusPurged, m.Status)
	assert.Equal(t, "", m.Content)
	assert.Nil(t, m.Summary)
	assert.Empty(t, m.Tags)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/memory/... -v`
Expected: FAIL.

- [ ] **Step 3: Implement MemoryRecord**

```go
// internal/domain/memory/memory.go
package memory

import (
	"fmt"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

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

type Option func(*MemoryRecord)

func WithValidFrom(t time.Time) Option {
	return func(m *MemoryRecord) { m.Temporal.ValidFrom = &t }
}

func WithValidUntil(t time.Time) Option {
	return func(m *MemoryRecord) { m.Temporal.ValidUntil = &t }
}

func WithSummary(s string) Option {
	return func(m *MemoryRecord) { m.Summary = &s }
}

func WithTags(tags []string) Option {
	return func(m *MemoryRecord) { m.Tags = tags }
}

func WithTopicKey(key string) Option {
	return func(m *MemoryRecord) { m.TopicKey = &key }
}

func WithFTSLanguage(lang string) Option {
	return func(m *MemoryRecord) { m.FTSLanguage = lang }
}

func NewMemoryRecord(
	memType shared.MemoryType,
	content string,
	scope shared.Scope,
	provenance shared.Provenance,
	clock shared.Clock,
	opts ...Option,
) (*MemoryRecord, error) {
	if content == "" {
		return nil, shared.NewValidationError(shared.FieldError{
			Field: "content", Message: "required",
		})
	}
	if !memType.IsValid() {
		return nil, shared.NewValidationError(shared.FieldError{
			Field: "type", Message: fmt.Sprintf("invalid memory type: %s", memType),
		})
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

	// Invariant: episodic must have ValidFrom
	if m.Type == shared.MemoryTypeEpisodic && m.Temporal.ValidFrom == nil {
		return nil, shared.NewValidationError(shared.FieldError{
			Field: "valid_from", Message: "required for episodic memories",
		})
	}

	return m, nil
}

func (m *MemoryRecord) Archive(by string, reason string) error {
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

func (m *MemoryRecord) MarkPurged() {
	m.Status = shared.MemoryStatusPurged
	m.Content = ""
	m.Summary = nil
	m.Tags = []string{}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/memory/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/memory/memory.go internal/domain/memory/memory_test.go
git commit -m "feat: add MemoryRecord entity with episodic/semantic invariants"
```

---

## Task 7: Domain Entities — Decision

**Files:**
- Create: `internal/domain/decision/decision.go`
- Create: `internal/domain/decision/decision_test.go`

- [ ] **Step 1: Write tests**

```go
// internal/domain/decision/decision_test.go
package decision_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validScope() shared.Scope {
	s, _ := shared.NewScope("proj-1")
	return s
}

func validProvenance() shared.Provenance {
	p, _ := shared.NewProvenance("agent:coder-1", shared.IngestMethodDirect, nil)
	return p
}

func validConfidence() shared.Confidence {
	c, _ := shared.NewConfidence(0.9, shared.ConfidenceSourceHuman)
	return c
}

func validEvidence() []shared.EvidenceRef {
	uri := "https://github.com/org/repo/pull/1"
	e, _ := shared.NewEvidenceRef(shared.EvidenceTypeExternalLink, nil, &uri, nil)
	return []shared.EvidenceRef{e}
}

func TestNewDecision_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, err := decision.NewDecision(
		"auth/token-strategy", "Use JWT with rotation",
		"We need stateless auth", "JWT allows horizontal scaling",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	require.NoError(t, err)
	assert.Equal(t, "auth/token-strategy", d.DecisionKey)
	assert.Equal(t, 1, d.Version)
	assert.Equal(t, shared.DecisionStatusActive, d.Status)
}

func TestNewDecision_RequiresEvidence(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	_, err := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		[]shared.EvidenceRef{}, validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	assert.Error(t, err)
}

func TestDecision_Supersede(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, _ := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	newID := shared.NewRecordID()
	err := d.Supersede(newID)
	require.NoError(t, err)
	assert.Equal(t, shared.DecisionStatusSuperseded, d.Status)
	assert.True(t, d.SupersededBy.Equal(newID))
}

func TestDecision_Supersede_NotActive(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, _ := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	_ = d.Supersede(shared.NewRecordID())
	err := d.Supersede(shared.NewRecordID())
	assert.ErrorIs(t, err, shared.ErrNotActive)
}

func TestDecision_Contradict(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, _ := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	err := d.Contradict()
	require.NoError(t, err)
	assert.Equal(t, shared.DecisionStatusContradicted, d.Status)
}

func TestDecision_Archive(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, _ := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	err := d.Archive()
	require.NoError(t, err)
	assert.Equal(t, shared.DecisionStatusArchived, d.Status)
}

func TestDecision_Archive_FromSuperseded(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, _ := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	_ = d.Supersede(shared.NewRecordID())
	err := d.Archive()
	require.NoError(t, err)
	assert.Equal(t, shared.DecisionStatusArchived, d.Status)
}

func TestDecision_NoTransitionToDeleted(t *testing.T) {
	// Decisions never have a "deleted" state — this tests the absence of such a method
	// Verified by: no Delete() method exists on Decision
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, _ := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	// The only valid transitions from active are: supersede, contradict, archive
	assert.Equal(t, shared.DecisionStatusActive, d.Status)
}

func TestDecision_CannotReactivate(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	d, _ := decision.NewDecision(
		"auth/key", "title", "desc", "rationale",
		validEvidence(), validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	_ = d.Archive()
	// Archived is terminal — no method to go back to active
	assert.Equal(t, shared.DecisionStatusArchived, d.Status)
	err := d.Archive()
	assert.ErrorIs(t, err, shared.ErrNotActive)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/decision/... -v`
Expected: FAIL.

- [ ] **Step 3: Implement Decision**

```go
// internal/domain/decision/decision.go
package decision

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type Decision struct {
	ID           shared.RecordID
	DecisionKey  string
	Version      int
	Title        string
	Description  string
	Rationale    string
	Evidence     []shared.EvidenceRef
	Scope        shared.Scope
	Provenance   shared.Provenance
	Temporal     shared.TemporalMetadata
	Confidence   shared.Confidence
	Status       shared.DecisionStatus
	SupersededBy *shared.RecordID
	FTSLanguage  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewDecision(
	key, title, description, rationale string,
	evidence []shared.EvidenceRef,
	scope shared.Scope,
	provenance shared.Provenance,
	confidence shared.Confidence,
	clock shared.Clock,
	version int,
) (*Decision, error) {
	var fields []shared.FieldError
	if key == "" {
		fields = append(fields, shared.FieldError{Field: "decision_key", Message: "required"})
	}
	if title == "" {
		fields = append(fields, shared.FieldError{Field: "title", Message: "required"})
	}
	if description == "" {
		fields = append(fields, shared.FieldError{Field: "description", Message: "required"})
	}
	if rationale == "" {
		fields = append(fields, shared.FieldError{Field: "rationale", Message: "required"})
	}
	if len(evidence) == 0 {
		fields = append(fields, shared.FieldError{Field: "evidence", Message: "at least one required"})
	}
	if len(fields) > 0 {
		return nil, shared.NewValidationError(fields...)
	}

	now := clock.Now()
	return &Decision{
		ID:          shared.NewRecordID(),
		DecisionKey: key,
		Version:     version,
		Title:       title,
		Description: description,
		Rationale:   rationale,
		Evidence:    evidence,
		Scope:       scope,
		Provenance:  provenance,
		Temporal:    shared.TemporalMetadata{Freshness: shared.FreshnessLevelFresh},
		Confidence:  confidence,
		Status:      shared.DecisionStatusActive,
		FTSLanguage: "spanish",
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (d *Decision) Supersede(by shared.RecordID) error {
	if d.Status != shared.DecisionStatusActive {
		return shared.ErrNotActive
	}
	d.Status = shared.DecisionStatusSuperseded
	d.SupersededBy = &by
	return nil
}

func (d *Decision) Contradict() error {
	if d.Status != shared.DecisionStatusActive {
		return shared.ErrNotActive
	}
	d.Status = shared.DecisionStatusContradicted
	return nil
}

func (d *Decision) Archive() error {
	switch d.Status {
	case shared.DecisionStatusActive, shared.DecisionStatusSuperseded, shared.DecisionStatusContradicted:
		d.Status = shared.DecisionStatusArchived
		return nil
	default:
		return shared.ErrNotActive
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/decision/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/decision/decision.go internal/domain/decision/decision_test.go
git commit -m "feat: add Decision entity with status transitions and invariants"
```

---

## Task 8: Domain Entities — HeuristicRule, Relation, PurgeRecord, ProjectProfile

**Files:**
- Create: `internal/domain/heuristic/heuristic.go` + `_test.go`
- Create: `internal/domain/relation/relation.go` + `_test.go`
- Create: `internal/domain/purge/purge.go` + `_test.go`
- Create: `internal/domain/projectprofile/profile.go` + `_test.go`

This task creates the remaining 4 domain entities. Each follows the same TDD pattern.

- [ ] **Step 1: Write HeuristicRule tests**

```go
// internal/domain/heuristic/heuristic_test.go
package heuristic_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validScope() shared.Scope {
	s, _ := shared.NewScope("proj-1")
	return s
}

func validProvenance() shared.Provenance {
	p, _ := shared.NewProvenance("agent:coder-1", shared.IngestMethodDirect, nil)
	return p
}

func validConfidence() shared.Confidence {
	c, _ := shared.NewConfidence(0.8, shared.ConfidenceSourceAgent)
	return c
}

func TestNewHeuristicRule_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	h, err := heuristic.NewHeuristicRule(
		"testing/always-integration", "When changing DB code", "Run integration tests",
		"Mocks missed a real migration bug last quarter",
		validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	require.NoError(t, err)
	assert.Equal(t, "testing/always-integration", h.HeuristicKey)
	assert.True(t, h.Enabled)
	assert.Equal(t, 1, h.Version)
}

func TestNewHeuristicRule_RequiresConditionActionRationale(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	_, err := heuristic.NewHeuristicRule(
		"key", "", "action", "rationale",
		validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	assert.Error(t, err)
}

func TestHeuristicRule_Toggle(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	h, _ := heuristic.NewHeuristicRule(
		"key", "cond", "action", "rationale",
		validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	assert.True(t, h.Enabled)
	h.SetEnabled(false)
	assert.False(t, h.Enabled)
	h.SetEnabled(true)
	assert.True(t, h.Enabled)
}

func TestHeuristicRule_IsActive_RespectsExpiry(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	expiry := clock.Now().Add(-1 * time.Hour)
	h, _ := heuristic.NewHeuristicRule(
		"key", "cond", "action", "rationale",
		validScope(), validProvenance(), validConfidence(), clock, 1,
		heuristic.WithValidUntil(expiry),
	)
	assert.False(t, h.IsActive(clock))
}

func TestHeuristicRule_IsActive_RespectsEnabled(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	h, _ := heuristic.NewHeuristicRule(
		"key", "cond", "action", "rationale",
		validScope(), validProvenance(), validConfidence(), clock, 1,
	)
	h.SetEnabled(false)
	assert.False(t, h.IsActive(clock))
}
```

- [ ] **Step 2: Implement HeuristicRule**

```go
// internal/domain/heuristic/heuristic.go
package heuristic

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type HeuristicRule struct {
	ID           shared.RecordID
	HeuristicKey string
	Version      int
	Condition    string
	Action       string
	Rationale    string
	Scope        shared.Scope
	Provenance   shared.Provenance
	Confidence   shared.Confidence
	Enabled      bool
	Temporal     shared.TemporalMetadata
	FTSLanguage  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Option func(*HeuristicRule)

func WithValidUntil(t time.Time) Option {
	return func(h *HeuristicRule) { h.Temporal.ValidUntil = &t }
}

func WithEnabled(enabled bool) Option {
	return func(h *HeuristicRule) { h.Enabled = enabled }
}

func WithFTSLanguage(lang string) Option {
	return func(h *HeuristicRule) { h.FTSLanguage = lang }
}

func NewHeuristicRule(
	key, condition, action, rationale string,
	scope shared.Scope,
	provenance shared.Provenance,
	confidence shared.Confidence,
	clock shared.Clock,
	version int,
	opts ...Option,
) (*HeuristicRule, error) {
	var fields []shared.FieldError
	if key == "" {
		fields = append(fields, shared.FieldError{Field: "heuristic_key", Message: "required"})
	}
	if condition == "" {
		fields = append(fields, shared.FieldError{Field: "condition", Message: "required"})
	}
	if action == "" {
		fields = append(fields, shared.FieldError{Field: "action", Message: "required"})
	}
	if rationale == "" {
		fields = append(fields, shared.FieldError{Field: "rationale", Message: "required"})
	}
	if len(fields) > 0 {
		return nil, shared.NewValidationError(fields...)
	}

	now := clock.Now()
	h := &HeuristicRule{
		ID:           shared.NewRecordID(),
		HeuristicKey: key,
		Version:      version,
		Condition:    condition,
		Action:       action,
		Rationale:    rationale,
		Scope:        scope,
		Provenance:   provenance,
		Confidence:   confidence,
		Enabled:      true,
		Temporal:     shared.TemporalMetadata{Freshness: shared.FreshnessLevelFresh},
		FTSLanguage:  "spanish",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h, nil
}

func (h *HeuristicRule) SetEnabled(enabled bool) {
	h.Enabled = enabled
}

func (h *HeuristicRule) IsActive(clock shared.Clock) bool {
	if !h.Enabled {
		return false
	}
	return !h.Temporal.IsExpired(clock)
}
```

- [ ] **Step 3: Write Relation tests and implement**

```go
// internal/domain/relation/relation_test.go
package relation_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRelation_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("proj-1")
	src := shared.NewRecordID()
	tgt := shared.NewRecordID()
	r, err := relation.NewRelation(src, tgt, shared.RelationTypeSupersedes, scope, clock)
	require.NoError(t, err)
	assert.True(t, r.ID.IsValid())
	assert.Equal(t, shared.RelationTypeSupersedes, r.Type)
}

func TestNewRelation_NoSelfReference(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("proj-1")
	id := shared.NewRecordID()
	_, err := relation.NewRelation(id, id, shared.RelationTypeRelatesTo, scope, clock)
	assert.Error(t, err)
}

func TestNewRelation_InvalidType(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("proj-1")
	_, err := relation.NewRelation(shared.NewRecordID(), shared.NewRecordID(), shared.RelationType("invalid"), scope, clock)
	assert.Error(t, err)
}
```

```go
// internal/domain/relation/relation.go
package relation

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type Relation struct {
	ID       shared.RecordID
	SourceID shared.RecordID
	TargetID shared.RecordID
	Type     shared.RelationType
	Metadata map[string]any
	Scope    shared.Scope
	Temporal shared.TemporalMetadata
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewRelation(
	sourceID, targetID shared.RecordID,
	relType shared.RelationType,
	scope shared.Scope,
	clock shared.Clock,
	opts ...func(*Relation),
) (*Relation, error) {
	if sourceID.Equal(targetID) {
		return nil, shared.NewValidationError(shared.FieldError{
			Field: "target_id", Message: "cannot reference self",
		})
	}
	if !relType.IsValid() {
		return nil, shared.NewValidationError(shared.FieldError{
			Field: "relation_type", Message: "invalid relation type",
		})
	}

	now := clock.Now()
	r := &Relation{
		ID:        shared.NewRecordID(),
		SourceID:  sourceID,
		TargetID:  targetID,
		Type:      relType,
		Metadata:  map[string]any{},
		Scope:     scope,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

func WithMetadata(m map[string]any) func(*Relation) {
	return func(r *Relation) { r.Metadata = m }
}

func WithValidFrom(t time.Time) func(*Relation) {
	return func(r *Relation) { r.Temporal.ValidFrom = &t }
}
```

- [ ] **Step 4: Write PurgeRecord tests and implement**

```go
// internal/domain/purge/purge_test.go
package purge_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPurgeRecord_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("proj-1")
	p, err := purge.NewPurgeRecord(
		shared.NewRecordID(), "memory",
		shared.PurgeReasonSecretLeak, "user:russell",
		"API key leaked in content", scope, clock,
	)
	require.NoError(t, err)
	assert.Equal(t, shared.PurgeStatusPending, p.Status)
}

func TestPurgeRecord_Execute(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("proj-1")
	p, _ := purge.NewPurgeRecord(
		shared.NewRecordID(), "memory",
		shared.PurgeReasonCompliance, "user:admin",
		"GDPR deletion request", scope, clock,
	)
	artifacts := purge.PurgedArtifacts{
		FTSInvalidated: true, RelationsRemoved: 3,
	}
	err := p.MarkExecuted(artifacts, clock)
	require.NoError(t, err)
	assert.Equal(t, shared.PurgeStatusExecuted, p.Status)
	assert.NotNil(t, p.ExecutedAt)
}

func TestPurgeRecord_Execute_AlreadyExecuted(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("proj-1")
	p, _ := purge.NewPurgeRecord(
		shared.NewRecordID(), "memory",
		shared.PurgeReasonCompliance, "user:admin",
		"reason", scope, clock,
	)
	_ = p.MarkExecuted(purge.PurgedArtifacts{}, clock)
	err := p.MarkExecuted(purge.PurgedArtifacts{}, clock)
	assert.ErrorIs(t, err, shared.ErrAlreadyExecuted)
}
```

```go
// internal/domain/purge/purge.go
package purge

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type PurgedArtifacts struct {
	FTSInvalidated       bool
	EmbeddingInvalidated bool
	CacheInvalidated     bool
	RelationsRemoved     int
	DerivedInvalidated   []shared.RecordID
}

type PurgeRecord struct {
	ID              shared.RecordID
	TargetID        shared.RecordID
	TargetType      string
	Reason          shared.PurgeReason
	RequestedBy     string
	Scope           shared.Scope
	Status          shared.PurgeStatus
	AuditNote       string
	ArtifactsPurged PurgedArtifacts
	ExecutedAt      *time.Time
	ErrorDetail     *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewPurgeRecord(
	targetID shared.RecordID,
	targetType string,
	reason shared.PurgeReason,
	requestedBy string,
	auditNote string,
	scope shared.Scope,
	clock shared.Clock,
) (*PurgeRecord, error) {
	var fields []shared.FieldError
	if !reason.IsValid() {
		fields = append(fields, shared.FieldError{Field: "reason", Message: "invalid purge reason"})
	}
	if requestedBy == "" {
		fields = append(fields, shared.FieldError{Field: "requested_by", Message: "required"})
	}
	if auditNote == "" {
		fields = append(fields, shared.FieldError{Field: "audit_note", Message: "required"})
	}
	if len(fields) > 0 {
		return nil, shared.NewValidationError(fields...)
	}

	now := clock.Now()
	return &PurgeRecord{
		ID:          shared.NewRecordID(),
		TargetID:    targetID,
		TargetType:  targetType,
		Reason:      reason,
		RequestedBy: requestedBy,
		Scope:       scope,
		Status:      shared.PurgeStatusPending,
		AuditNote:   auditNote,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (p *PurgeRecord) MarkExecuting() error {
	if p.Status != shared.PurgeStatusPending {
		return shared.ErrNotActive
	}
	p.Status = shared.PurgeStatusExecuting
	return nil
}

func (p *PurgeRecord) MarkExecuted(artifacts PurgedArtifacts, clock shared.Clock) error {
	if p.Status == shared.PurgeStatusExecuted {
		return shared.ErrAlreadyExecuted
	}
	now := clock.Now()
	p.Status = shared.PurgeStatusExecuted
	p.ArtifactsPurged = artifacts
	p.ExecutedAt = &now
	return nil
}

func (p *PurgeRecord) MarkFailed(detail string) {
	p.Status = shared.PurgeStatusFailed
	p.ErrorDetail = &detail
}
```

- [ ] **Step 5: Write ProjectProfile tests and implement**

```go
// internal/domain/projectprofile/profile_test.go
package projectprofile_test

import (
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/projectprofile"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProjectProfile_Valid(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("proj-1")
	tr, _ := shared.NewTimeRange(
		clock.Now().Add(-7*24*time.Hour),
		clock.Now(),
	)
	p, err := projectprofile.NewProjectProfile(
		"proj-1", "Project uses hexagonal architecture with Go",
		scope, tr, clock, 1,
	)
	require.NoError(t, err)
	assert.Equal(t, "proj-1", p.ProjectID)
	assert.Equal(t, 1, p.Version)
	assert.False(t, p.Freshness.StaleAfter.IsZero())
}
```

```go
// internal/domain/projectprofile/profile.go
package projectprofile

import (
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type PatternEntry struct {
	Pattern    string
	Frequency  int
	LastSeenAt time.Time
	Confidence shared.Confidence
}

type SourceCounts struct {
	EpisodicConsidered   int
	SemanticConsidered   int
	DecisionsConsidered  int
	HeuristicsConsidered int
}

type FreshnessState struct {
	GeneratedAt     time.Time
	SourceTimeRange shared.TimeRange
	StaleAfter      time.Time
	SourceCounts    SourceCounts
}

type ProjectProfile struct {
	ID              shared.RecordID
	ProjectID       string
	Version         int
	Summary         string
	ActiveDecisions []shared.RecordID
	TopHeuristics   []shared.RecordID
	Patterns        []PatternEntry
	ArchSignals     []string
	Freshness       FreshnessState
	Scope           shared.Scope
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewProjectProfile(
	projectID, summary string,
	scope shared.Scope,
	sourceRange shared.TimeRange,
	clock shared.Clock,
	version int,
) (*ProjectProfile, error) {
	if projectID == "" {
		return nil, shared.NewValidationError(shared.FieldError{
			Field: "project_id", Message: "required",
		})
	}

	now := clock.Now()
	return &ProjectProfile{
		ID:              shared.NewRecordID(),
		ProjectID:       projectID,
		Version:         version,
		Summary:         summary,
		ActiveDecisions: []shared.RecordID{},
		TopHeuristics:   []shared.RecordID{},
		Patterns:        []PatternEntry{},
		ArchSignals:     []string{},
		Freshness: FreshnessState{
			GeneratedAt:     now,
			SourceTimeRange: sourceRange,
			StaleAfter:      now.Add(24 * time.Hour), // default: stale after 1 day
		},
		Scope:     scope,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
```

- [ ] **Step 6: Run all domain tests**

Run: `go test ./internal/domain/... -v -count=1`
Expected: PASS — all tests across all 6 aggregates + shared.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/heuristic/ internal/domain/relation/ internal/domain/purge/ internal/domain/projectprofile/
git commit -m "feat: add HeuristicRule, Relation, PurgeRecord, ProjectProfile entities"
```

---

## Task 9: Outbound Ports

**Files:**
- Create all files in `internal/ports/outbound/`

- [ ] **Step 1: Create repository interfaces**

```go
// internal/ports/outbound/memory_repository.go
package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type MemoryRepository interface {
	Save(ctx context.Context, record *memory.MemoryRecord) error
	FindByID(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error)
	UpdateStatus(ctx context.Context, id shared.RecordID, status shared.MemoryStatus) error
	WipeContent(ctx context.Context, id shared.RecordID) error
}
```

```go
// internal/ports/outbound/decision_repository.go
package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type DecisionRepository interface {
	Save(ctx context.Context, d *decision.Decision) error
	FindByID(ctx context.Context, id shared.RecordID) (*decision.Decision, error)
	FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*decision.Decision, error)
	FindByKey(ctx context.Context, key string, scope shared.Scope) ([]decision.Decision, error)
	UpdateStatus(ctx context.Context, id shared.RecordID, status shared.DecisionStatus, supersededBy *shared.RecordID) error
}
```

```go
// internal/ports/outbound/heuristic_repository.go
package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type HeuristicRepository interface {
	Save(ctx context.Context, rule *heuristic.HeuristicRule) error
	FindByID(ctx context.Context, id shared.RecordID) (*heuristic.HeuristicRule, error)
	FindActiveByKey(ctx context.Context, key string, scope shared.Scope) (*heuristic.HeuristicRule, error)
	FindByScope(ctx context.Context, scope shared.Scope, enabled *bool) ([]heuristic.HeuristicRule, error)
	UpdateEnabled(ctx context.Context, id shared.RecordID, enabled bool) error
}
```

```go
// internal/ports/outbound/relation_repository.go
package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type TraverseDirection string

const (
	TraverseOutbound TraverseDirection = "outbound"
	TraverseInbound  TraverseDirection = "inbound"
	TraverseBoth     TraverseDirection = "both"
)

type TraverseQuery struct {
	StartID         shared.RecordID
	Direction       TraverseDirection
	MaxDepth        int
	Types           []shared.RelationType
	Scope           *shared.Scope
	ValidAt         *time.Time
	ExcludeStatuses []string
}

type TraverseResult struct {
	Relation relation.Relation
	Depth    int
	Path     []shared.RecordID
}

type RelationRepository interface {
	Save(ctx context.Context, rel *relation.Relation) error
	FindFromSource(ctx context.Context, sourceID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error)
	FindToTarget(ctx context.Context, targetID shared.RecordID, relType *shared.RelationType) ([]relation.Relation, error)
	Traverse(ctx context.Context, query TraverseQuery) ([]TraverseResult, error)
	DeleteByTarget(ctx context.Context, targetID shared.RecordID) (int, error)
}
```

```go
// internal/ports/outbound/purge_repository.go
package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type PurgeRepository interface {
	Save(ctx context.Context, record *purge.PurgeRecord) error
	FindByID(ctx context.Context, id shared.RecordID) (*purge.PurgeRecord, error)
	UpdateStatus(ctx context.Context, id shared.RecordID, status shared.PurgeStatus, artifacts *purge.PurgedArtifacts) error
}
```

```go
// internal/ports/outbound/profile_repository.go
package outbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/projectprofile"
)

type ProjectProfileRepository interface {
	Save(ctx context.Context, profile *projectprofile.ProjectProfile) error
	FindLatest(ctx context.Context, projectID string) (*projectprofile.ProjectProfile, error)
}
```

```go
// internal/ports/outbound/search_index.go
package outbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

// SearchIndex is a read-model/indexing port. Not authoritative storage.
// Rebuildable entirely from aggregate repositories.
type SearchIndex interface {
	Index(ctx context.Context, entry SearchEntry) error
	Remove(ctx context.Context, id shared.RecordID) error
	Search(ctx context.Context, query FTSQuery) ([]FTSResult, error)
}

type SearchEntry struct {
	ID         shared.RecordID
	RecordType string
	Content    string
	Title      *string
	Tags       []string
	Scope      shared.Scope
	CreatedAt  time.Time
}

type FTSQuery struct {
	Text      string
	Scope     shared.Scope
	Types     []string
	TimeRange *shared.TimeRange
	Limit     int
	Offset    int
}

type FTSResult struct {
	ID         shared.RecordID
	RecordType string
	Rank       float64
	Snippet    string
}
```

```go
// internal/ports/outbound/embedding_generator.go
package outbound

import (
	"context"
	"errors"
)

var ErrEmbeddingsNotConfigured = errors.New("embeddings not configured")

// EmbeddingGenerator generates vector embeddings for text.
// Phase 1: stub implementation returns ErrEmbeddingsNotConfigured.
type EmbeddingGenerator interface {
	Generate(ctx context.Context, text string) ([]float64, error)
	BatchGenerate(ctx context.Context, texts []string) ([][]float64, error)
}
```

```go
// internal/ports/outbound/tx_manager.go
package outbound

import "context"

// TransactionManager wraps operations in a database transaction.
// The ctx passed to fn carries the transaction — repositories detect and use it.
type TransactionManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

```go
// internal/ports/outbound/clock.go
package outbound

import "github.com/sophia-engine/memory-engine/internal/domain/shared"

// Re-export Clock from shared domain for convenience.
// The actual Clock interface and implementations live in domain/shared
// since domain entities need it too.
type Clock = shared.Clock
```

```go
// internal/ports/outbound/event_publisher.go
package outbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type DomainEvent struct {
	ID            string
	Type          shared.EventType
	AggregateID   shared.RecordID
	AggregateType string
	Scope         shared.Scope
	Payload       any
	OccurredAt    time.Time
}

type EventHandler func(ctx context.Context, event DomainEvent) error

type EventPublisher interface {
	Publish(ctx context.Context, event DomainEvent) error
}
```

- [ ] **Step 2: Add missing import in relation_repository.go**

The `time` package needs to be imported in `relation_repository.go` for `TraverseQuery.ValidAt`.

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: Clean compilation.

- [ ] **Step 4: Commit**

```bash
git add internal/ports/outbound/
git commit -m "feat: add outbound port interfaces for all repositories and infrastructure"
```

---

## Task 10: Inbound Ports

**Files:**
- Create all files in `internal/ports/inbound/`

- [ ] **Step 1: Create inbound service interfaces with Cmd/Query/Result types**

```go
// internal/ports/inbound/memory_service.go
package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type IngestMemoryCmd struct {
	Type        shared.MemoryType
	Content     string
	Summary     *string
	Tags        []string
	TopicKey    *string
	FTSLanguage *string
	Scope       shared.Scope
	Provenance  shared.Provenance
	ValidFrom   *time.Time
	ValidUntil  *time.Time
}

type IngestMemoryResult struct {
	ID        shared.RecordID
	CreatedAt time.Time
}

type ArchiveMemoryCmd struct {
	ID          shared.RecordID
	Reason      string
	RequestedBy string
}

type MemoryService interface {
	Ingest(ctx context.Context, cmd IngestMemoryCmd) (*IngestMemoryResult, error)
	Get(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error)
	Archive(ctx context.Context, cmd ArchiveMemoryCmd) error
}
```

```go
// internal/ports/inbound/decision_service.go
package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/decision"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type RecordDecisionCmd struct {
	DecisionKey string
	Title       string
	Description string
	Rationale   string
	Evidence    []shared.EvidenceRef
	FTSLanguage *string
	Scope       shared.Scope
	Provenance  shared.Provenance
	Confidence  shared.Confidence
	ValidFrom   *time.Time
}

type RecordDecisionResult struct {
	ID         shared.RecordID
	Version    int
	Superseded *shared.RecordID
	CreatedAt  time.Time
}

type DecisionHistoryQuery struct {
	DecisionKey string
	Scope       shared.Scope
}

type ContradictDecisionCmd struct {
	TargetID       shared.RecordID
	ContradictedBy shared.RecordID
	Reason         string
	Provenance     shared.Provenance
}

type DecisionService interface {
	Record(ctx context.Context, cmd RecordDecisionCmd) (*RecordDecisionResult, error)
	Get(ctx context.Context, id shared.RecordID) (*decision.Decision, error)
	GetHistory(ctx context.Context, query DecisionHistoryQuery) ([]decision.Decision, error)
	Contradict(ctx context.Context, cmd ContradictDecisionCmd) error
}
```

```go
// internal/ports/inbound/heuristic_service.go
package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/heuristic"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type CreateHeuristicCmd struct {
	HeuristicKey string
	Condition    string
	Action       string
	Rationale    string
	FTSLanguage  *string
	Scope        shared.Scope
	Provenance   shared.Provenance
	Confidence   shared.Confidence
	Enabled      *bool
	ValidUntil   *time.Time
}

type CreateHeuristicResult struct {
	ID               shared.RecordID
	Version          int
	DisabledPrevious *shared.RecordID
	CreatedAt        time.Time
}

type GetActiveHeuristicQuery struct {
	HeuristicKey string
	Scope        shared.Scope
}

type ListHeuristicsQuery struct {
	Scope   shared.Scope
	Enabled *bool
}

type ToggleHeuristicCmd struct {
	ID      shared.RecordID
	Enabled bool
}

type HeuristicService interface {
	Create(ctx context.Context, cmd CreateHeuristicCmd) (*CreateHeuristicResult, error)
	GetActive(ctx context.Context, query GetActiveHeuristicQuery) (*heuristic.HeuristicRule, error)
	ListByScope(ctx context.Context, query ListHeuristicsQuery) ([]heuristic.HeuristicRule, error)
	Toggle(ctx context.Context, cmd ToggleHeuristicCmd) error
}
```

```go
// internal/ports/inbound/relation_service.go
package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/relation"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type CreateRelationCmd struct {
	SourceID  shared.RecordID
	TargetID  shared.RecordID
	Type      shared.RelationType
	Metadata  map[string]any
	Scope     shared.Scope
	ValidFrom *time.Time
}

type CreateRelationResult struct {
	ID        shared.RecordID
	CreatedAt time.Time
}

type RelationQuery struct {
	RecordID shared.RecordID
	Type     *shared.RelationType
	MaxDepth *int
	Scope    *shared.Scope
	ValidAt  *time.Time
}

type RelationResult struct {
	Relation relation.Relation
	Depth    int
	Path     []shared.RecordID
}

type RelationService interface {
	Create(ctx context.Context, cmd CreateRelationCmd) (*CreateRelationResult, error)
	GetFrom(ctx context.Context, query RelationQuery) ([]RelationResult, error)
	GetTo(ctx context.Context, query RelationQuery) ([]RelationResult, error)
}
```

```go
// internal/ports/inbound/retrieval_service.go
package inbound

import (
	"context"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type SearchQuery struct {
	Query     string
	Scope     shared.Scope
	Types     []string
	Freshness *shared.FreshnessLevel
	TimeRange *shared.TimeRange
	MinScore  *float64
	Limit     *int
	Offset    *int
}

type SearchResults struct {
	Results    []SearchResult
	TotalCount int
	Query      string
	Scope      shared.Scope
}

type SearchResult struct {
	ID         shared.RecordID
	RecordType string
	Title      string
	Snippet    string
	Score      float64
	Ranking    RankingExplanation
	Scope      shared.Scope
	Freshness  shared.FreshnessLevel
	CreatedAt  time.Time
}

type RankingExplanation struct {
	FTSScore       float64
	TRGMScore      float64
	RecencyBoost   float64
	ImportanceScore float64
	TypeBoost      float64
	FreshnessBoost float64
	ScopeExactness float64
	FinalScore     float64
}

type ContextRequest struct {
	Scope        shared.Scope
	Query        *string
	MaxTokens    *int
	IncludeTypes []string
	ExpandGraph  *bool
}

type ContextBundle struct {
	Sections    []ContextSection
	TotalTokens int
	Truncated   bool
	GeneratedAt time.Time
	ScopeUsed   shared.Scope
	DebugInfo   *ContextDebugInfo
}

type ContextSection struct {
	Type       string
	Records    []ContextRecord
	TokenCount int
}

type ContextRecord struct {
	ID       shared.RecordID
	Type     string
	Content  string
	Score    float64
	Relation *string
}

type ContextDebugInfo struct {
	Strategy        string
	RecordsScanned  int
	RecordsIncluded int
	GraphExpanded   bool
	GraphDepth      int
	TokenBudgetUsed int
	DurationMs      int64
}

type RetrievalService interface {
	Search(ctx context.Context, query SearchQuery) (*SearchResults, error)
	BuildContext(ctx context.Context, req ContextRequest) (*ContextBundle, error)
}
```

```go
// internal/ports/inbound/purge_service.go
package inbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/purge"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type RequestPurgeCmd struct {
	TargetID    shared.RecordID
	Reason      shared.PurgeReason
	RequestedBy string
	AuditNote   string
}

type ExecutePurgeCmd struct {
	PurgeID shared.RecordID
}

type PurgeService interface {
	Request(ctx context.Context, cmd RequestPurgeCmd) (*purge.PurgeRecord, error)
	Execute(ctx context.Context, cmd ExecutePurgeCmd) (*purge.PurgeRecord, error)
}
```

```go
// internal/ports/inbound/profile_service.go
package inbound

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/projectprofile"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type GenerateProfileCmd struct {
	ProjectID string
	Scope     shared.Scope
}

type GetProfileQuery struct {
	ProjectID string
	Scope     shared.Scope
}

type ProjectProfileService interface {
	Generate(ctx context.Context, cmd GenerateProfileCmd) (*projectprofile.ProjectProfile, error)
	Get(ctx context.Context, query GetProfileQuery) (*projectprofile.ProjectProfile, error)
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: Clean compilation.

- [ ] **Step 3: Commit**

```bash
git add internal/ports/inbound/
git commit -m "feat: add inbound port interfaces with command, query, and result types"
```

---

## Task 11: PostgreSQL Migrations

**Files:**
- Create: `migrations/postgres/001_initial_schema.up.sql`
- Create: `migrations/postgres/001_initial_schema.down.sql`

- [ ] **Step 1: Write the up migration**

Copy the full schema from the spec (section 6) into `migrations/postgres/001_initial_schema.up.sql`. This includes:
- Extension: `pg_trgm`
- All 7 tables: `memories`, `decisions`, `heuristics`, `relations`, `purge_records`, `project_profiles`, `domain_events`
- All indexes (FTS GIN, trigram GIN, scope composites, partial indexes)
- FTS trigger functions (using `fts_language` column per table)
- `set_updated_at()` trigger function + triggers on all mutable tables
- Check constraints and unique indexes as defined in spec

The full SQL is in the spec document sections on PostgreSQL schema — implement it verbatim.

- [ ] **Step 2: Write the down migration**

```sql
-- migrations/postgres/001_initial_schema.down.sql
DROP TABLE IF EXISTS domain_events CASCADE;
DROP TABLE IF EXISTS project_profiles CASCADE;
DROP TABLE IF EXISTS purge_records CASCADE;
DROP TABLE IF EXISTS relations CASCADE;
DROP TABLE IF EXISTS heuristics CASCADE;
DROP TABLE IF EXISTS decisions CASCADE;
DROP TABLE IF EXISTS memories CASCADE;

DROP FUNCTION IF EXISTS set_updated_at() CASCADE;
DROP FUNCTION IF EXISTS memories_search_vector_update() CASCADE;
DROP FUNCTION IF EXISTS decisions_search_vector_update() CASCADE;
DROP FUNCTION IF EXISTS heuristics_search_vector_update() CASCADE;

DROP EXTENSION IF EXISTS pg_trgm;
```

- [ ] **Step 3: Commit**

```bash
git add migrations/postgres/
git commit -m "feat: add initial PostgreSQL migration with all 7 tables and FTS indexes"
```

---

## Task 12: Database Infrastructure + Test Helpers

**Files:**
- Create: `internal/infrastructure/database/postgres.go`
- Create: `test/integration/testhelper/pg.go`
- Create: `internal/infrastructure/events/inprocess.go`
- Create: `internal/infrastructure/events/inprocess_test.go`
- Create: `internal/adapters/outbound/embeddings/noop.go`

- [ ] **Step 1: Create PostgreSQL connection helper**

```go
// internal/infrastructure/database/postgres.go
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
)

func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
```

- [ ] **Step 2: Create testcontainers helper**

```go
// test/integration/testhelper/pg.go
package testhelper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("memory_engine_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	runMigrations(t, pool)
	return pool
}

func runMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// Find migrations directory relative to project root
	migrationPath := findMigrationPath(t)
	sql, err := os.ReadFile(filepath.Join(migrationPath, "001_initial_schema.up.sql"))
	if err != nil {
		t.Fatalf("failed to read migration: %v", err)
	}
	if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
		t.Fatalf("failed to run migration: %v", err)
	}
}

func findMigrationPath(t *testing.T) string {
	t.Helper()
	// Walk up from test directory to find migrations/postgres/
	candidates := []string{
		"../../../migrations/postgres",
		"../../../../migrations/postgres",
		"migrations/postgres",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "001_initial_schema.up.sql")); err == nil {
			return c
		}
	}
	t.Fatal("could not find migrations directory")
	return ""
}
```

- [ ] **Step 3: Create InProcessEventPublisher**

```go
// internal/infrastructure/events/inprocess.go
package events

import (
	"context"
	"sync"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

type InProcessEventPublisher struct {
	mu       sync.RWMutex
	handlers map[shared.EventType][]outbound.EventHandler
}

func NewInProcessEventPublisher() *InProcessEventPublisher {
	return &InProcessEventPublisher{
		handlers: make(map[shared.EventType][]outbound.EventHandler),
	}
}

func (p *InProcessEventPublisher) Subscribe(eventType shared.EventType, handler outbound.EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

func (p *InProcessEventPublisher) Publish(ctx context.Context, event outbound.DomainEvent) error {
	p.mu.RLock()
	handlers := p.handlers[event.Type]
	p.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, event); err != nil {
			// Log but don't fail — events are at-most-once in phase 1
			continue
		}
	}
	return nil
}
```

- [ ] **Step 4: Test InProcessEventPublisher**

```go
// internal/infrastructure/events/inprocess_test.go
package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/events"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
	"github.com/stretchr/testify/assert"
)

func TestInProcessEventPublisher_PublishAndSubscribe(t *testing.T) {
	pub := events.NewInProcessEventPublisher()
	var received outbound.DomainEvent

	pub.Subscribe(shared.EventMemoryIngested, func(ctx context.Context, e outbound.DomainEvent) error {
		received = e
		return nil
	})

	event := outbound.DomainEvent{
		ID:            "test-1",
		Type:          shared.EventMemoryIngested,
		AggregateID:   shared.NewRecordID(),
		AggregateType: "memory",
		OccurredAt:    time.Now(),
	}
	err := pub.Publish(context.Background(), event)
	assert.NoError(t, err)
	assert.Equal(t, "test-1", received.ID)
}

func TestInProcessEventPublisher_NoSubscribers(t *testing.T) {
	pub := events.NewInProcessEventPublisher()
	event := outbound.DomainEvent{
		Type: shared.EventMemoryIngested,
	}
	err := pub.Publish(context.Background(), event)
	assert.NoError(t, err) // no error when no subscribers
}
```

- [ ] **Step 5: Create NoopEmbeddingGenerator**

```go
// internal/adapters/outbound/embeddings/noop.go
package embeddings

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

type NoopEmbeddingGenerator struct{}

func NewNoopEmbeddingGenerator() *NoopEmbeddingGenerator {
	return &NoopEmbeddingGenerator{}
}

func (n *NoopEmbeddingGenerator) Generate(ctx context.Context, text string) ([]float64, error) {
	return nil, outbound.ErrEmbeddingsNotConfigured
}

func (n *NoopEmbeddingGenerator) BatchGenerate(ctx context.Context, texts []string) ([][]float64, error) {
	return nil, outbound.ErrEmbeddingsNotConfigured
}
```

- [ ] **Step 6: Run tests and verify compilation**

Run: `go build ./... && go test ./internal/infrastructure/events/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/infrastructure/database/ test/integration/testhelper/ \
  internal/infrastructure/events/ internal/adapters/outbound/embeddings/
git commit -m "feat: add database pool, test helpers, event publisher, and noop embedding stub"
```

---

## Task 13: PostgreSQL Adapters — Memory Repository

**Files:**
- Create: `internal/adapters/outbound/persistence/helpers.go`
- Create: `internal/adapters/outbound/persistence/tx_manager_pg.go`
- Create: `internal/adapters/outbound/persistence/memory_pg.go`
- Create: `test/integration/memory_repository_test.go`

This is the first PG adapter. It establishes patterns (scope filter builder, scan helpers, tx context) that all other adapters follow.

- [ ] **Step 1: Create shared helpers and TX manager**

```go
// internal/adapters/outbound/persistence/helpers.go
package persistence

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

type ctxKey string

const txKey ctxKey = "pg_tx"

// DBTX is the interface that both *pgxpool.Pool and pgx.Tx implement.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getConn(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txKey).(pgx.Tx); ok {
		return tx
	}
	return pool
}

// ScopeFilter builds WHERE clause fragments for scope filtering.
type ScopeFilter struct {
	clauses []string
	args    []any
	argIdx  int
}

func NewScopeFilter(scope shared.Scope, startArgIdx int) *ScopeFilter {
	f := &ScopeFilter{argIdx: startArgIdx}
	f.add("project_id", scope.ProjectID)
	if scope.TenantID != nil {
		f.add("tenant_id", *scope.TenantID)
	}
	if scope.RepoID != nil {
		f.add("repo_id", *scope.RepoID)
	}
	if scope.AgentID != nil {
		f.add("agent_id", *scope.AgentID)
	}
	if scope.SessionID != nil {
		f.add("session_id", *scope.SessionID)
	}
	if scope.Environment != nil {
		f.add("environment", *scope.Environment)
	}
	return f
}

func (f *ScopeFilter) add(column string, value any) {
	f.argIdx++
	f.clauses = append(f.clauses, column+" = $"+itoa(f.argIdx))
	f.args = append(f.args, value)
}

func (f *ScopeFilter) SQL() string {
	return strings.Join(f.clauses, " AND ")
}

func (f *ScopeFilter) Args() []any {
	return f.args
}

func (f *ScopeFilter) NextArgIdx() int {
	return f.argIdx + 1
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
```

Note: you need to add `"fmt"` and `"github.com/jackc/pgx/v5/pgconn"` imports.

```go
// internal/adapters/outbound/persistence/tx_manager_pg.go
package persistence

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PgTxManager struct {
	pool *pgxpool.Pool
}

func NewPgTxManager(pool *pgxpool.Pool) *PgTxManager {
	return &PgTxManager{pool: pool}
}

func (m *PgTxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	txCtx := context.WithValue(ctx, txKey, tx)
	if err := fn(txCtx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 2: Implement MemoryPgRepository**

This is a substantial file. Implement `Save`, `FindByID`, `UpdateStatus`, `WipeContent` using the scope filter helper and tx-aware connection.

Key patterns:
- `Save` does an INSERT with all columns from MemoryRecord
- `FindByID` does a SELECT and scans into a MemoryRecord, returning `ErrNotFound` or `ErrPurged`
- `UpdateStatus` updates the `status` column
- `WipeContent` sets content='', summary=NULL, tags='{}', search_vector=NULL, status='purged'
- All methods use `getConn(ctx, pool)` to be transaction-aware

- [ ] **Step 3: Write integration tests**

```go
// test/integration/memory_repository_test.go
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/test/integration/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryRepository_SaveAndFindByID(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("test-project")
	prov, _ := shared.NewProvenance("agent:test", shared.IngestMethodDirect, nil)
	validFrom := clock.Now()

	m, err := memory.NewMemoryRecord(
		shared.MemoryTypeEpisodic, "test memory content",
		scope, prov, clock,
		memory.WithValidFrom(validFrom),
		memory.WithTags([]string{"test", "integration"}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	err = repo.Save(ctx, m)
	require.NoError(t, err)

	found, err := repo.FindByID(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, m.ID.String(), found.ID.String())
	assert.Equal(t, "test memory content", found.Content)
	assert.Equal(t, shared.MemoryStatusActive, found.Status)
}

func TestMemoryRepository_FindByID_NotFound(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)

	_, err := repo.FindByID(context.Background(), shared.NewRecordID())
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestMemoryRepository_FindByID_Purged(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("test-project")
	prov, _ := shared.NewProvenance("agent:test", shared.IngestMethodDirect, nil)

	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "secret content",
		scope, prov, clock,
	)
	ctx := context.Background()
	_ = repo.Save(ctx, m)
	_ = repo.WipeContent(ctx, m.ID)

	_, err := repo.FindByID(ctx, m.ID)
	assert.ErrorIs(t, err, shared.ErrPurged)
}

func TestMemoryRepository_WipeContent_Tombstone(t *testing.T) {
	pool := testhelper.SetupTestDB(t)
	repo := persistence.NewMemoryPgRepository(pool)
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	scope, _ := shared.NewScope("test-project")
	prov, _ := shared.NewProvenance("agent:test", shared.IngestMethodDirect, nil)

	m, _ := memory.NewMemoryRecord(
		shared.MemoryTypeSemantic, "content to purge",
		scope, prov, clock,
		memory.WithSummary("summary"),
		memory.WithTags([]string{"tag1"}),
	)
	ctx := context.Background()
	_ = repo.Save(ctx, m)
	err := repo.WipeContent(ctx, m.ID)
	require.NoError(t, err)

	// Verify tombstone via direct query (bypassing ErrPurged in FindByID)
	var content string
	var status string
	row := pool.QueryRow(ctx, "SELECT content, status FROM memories WHERE id = $1", m.ID.String())
	err = row.Scan(&content, &status)
	require.NoError(t, err)
	assert.Equal(t, "", content)
	assert.Equal(t, "purged", status)
}
```

- [ ] **Step 4: Run integration tests**

Run: `go test ./test/integration/... -tags=integration -run TestMemoryRepository -v -count=1`
Expected: PASS (requires Docker for testcontainers).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/outbound/persistence/ test/integration/memory_repository_test.go
git commit -m "feat: add PostgreSQL memory repository with integration tests"
```

---

## Tasks 14-17: Remaining PG Adapters

Follow the same pattern as Task 13 for each repository:

**Task 14: Decision PG Repository** — `Save`, `FindByID`, `FindActiveByKey`, `FindByKey`, `UpdateStatus`. Integration tests verify supersede constraint, version uniqueness, scope-aware history queries.

**Task 15: Heuristic PG Repository** — `Save`, `FindByID`, `FindActiveByKey`, `FindByScope`, `UpdateEnabled`. Integration tests verify active unique index, enabled filter, expiry semantics.

**Task 16: Relation PG Repository** — `Save`, `FindFromSource`, `FindToTarget`, `Traverse`, `DeleteByTarget`. Integration tests verify recursive CTE with depth bounds, cycle prevention, temporal filter, scope filter.

**Task 17: Purge + Profile PG Repositories** — Simpler implementations. Integration tests for purge lifecycle and profile versioning.

Each task follows: write integration test → implement adapter → verify → commit.

---

## Task 18: Application Service — IngestMemory

**Files:**
- Create: `internal/application/ingest/service.go`
- Create: `internal/application/ingest/service_test.go`

- [ ] **Step 1: Write tests with mocked ports**

```go
// internal/application/ingest/service_test.go
package ingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/sophia-engine/memory-engine/internal/application/ingest"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	// mock imports for outbound ports
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIngestService_Ingest_Episodic_Success(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	memRepo := &mockMemoryRepo{}
	searchIdx := &mockSearchIndex{}
	eventPub := &mockEventPublisher{}

	svc := ingest.NewService(memRepo, searchIdx, eventPub, clock)
	scope, _ := shared.NewScope("proj-1")
	prov, _ := shared.NewProvenance("agent:coder-1", shared.IngestMethodDirect, nil)
	validFrom := clock.Now()

	result, err := svc.Ingest(context.Background(), inbound.IngestMemoryCmd{
		Type:       shared.MemoryTypeEpisodic,
		Content:    "Found auth bug in middleware",
		Scope:      scope,
		Provenance: prov,
		ValidFrom:  &validFrom,
	})
	require.NoError(t, err)
	assert.True(t, result.ID.IsValid())
	assert.True(t, memRepo.saved)
	assert.True(t, eventPub.published)
	assert.Equal(t, shared.EventMemoryIngested, eventPub.lastEvent.Type)
}

func TestIngestService_Ingest_Episodic_MissingValidFrom(t *testing.T) {
	clock := shared.NewFixedClock(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC))
	svc := ingest.NewService(&mockMemoryRepo{}, &mockSearchIndex{}, &mockEventPublisher{}, clock)
	scope, _ := shared.NewScope("proj-1")
	prov, _ := shared.NewProvenance("agent:x", shared.IngestMethodDirect, nil)

	_, err := svc.Ingest(context.Background(), inbound.IngestMemoryCmd{
		Type: shared.MemoryTypeEpisodic, Content: "content",
		Scope: scope, Provenance: prov,
		// Missing ValidFrom
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, shared.ErrValidation)
}

// Mock implementations for outbound ports
// In practice, generate these with moq/mockgen. Shown inline for clarity.

type mockMemoryRepo struct {
	saved bool
}
func (m *mockMemoryRepo) Save(ctx context.Context, record *memory.MemoryRecord) error {
	m.saved = true; return nil
}
func (m *mockMemoryRepo) FindByID(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
	return nil, shared.ErrNotFound
}
func (m *mockMemoryRepo) UpdateStatus(ctx context.Context, id shared.RecordID, status shared.MemoryStatus) error {
	return nil
}
func (m *mockMemoryRepo) WipeContent(ctx context.Context, id shared.RecordID) error {
	return nil
}

// ... similar mocks for SearchIndex, EventPublisher
```

- [ ] **Step 2: Implement IngestService**

```go
// internal/application/ingest/service.go
package ingest

import (
	"context"

	"github.com/sophia-engine/memory-engine/internal/domain/memory"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
	"github.com/sophia-engine/memory-engine/internal/ports/outbound"
)

type Service struct {
	memRepo    outbound.MemoryRepository
	searchIdx  outbound.SearchIndex
	eventPub   outbound.EventPublisher
	clock      shared.Clock
}

func NewService(
	memRepo outbound.MemoryRepository,
	searchIdx outbound.SearchIndex,
	eventPub outbound.EventPublisher,
	clock shared.Clock,
) *Service {
	return &Service{
		memRepo: memRepo, searchIdx: searchIdx,
		eventPub: eventPub, clock: clock,
	}
}

func (s *Service) Ingest(ctx context.Context, cmd inbound.IngestMemoryCmd) (*inbound.IngestMemoryResult, error) {
	opts := []memory.Option{}
	if cmd.ValidFrom != nil {
		opts = append(opts, memory.WithValidFrom(*cmd.ValidFrom))
	}
	if cmd.ValidUntil != nil {
		opts = append(opts, memory.WithValidUntil(*cmd.ValidUntil))
	}
	if cmd.Summary != nil {
		opts = append(opts, memory.WithSummary(*cmd.Summary))
	}
	if len(cmd.Tags) > 0 {
		opts = append(opts, memory.WithTags(cmd.Tags))
	}
	if cmd.TopicKey != nil {
		opts = append(opts, memory.WithTopicKey(*cmd.TopicKey))
	}
	if cmd.FTSLanguage != nil {
		opts = append(opts, memory.WithFTSLanguage(*cmd.FTSLanguage))
	}

	record, err := memory.NewMemoryRecord(cmd.Type, cmd.Content, cmd.Scope, cmd.Provenance, s.clock, opts...)
	if err != nil {
		return nil, err
	}

	if err := s.memRepo.Save(ctx, record); err != nil {
		return nil, err
	}

	// Event published after successful persistence — at-most-once
	_ = s.eventPub.Publish(ctx, outbound.DomainEvent{
		ID:            shared.NewRecordID().String(),
		Type:          shared.EventMemoryIngested,
		AggregateID:   record.ID,
		AggregateType: "memory",
		Scope:         record.Scope,
		Payload:       map[string]any{"type": string(record.Type), "topic_key": cmd.TopicKey},
		OccurredAt:    record.CreatedAt,
	})

	return &inbound.IngestMemoryResult{
		ID: record.ID, CreatedAt: record.CreatedAt,
	}, nil
}

func (s *Service) Get(ctx context.Context, id shared.RecordID) (*memory.MemoryRecord, error) {
	return s.memRepo.FindByID(ctx, id)
}

func (s *Service) Archive(ctx context.Context, cmd inbound.ArchiveMemoryCmd) error {
	record, err := s.memRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if err := record.Archive(cmd.RequestedBy, cmd.Reason); err != nil {
		return err
	}
	return s.memRepo.UpdateStatus(ctx, cmd.ID, shared.MemoryStatusArchived)
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/application/ingest/... -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/application/ingest/
git commit -m "feat: add IngestMemory application service"
```

---

## Tasks 19-23: Remaining Application Services

**Task 19: Decision Application Service** — `Record` (with transactional supersede), `Get`, `GetHistory`, `Contradict`. Tests verify transactional consistency: supersede previous + create relation + create new in one tx.

**Task 20: Heuristic Application Service** — `Create` (with transactional disable of previous), `GetActive`, `ListByScope`, `Toggle`. Tests verify only one enabled per key+scope.

**Task 21: Relation Application Service** — `Create` (validates source/target exist), `GetFrom`, `GetTo`. Tests verify validation, depth capping.

**Task 22: Search Service** — Implements `RT1 Search`. Uses UNION ALL query via SearchIndex, computes scope exactness and ranking in Go. Tests verify ranking composition matches expected formula.

**Task 23: Context Builder Service** — Implements `RT2 BuildContext`. Fetches active decisions/heuristics by scope (always included), fetches memories by FTS query, allocates token budget, expands graph. Tests verify decisions always present, token budget respected.

Each task follows: write tests with mocked ports → implement → verify → commit.

---

## Task 24: HTTP Adapter

**Files:**
- Create: `internal/adapters/inbound/http/server.go`
- Create: `internal/adapters/inbound/http/responses.go`
- Create: `internal/adapters/inbound/http/middleware.go`
- Create: `internal/adapters/inbound/http/memory_handler.go`
- Create: `internal/adapters/inbound/http/decision_handler.go`
- Create: `internal/adapters/inbound/http/heuristic_handler.go`
- Create: `internal/adapters/inbound/http/relation_handler.go`
- Create: `internal/adapters/inbound/http/retrieval_handler.go`

- [ ] **Step 1: Create response helpers and error mapping**

```go
// internal/adapters/inbound/http/responses.go
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sophia-engine/memory-engine/internal/domain/shared"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shared.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrPurged):
		writeJSON(w, http.StatusGone, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrValidation):
		var ve *shared.ValidationError
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "validation_error", "fields": ve.Fields})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	case errors.Is(err, shared.ErrNotActive):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrDuplicateRelation):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrAlreadyArchived):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, shared.ErrAlreadyExecuted):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
```

- [ ] **Step 2: Create router**

```go
// internal/adapters/inbound/http/server.go
package http

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

func NewRouter(
	memorySvc inbound.MemoryService,
	decisionSvc inbound.DecisionService,
	heuristicSvc inbound.HeuristicService,
	relationSvc inbound.RelationService,
	retrievalSvc inbound.RetrievalService,
	purgeSvc inbound.PurgeService,
	profileSvc inbound.ProjectProfileService,
) chi.Router {
	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Recoverer)
	r.Use(RequestLogger)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/memories", func(r chi.Router) {
			memHandler := NewMemoryHandler(memorySvc)
			r.Post("/", memHandler.Ingest)
			r.Get("/{id}", memHandler.Get)
			r.Post("/{id}/archive", memHandler.Archive)
		})
		r.Route("/decisions", func(r chi.Router) {
			decHandler := NewDecisionHandler(decisionSvc)
			r.Post("/", decHandler.Record)
			r.Get("/{id}", decHandler.Get)
			r.Get("/history/{key}", decHandler.GetHistory)
			r.Post("/{id}/contradict", decHandler.Contradict)
		})
		r.Route("/heuristics", func(r chi.Router) {
			hHandler := NewHeuristicHandler(heuristicSvc)
			r.Post("/", hHandler.Create)
			r.Get("/active/{key}", hHandler.GetActive)
			r.Get("/", hHandler.ListByScope)
			r.Post("/{id}/toggle", hHandler.Toggle)
		})
		r.Route("/relations", func(r chi.Router) {
			relHandler := NewRelationHandler(relationSvc)
			r.Post("/", relHandler.Create)
			r.Get("/from/{id}", relHandler.GetFrom)
			r.Get("/to/{id}", relHandler.GetTo)
		})
		r.Route("/search", func(r chi.Router) {
			retHandler := NewRetrievalHandler(retrievalSvc)
			r.Post("/", retHandler.Search)
			r.Post("/context", retHandler.BuildContext)
		})
		r.Route("/purge", func(r chi.Router) {
			purgeHandler := NewPurgeHandler(purgeSvc)
			r.Post("/request", purgeHandler.Request)
			r.Post("/{id}/execute", purgeHandler.Execute)
		})
		r.Route("/profiles", func(r chi.Router) {
			profHandler := NewProfileHandler(profileSvc)
			r.Post("/generate", profHandler.Generate)
			r.Get("/{projectID}", profHandler.Get)
		})
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return r
}
```

- [ ] **Step 3: Create one handler as reference pattern (memory)**

```go
// internal/adapters/inbound/http/memory_handler.go
package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

type MemoryHandler struct {
	svc inbound.MemoryService
}

func NewMemoryHandler(svc inbound.MemoryService) *MemoryHandler {
	return &MemoryHandler{svc: svc}
}

func (h *MemoryHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type        string   `json:"type"`
		Content     string   `json:"content"`
		Summary     *string  `json:"summary,omitempty"`
		Tags        []string `json:"tags,omitempty"`
		TopicKey    *string  `json:"topic_key,omitempty"`
		FTSLanguage *string  `json:"fts_language,omitempty"`
		Scope       scopeDTO `json:"scope"`
		Provenance  provDTO  `json:"provenance"`
		ValidFrom   *string  `json:"valid_from,omitempty"`
		ValidUntil  *string  `json:"valid_until,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	scope, err := parseScopeDTO(req.Scope)
	if err != nil {
		writeError(w, err)
		return
	}
	prov, err := parseProvDTO(req.Provenance)
	if err != nil {
		writeError(w, err)
		return
	}

	cmd := inbound.IngestMemoryCmd{
		Type:        shared.MemoryType(req.Type),
		Content:     req.Content,
		Summary:     req.Summary,
		Tags:        req.Tags,
		TopicKey:    req.TopicKey,
		FTSLanguage: req.FTSLanguage,
		Scope:       scope,
		Provenance:  prov,
	}
	if req.ValidFrom != nil {
		t, err := time.Parse(time.RFC3339, *req.ValidFrom)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_from format"})
			return
		}
		cmd.ValidFrom = &t
	}
	if req.ValidUntil != nil {
		t, err := time.Parse(time.RFC3339, *req.ValidUntil)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid valid_until format"})
			return
		}
		cmd.ValidUntil = &t
	}

	result, err := h.svc.Ingest(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *MemoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := shared.RecordIDFromString(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	record, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *MemoryHandler) Archive(w http.ResponseWriter, r *http.Request) {
	id, err := shared.RecordIDFromString(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var req struct {
		Reason      string `json:"reason"`
		RequestedBy string `json:"requested_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	err = h.svc.Archive(r.Context(), inbound.ArchiveMemoryCmd{
		ID: id, Reason: req.Reason, RequestedBy: req.RequestedBy,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// DTO parsing helpers — shared across handlers
type scopeDTO struct {
	TenantID    *string `json:"tenant_id,omitempty"`
	ProjectID   string  `json:"project_id"`
	RepoID      *string `json:"repo_id,omitempty"`
	AgentID     *string `json:"agent_id,omitempty"`
	SessionID   *string `json:"session_id,omitempty"`
	Environment *string `json:"environment,omitempty"`
}

type provDTO struct {
	Source    string  `json:"source"`
	SourceURI *string `json:"source_uri,omitempty"`
	Method   string  `json:"method"`
	ParentID *string `json:"parent_id,omitempty"`
}

func parseScopeDTO(dto scopeDTO) (shared.Scope, error) {
	opts := []shared.ScopeOption{}
	if dto.TenantID != nil { opts = append(opts, shared.WithTenantID(*dto.TenantID)) }
	if dto.RepoID != nil { opts = append(opts, shared.WithRepoID(*dto.RepoID)) }
	if dto.AgentID != nil { opts = append(opts, shared.WithAgentID(*dto.AgentID)) }
	if dto.SessionID != nil { opts = append(opts, shared.WithSessionID(*dto.SessionID)) }
	if dto.Environment != nil { opts = append(opts, shared.WithEnvironment(*dto.Environment)) }
	return shared.NewScope(dto.ProjectID, opts...)
}

func parseProvDTO(dto provDTO) (shared.Provenance, error) {
	var parentID *shared.RecordID
	if dto.ParentID != nil {
		id, err := shared.RecordIDFromString(*dto.ParentID)
		if err != nil { return shared.Provenance{}, err }
		parentID = &id
	}
	prov, err := shared.NewProvenance(dto.Source, shared.IngestMethod(dto.Method), parentID)
	if err != nil { return shared.Provenance{}, err }
	if dto.SourceURI != nil {
		prov = prov.WithSourceURI(*dto.SourceURI)
	}
	return prov, nil
}
```

- [ ] **Step 4: Create remaining handlers**

Follow the memory handler pattern for decisions, heuristics, relations, retrieval, purge, and profiles. Each handler parses JSON request → builds command/query → calls service → writes JSON response.

- [ ] **Step 5: Create middleware**

```go
// internal/adapters/inbound/http/middleware.go
package http

import (
	"log/slog"
	"net/http"
	"time"
)

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./...`
Expected: Clean.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/inbound/http/
git commit -m "feat: add HTTP REST adapter with chi router and all handlers"
```

---

## Task 25: Main Entrypoint + Wiring

**Files:**
- Modify: `cmd/memory-engine/main.go`

- [ ] **Step 1: Wire everything together**

```go
// cmd/memory-engine/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sophia-engine/memory-engine/internal/adapters/inbound/http"
	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/embeddings"
	"github.com/sophia-engine/memory-engine/internal/adapters/outbound/persistence"
	"github.com/sophia-engine/memory-engine/internal/application/ingest"
	// ... other application services
	"github.com/sophia-engine/memory-engine/internal/domain/shared"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/config"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/database"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/events"
	"github.com/sophia-engine/memory-engine/internal/infrastructure/logging"
)

func main() {
	logger := logging.NewLogger("info")
	slog.SetDefault(logger)

	cfg := config.DefaultConfig()
	cfg.Database.URL = os.Getenv("DATABASE_URL")
	if cfg.Database.URL == "" {
		slog.Error("DATABASE_URL environment variable is required")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Infrastructure
	clock := shared.RealClock{}
	eventPub := events.NewInProcessEventPublisher()
	txManager := persistence.NewPgTxManager(pool)
	_ = embeddings.NewNoopEmbeddingGenerator() // available but unused in phase 1
	_ = txManager // used by decision and heuristic services

	// Repositories
	memRepo := persistence.NewMemoryPgRepository(pool)
	decRepo := persistence.NewDecisionPgRepository(pool)
	heurRepo := persistence.NewHeuristicPgRepository(pool)
	relRepo := persistence.NewRelationPgRepository(pool)
	purgeRepo := persistence.NewPurgePgRepository(pool)
	profileRepo := persistence.NewProfilePgRepository(pool)
	// searchIdx := persistence.NewPostgresFTSIndex(pool)  // TODO in search adapter task

	// Application Services
	memorySvc := ingest.NewService(memRepo, nil, eventPub, clock)
	_ = decRepo; _ = heurRepo; _ = relRepo; _ = purgeRepo; _ = profileRepo
	// ... wire remaining services

	// HTTP Router
	router := http.NewRouter(memorySvc, nil, nil, nil, nil, nil, nil) // TODO: wire all services

	// Server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Graceful shutdown
	go func() {
		slog.Info("starting server", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./cmd/memory-engine/...`
Expected: Clean (may need to adjust imports as services get wired).

- [ ] **Step 3: Commit**

```bash
git add cmd/memory-engine/main.go
git commit -m "feat: add main entrypoint with dependency wiring and graceful shutdown"
```

---

## Task 26: Test Fixtures

**Files:**
- Create: `test/fixtures/memories.go`
- Create: `test/fixtures/decisions.go`
- Create: `test/fixtures/heuristics.go`
- Create: `test/fixtures/relations.go`
- Create: `test/fixtures/scopes.go`

Create factory functions using functional options pattern for each entity. These are used by both unit and integration tests.

---

## Task 27: Integration Tests — Search + Context

**Files:**
- Create: `test/integration/search_index_test.go`
- Create: `test/integration/usecases/search_test.go`
- Create: `test/integration/usecases/build_context_test.go`

Critical tests:
- FTS spanish stemming ("configuración" finds "configurar")
- Trigram fallback ("middleware" found even though not Spanish)
- `FTSLanguage` default spanish vs override simple
- Ranking composition verifiable
- BuildContext always includes active decisions regardless of query
- Token budget respected

---

## Task 28: Integration Tests — Decision + Heuristic Transactional

**Files:**
- Create: `test/integration/usecases/record_decision_test.go`
- Create: `test/integration/usecases/ingest_memory_test.go`

Critical tests:
- RecordDecision supersedes previous in same transaction
- CreateHeuristic disables previous in same transaction
- Rollback on mid-transaction failure

---

## Summary — Implementation Order

| Order | Task | What | Dependencies |
|---|---|---|---|
| 1 | Bootstrap | go.mod, Makefile, config, logging | None |
| 2 | Enums + Errors | Domain constants and error types | 1 |
| 3 | RecordID | ULID value object | 2 |
| 4 | Scope, Provenance, TimeRange | Core VOs | 3 |
| 5 | Temporal, Confidence, Importance, Evidence | Remaining VOs | 3, 4 |
| 6 | MemoryRecord | First entity | 2-5 |
| 7 | Decision | With status transitions | 2-5 |
| 8 | Heuristic, Relation, Purge, Profile | Remaining entities | 2-5 |
| 9 | Outbound Ports | Repository + infra interfaces | 6-8 |
| 10 | Inbound Ports | Service interfaces + DTOs | 6-8, 9 |
| 11 | Migrations | PostgreSQL schema | Spec |
| 12 | Database + Infra | Pool, test helpers, events, noop embed | 9 |
| 13 | Memory PG Repo | First adapter + integration tests | 11, 12 |
| 14-17 | Remaining PG Repos | All adapters | 13 |
| 18 | Ingest Service | First app service | 9, 10, 13 |
| 19-23 | Remaining App Services | Decision, Heuristic, Relation, Search, Context | 14-17 |
| 24 | HTTP Adapter | REST handlers | 18-23 |
| 25 | Main Entrypoint | Wiring | 24 |
| 26 | Test Fixtures | Factory functions | 6-8 |
| 27 | Search + Context Integration | FTS, ranking, budget tests | 22, 23 |
| 28 | Transactional Integration | Supersede/disable tests | 19, 20 |
