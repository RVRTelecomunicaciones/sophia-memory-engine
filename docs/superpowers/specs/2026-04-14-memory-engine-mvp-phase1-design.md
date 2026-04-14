# Memory Engine — MVP Phase 1 Technical Design

**Date**: 2026-04-14
**Status**: Approved
**Scope**: Phase 1 MVP — domain model, persistence, retrieval, contracts, testing

---

## 1. Architecture Decisions

### 1.1 Consumer Model

- **Phase 1**: Multiple heterogeneous agents within one platform (coding, review, QA, planning, docs)
- **Phase 2+**: Shared multi-project, multi-team service
- `tenant_id` exists as optional field in domain and schema, not operationally active in phase 1

### 1.2 Domain Strategy

Shared-kernel domain with differentiated aggregates:

- `internal/domain/shared` — common value objects only (RecordID, Scope, Provenance, TemporalMetadata, Confidence, ImportanceScore, EvidenceRef, TimeRange)
- Each aggregate has its own directory, entities, rules, invariants
- Shared kernel must NOT contain aggregate-specific business logic
- Unified read model for retrieval lives in application layer, not domain

### 1.3 Retrieval Strategy

Phase 1 retrieval = FTS + trigram + scope + temporal + graph expansion. No vector similarity.

- PostgreSQL FTS with `spanish` default language, per-record override via `fts_language` column
- `pg_trgm` as fallback for technical English terms, typos, partial matches
- Combined ranking: FTS stemmed + trigram fuzzy
- Embedding port exists as interface with noop stub, no use case depends on it
- Phase 1 "hybrid" = multiple non-vector signals combined, NOT FTS + vector

### 1.4 Graph Model

Adjacency list in PostgreSQL with recursive CTEs:

- Table `relations` with explicit `relation_type`
- Traversal bounded by `max_depth` (hard cap = 3)
- Cycle prevention via path tracking in CTE
- Scope and temporal filters applied during traversal
- Phase 2 may introduce graph DB adapter if volume demands

### 1.5 FTS Language

- Default: `spanish` — majority of content and queries in Latin American Spanish
- Override per record via `fts_language` column
- Phase 1 query parser assumes `plainto_tsquery('spanish', ...)` as default
- This is a pragmatic phase 1 decision, not a complete multilanguage solution

---

## 2. Domain Entities

### 2.1 Aggregate: memory

```
MemoryRecord
├── ID          : RecordID (ULID)
├── Type        : MemoryType (episodic | semantic)
├── Content     : string (required, non-empty; empty string = purged tombstone)
├── Summary     : *string
├── Tags        : []string
├── TopicKey    : *string (optional, for logical grouping and consolidation)
├── Scope       : Scope
├── Provenance  : Provenance
├── Temporal    : TemporalMetadata
├── Importance  : ImportanceScore
├── Status      : MemoryStatus (active | archived | purged)
├── ArchivedBy  : *string
├── ArchiveReason: *string
├── FTSLanguage : string (default "spanish")
├── CreatedAt   : time.Time
└── UpdatedAt   : time.Time
```

Type differentiation:

- **episodic**: what happened, when, in what context. MUST have `ValidFrom`. Importance decays with time.
- **semantic**: derived knowledge, pattern, convention. `ValidFrom` optional, `ValidUntil` often nil. Importance more stable.

### 2.2 Aggregate: decision

```
Decision
├── ID           : RecordID
├── DecisionKey  : string (logical grouping key, e.g. "auth/token-strategy")
├── Version      : int (auto-incremented within DecisionKey + Scope)
├── Title        : string
├── Description  : string
├── Rationale    : string
├── Evidence     : []EvidenceRef (at least one)
├── Scope        : Scope
├── Provenance   : Provenance
├── Temporal     : TemporalMetadata
├── Confidence   : Confidence
├── Status       : DecisionStatus (active | superseded | contradicted | archived)
├── SupersededBy : *RecordID
├── FTSLanguage  : string (default "spanish")
├── CreatedAt    : time.Time
└── UpdatedAt    : time.Time
```

Status transitions (mutually exclusive):

```
active → superseded    (new version with same DecisionKey takes over)
active → contradicted  (evidence registered via contradicts relation)
active → archived      (no longer applies, preserved for history)
superseded → archived
contradicted → archived
```

No state transitions TO active (create new version instead) or TO deleted (decisions never disappear).

`DecisionKey` groups decisions on the same topic over time. Version auto-increments within `DecisionKey + Scope`.

### 2.3 Aggregate: heuristic

```
HeuristicRule
├── ID            : RecordID
├── HeuristicKey  : string (logical key, e.g. "testing/always-run-integration")
├── Version       : int
├── Condition     : string
├── Action        : string
├── Rationale     : string
├── Scope         : Scope
├── Provenance    : Provenance
├── Confidence    : Confidence
├── Enabled       : bool
├── Temporal      : TemporalMetadata (includes ValidUntil for expiry)
├── FTSLanguage   : string (default "spanish")
├── CreatedAt     : time.Time
└── UpdatedAt     : time.Time
```

Active version rule: `WHERE key=? AND scope matches AND enabled=true AND (valid_until IS NULL OR valid_until > now()) ORDER BY version DESC LIMIT 1`

Only ONE version can be `Enabled=true` per `HeuristicKey + Scope`. Creating a new enabled version disables the previous one in the same transaction.

### 2.4 Aggregate: relation

```
Relation
├── ID          : RecordID
├── SourceID    : RecordID
├── TargetID    : RecordID
├── Type        : RelationType
├── Metadata    : map[string]any (auxiliary context only, not business rules)
├── Scope       : Scope
├── Temporal    : TemporalMetadata (ValidFrom/ValidUntil for temporal filtering)
├── CreatedAt   : time.Time
└── UpdatedAt   : time.Time
```

RelationType: `relates_to`, `depends_on`, `supersedes`, `contradicts`, `references`, `derived_from`, `resolves`, `extends`

Metadata is auxiliary context (reason strings, URLs, step numbers). If a metadata field is used programmatically for business decisions, it must be promoted to an explicit entity field.

### 2.5 Aggregate: purge

```
PurgeRecord
├── ID              : RecordID
├── TargetID        : RecordID
├── TargetType      : string (memory | decision | heuristic | relation)
├── Reason          : PurgeReason (secret_leak | compliance | sensitive_content)
├── RequestedBy     : string
├── Scope           : Scope
├── Status          : PurgeStatus (pending | executing | executed | failed)
├── AuditNote       : string
├── ArtifactsPurged : PurgedArtifacts
├── ExecutedAt      : *time.Time
├── CreatedAt       : time.Time
└── UpdatedAt       : time.Time
```

```
PurgedArtifacts
├── FTSInvalidated       : bool
├── EmbeddingInvalidated : bool
├── CacheInvalidated     : bool
├── RelationsRemoved     : int
├── DerivedInvalidated   : []RecordID
```

Purge flow: target content wiped to empty string (tombstone), FTS cleared, embedding nulled, relations removed, derived records marked invalidated. PurgeRecord is the authoritative audit trail.

### 2.6 Aggregate: projectprofile

```
ProjectProfile
├── ID               : RecordID
├── ProjectID        : string
├── Version          : int
├── Summary          : string
├── ActiveDecisions  : []RecordID
├── TopHeuristics    : []RecordID
├── Patterns         : []PatternEntry
├── ArchSignals      : []string
├── Freshness        : FreshnessState
├── Scope            : Scope
├── CreatedAt        : time.Time
└── UpdatedAt        : time.Time
```

```
FreshnessState
├── GeneratedAt     : time.Time
├── SourceTimeRange : TimeRange
├── StaleAfter      : time.Time
├── SourceCounts    : SourceCounts
```

Derived knowledge only — regenerated, never manually edited. Phase 1: invocable manually via HTTP/SDK. Phase 2: worker-driven with StaleAfter as trigger.

---

## 3. Value Objects (`internal/domain/shared`)

All immutable, compared by value, no identity.

### RecordID

ULID (26 chars, Crockford base32). Time-sortable, no PG extension required.

### Scope

```
Scope
├── TenantID    : *string (nil in phase 1)
├── ProjectID   : string  (ALWAYS required)
├── RepoID      : *string
├── AgentID     : *string
├── SessionID   : *string
├── Environment : *string
```

Scope filter matching: present fields → exact match, nil fields → no filter at that level.

### Provenance

```
Provenance
├── Source    : string (required, format "type:identifier")
├── SourceURI: *string
├── Method   : IngestMethod (direct | derived | imported | worker_generated)
├── ParentID : *RecordID (required when Method = derived)
```

### TemporalMetadata

```
TemporalMetadata
├── ValidFrom   : *time.Time
├── ValidUntil  : *time.Time
├── LastAccessed: *time.Time
├── Freshness   : FreshnessLevel (fresh | aging | stale | expired)
```

Freshness is cached but computable via `ComputeFreshness(config, clock)`. Thresholds are configurable per scope/type, not hardcoded.

Persistence mapping: `valid_from`, `valid_until`, `last_accessed` → `timestamptz`, `freshness` → `varchar`.

### Confidence

```
Confidence
├── Score  : float64 (0.0 to 1.0, clamped)
├── Source : ConfidenceSource (human | agent | derived | computed)
```

### ImportanceScore

```
ImportanceScore
├── Score      : float64 (0.0 to 1.0)
├── ComputedAt : time.Time
├── Factors    : []ImportanceFactor{Name, Weight, Value}
```

Always explainable — Factors decompose how the score was computed.

### EvidenceRef

```
EvidenceRef
├── RecordID : *RecordID
├── URI      : *string
├── Excerpt  : *string
├── Type     : EvidenceType (memory_ref | external_link | inline_excerpt)
```

At least one of RecordID/URI must be present. Type determines which field is required.

### TimeRange

```
TimeRange { From, To time.Time }
```

Validation: From <= To.

---

## 4. Use Cases — MVP Priority

### P0 — Core (6)

| ID | Name | Aggregate | Key behavior |
|---|---|---|---|
| M1 | IngestMemory | memory | Persist + FTS index + initial importance + event |
| M2 | GetMemory | memory | ErrNotFound vs ErrPurged distinction |
| RT1 | Search | retrieval | FTS + trigram + scope + temporal + explainable ranking |
| D1 | RecordDecision | decision | Transactional supersede of previous + relation creation |
| H1 | CreateHeuristic | heuristic | Transactional disable of previous enabled version |
| R1 | CreateRelation | relation | Validates source/target exist and not purged |

### P1-alto — Essential (4)

| ID | Name | Aggregate | Key behavior |
|---|---|---|---|
| RT2 | BuildContext | retrieval | Token budget, sections, decisions/heuristics always included |
| R2 | GetRelationsFrom | relation | Bounded recursive CTE traversal |
| D2 | GetDecision | decision | Direct lookup by ID |
| H2 | GetActiveHeuristic | heuristic | Active version by key + scope |

### P1 — Important (3)

| ID | Name | Aggregate | Key behavior |
|---|---|---|---|
| D3 | GetDecisionHistory | decision | All versions by DecisionKey, scope-aware |
| H3 | ListHeuristicsByScope | heuristic | All heuristics filtered by scope and enabled |
| M3 | ArchiveMemory | memory | active → archived with RequestedBy |

### P2 — Scaffolded, implemented at end of phase 1 or early phase 2 (4)

| ID | Name | Aggregate | Key behavior |
|---|---|---|---|
| P1 | RequestPurge | purge | Creates pending purge record |
| P2 | ExecutePurge | purge | Atomic wipe + invalidation |
| H4 | ToggleHeuristic | heuristic | Enable/disable with conflict check |
| D4 | ContradictDecision | decision | Marks contradicted + creates relation |

### P3 — Manual only (1)

| ID | Name | Aggregate | Key behavior |
|---|---|---|---|
| PP1 | GenerateProjectProfile | projectprofile | Manual invocation only, not worker-driven |

---

## 5. Ports

### 5.1 Inbound Ports (`internal/ports/inbound`)

All mutative operations use command objects. All composite returns use result objects.

```
MemoryService
  Ingest(ctx, IngestMemoryCmd) → (*IngestMemoryResult, error)
  Get(ctx, RecordID) → (*MemoryRecord, error)
  Archive(ctx, ArchiveMemoryCmd) → error

DecisionService
  Record(ctx, RecordDecisionCmd) → (*RecordDecisionResult, error)
  Get(ctx, RecordID) → (*Decision, error)
  GetHistory(ctx, DecisionHistoryQuery) → ([]Decision, error)
  Contradict(ctx, ContradictDecisionCmd) → error

HeuristicService
  Create(ctx, CreateHeuristicCmd) → (*CreateHeuristicResult, error)
  GetActive(ctx, GetActiveHeuristicQuery) → (*HeuristicRule, error)
  ListByScope(ctx, ListHeuristicsQuery) → ([]HeuristicRule, error)
  Toggle(ctx, ToggleHeuristicCmd) → error

RelationService
  Create(ctx, CreateRelationCmd) → (*CreateRelationResult, error)
  GetFrom(ctx, RelationQuery) → ([]RelationResult, error)
  GetTo(ctx, RelationQuery) → ([]RelationResult, error)

RetrievalService
  Search(ctx, SearchQuery) → (*SearchResults, error)
  BuildContext(ctx, ContextRequest) → (*ContextBundle, error)

PurgeService
  Request(ctx, RequestPurgeCmd) → (*PurgeRecord, error)
  Execute(ctx, ExecutePurgeCmd) → (*PurgeRecord, error)

ProjectProfileService
  Generate(ctx, GenerateProfileCmd) → (*ProjectProfile, error)
  Get(ctx, GetProfileQuery) → (*ProjectProfile, error)
```

Get-by-ID semantics: non-existent → `ErrNotFound`, purged → `ErrPurged`, archived → returned normally.

String transport fields (type, method, reason, status) validate against domain constants in `internal/domain/shared/enums.go`.

`ErrValidation` wraps `[]FieldError{Field, Message, Value}` for structured error reporting.

### 5.2 Outbound Ports (`internal/ports/outbound`)

```
MemoryRepository       — Save, FindByID, UpdateStatus, WipeContent
DecisionRepository     — Save, FindByID, FindActiveByKey, FindByKey, UpdateStatus
HeuristicRepository    — Save, FindByID, FindActiveByKey, FindByScope, UpdateEnabled
RelationRepository     — Save, FindFromSource, FindToTarget, Traverse, DeleteByTarget
PurgeRepository        — Save, FindByID, UpdateStatus
ProjectProfileRepository — Save, FindLatest
SearchIndex            — Index (upsert), Remove, Search (read-model, not authoritative)
EmbeddingGenerator     — Generate, BatchGenerate (stub in phase 1, no use case depends on it)
TransactionManager     — WithTx (ctx carries tx, repos detect and use it)
Clock                  — Now() (RealClock prod, FixedClock tests)
EventPublisher         — Publish(DomainEvent) (InProcess sync bus, NOT transactional)
```

SearchIndex is a read-model/indexing port. Not authoritative storage. Rebuildable from repositories.

EventPublisher phase 1: in-process synchronous bus. Events published AFTER successful persistence. At-most-once logging to domain_events table. NOT exactly-once delivery.

### 5.3 Port-to-Adapter Mapping

| Port (outbound) | Phase 1 adapter | Phase 2+ |
|---|---|---|
| *Repository | PostgreSQL | PostgreSQL |
| SearchIndex | PostgreSQL FTS (trigger-maintained) | Elasticsearch/Meilisearch |
| EmbeddingGenerator | NoopEmbeddingGenerator | OpenAI/Cohere/local |
| TransactionManager | pgx transaction | pgx transaction |
| Clock | RealClock | RealClock |
| EventPublisher | InProcessEventPublisher | NATS/Kafka/PG NOTIFY |

| Port (inbound) | Phase 1 adapter | Phase 2+ |
|---|---|---|
| All services | HTTP REST | HTTP REST + gRPC |
| All services | Go SDK | Go SDK |
| Read-only subset | — | MCP read adapter |

---

## 6. PostgreSQL Schema

### Extensions

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
-- Phase 2: CREATE EXTENSION IF NOT EXISTS vector;
```

### Table: memories

```sql
CREATE TABLE memories (
    id              VARCHAR(26) PRIMARY KEY,
    type            VARCHAR(20)  NOT NULL CHECK (type IN ('episodic', 'semantic')),
    content         TEXT         NOT NULL,  -- empty string = purged tombstone
    summary         TEXT,
    tags            TEXT[]       DEFAULT '{}',
    topic_key       VARCHAR(255),
    fts_language    REGCONFIG    NOT NULL DEFAULT 'spanish',
    tenant_id       VARCHAR(100),
    project_id      VARCHAR(100) NOT NULL,
    repo_id         VARCHAR(100),
    agent_id        VARCHAR(100),
    session_id      VARCHAR(100),
    environment     VARCHAR(50),
    source          VARCHAR(255) NOT NULL,
    source_uri      TEXT,
    ingest_method   VARCHAR(20)  NOT NULL CHECK (ingest_method IN ('direct','derived','imported','worker_generated')),
    parent_id       VARCHAR(26)  REFERENCES memories(id),
    valid_from      TIMESTAMPTZ,
    valid_until     TIMESTAMPTZ,
    last_accessed   TIMESTAMPTZ,
    freshness       VARCHAR(10)  DEFAULT 'fresh' CHECK (freshness IN ('fresh','aging','stale','expired')),
    importance_score       NUMERIC(4,3) DEFAULT 0.500,
    importance_computed_at TIMESTAMPTZ,
    importance_factors     JSONB,
    status          VARCHAR(20)  NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived','purged')),
    archived_by     VARCHAR(255),
    archive_reason  TEXT,
    search_vector   TSVECTOR,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_episodic_valid_from CHECK (type != 'episodic' OR valid_from IS NOT NULL)
);
```

Key indexes: `project_id`, `project_id+type`, `topic_key`, scope composite, `status`, `freshness` (partial), `created_at DESC`, FTS GIN, trigram GIN, `tenant_id` (partial).

FTS trigger uses `NEW.fts_language` per record with weighted fields: summary (A), tags (B), content (C).

### Table: decisions

```sql
CREATE TABLE decisions (
    id              VARCHAR(26) PRIMARY KEY,
    decision_key    VARCHAR(255) NOT NULL,
    version         INT          NOT NULL DEFAULT 1,
    title           TEXT         NOT NULL,
    description     TEXT         NOT NULL,
    rationale       TEXT         NOT NULL,
    evidence        JSONB        NOT NULL DEFAULT '[]',
    fts_language    REGCONFIG    NOT NULL DEFAULT 'spanish',
    tenant_id       VARCHAR(100),
    project_id      VARCHAR(100) NOT NULL,
    repo_id         VARCHAR(100),
    agent_id        VARCHAR(100),
    session_id      VARCHAR(100),
    environment     VARCHAR(50),
    source          VARCHAR(255) NOT NULL,
    source_uri      TEXT,
    ingest_method   VARCHAR(20)  NOT NULL,
    valid_from      TIMESTAMPTZ,
    valid_until     TIMESTAMPTZ,
    last_accessed   TIMESTAMPTZ,
    freshness       VARCHAR(10)  DEFAULT 'fresh',
    confidence_score  NUMERIC(4,3) NOT NULL,
    confidence_source VARCHAR(20)  NOT NULL CHECK (confidence_source IN ('human','agent','derived','computed')),
    status          VARCHAR(20)  NOT NULL DEFAULT 'active' CHECK (status IN ('active','superseded','contradicted','archived')),
    superseded_by   VARCHAR(26)  REFERENCES decisions(id),
    search_vector   TSVECTOR,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_superseded_has_ref CHECK (status != 'superseded' OR superseded_by IS NOT NULL)
);
```

Unique index on `(decision_key, project_id, COALESCE(repo_id,''), COALESCE(agent_id,''), COALESCE(environment,''), version)` — version scoped by key + full scope.

### Table: heuristics

```sql
CREATE TABLE heuristics (
    id              VARCHAR(26) PRIMARY KEY,
    heuristic_key   VARCHAR(255) NOT NULL,
    version         INT          NOT NULL DEFAULT 1,
    condition       TEXT         NOT NULL,
    action          TEXT         NOT NULL,
    rationale       TEXT         NOT NULL,
    fts_language    REGCONFIG    NOT NULL DEFAULT 'spanish',
    tenant_id       VARCHAR(100),
    project_id      VARCHAR(100) NOT NULL,
    repo_id         VARCHAR(100),
    agent_id        VARCHAR(100),
    session_id      VARCHAR(100),
    environment     VARCHAR(50),
    source          VARCHAR(255) NOT NULL,
    source_uri      TEXT,
    ingest_method   VARCHAR(20)  NOT NULL,
    valid_from      TIMESTAMPTZ,
    valid_until     TIMESTAMPTZ,
    last_accessed   TIMESTAMPTZ,
    freshness       VARCHAR(10)  DEFAULT 'fresh',
    confidence_score  NUMERIC(4,3) NOT NULL,
    confidence_source VARCHAR(20)  NOT NULL,
    enabled         BOOLEAN      NOT NULL DEFAULT true,
    search_vector   TSVECTOR,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
```

Unique index on version scope (same pattern as decisions). Partial unique index on `(heuristic_key, project_id, COALESCE(repo_id,''), COALESCE(agent_id,'')) WHERE enabled = true` — no `NOW()`, temporal expiry enforced at application level.

### Table: relations

```sql
CREATE TABLE relations (
    id              VARCHAR(26) PRIMARY KEY,
    source_id       VARCHAR(26) NOT NULL,
    target_id       VARCHAR(26) NOT NULL,
    relation_type   VARCHAR(30) NOT NULL CHECK (relation_type IN (
        'relates_to','depends_on','supersedes','contradicts',
        'references','derived_from','resolves','extends'
    )),
    metadata        JSONB        DEFAULT '{}',
    tenant_id       VARCHAR(100),
    project_id      VARCHAR(100) NOT NULL,
    repo_id         VARCHAR(100),
    agent_id        VARCHAR(100),
    session_id      VARCHAR(100),
    environment     VARCHAR(50),
    valid_from      TIMESTAMPTZ,
    valid_until     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_no_self_ref CHECK (source_id != target_id),
    UNIQUE(source_id, target_id, relation_type)
);
```

No foreign keys on source_id/target_id — cross-aggregate references. Referential integrity enforced at application layer. Documented trade-off.

Recursive CTE template for traversal with: max_depth bound, cycle prevention via path array, scope filter, temporal filter, exclusion of purged targets.

### Table: purge_records

```sql
CREATE TABLE purge_records (
    id              VARCHAR(26) PRIMARY KEY,
    target_id       VARCHAR(26) NOT NULL,
    target_type     VARCHAR(20) NOT NULL CHECK (target_type IN ('memory','decision','heuristic','relation')),
    reason          VARCHAR(30) NOT NULL CHECK (reason IN ('secret_leak','compliance','sensitive_content')),
    requested_by    VARCHAR(255) NOT NULL,
    audit_note      TEXT         NOT NULL,
    tenant_id       VARCHAR(100),
    project_id      VARCHAR(100) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','executing','executed','failed')),
    artifacts_purged JSONB,
    executed_at     TIMESTAMPTZ,
    error_detail    TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
```

### Table: project_profiles

```sql
CREATE TABLE project_profiles (
    id              VARCHAR(26) PRIMARY KEY,
    project_id      VARCHAR(100) NOT NULL,
    version         INT          NOT NULL DEFAULT 1,
    summary         TEXT         NOT NULL,
    active_decisions  VARCHAR(26)[] DEFAULT '{}',
    top_heuristics    VARCHAR(26)[] DEFAULT '{}',
    patterns        JSONB        NOT NULL DEFAULT '[]',
    arch_signals    TEXT[]        DEFAULT '{}',
    generated_at         TIMESTAMPTZ NOT NULL,
    source_time_from     TIMESTAMPTZ NOT NULL,
    source_time_to       TIMESTAMPTZ NOT NULL,
    stale_after          TIMESTAMPTZ NOT NULL,
    source_counts        JSONB       NOT NULL,
    tenant_id       VARCHAR(100),
    repo_id         VARCHAR(100),
    agent_id        VARCHAR(100),
    environment     VARCHAR(50),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, version)
);
```

### Table: domain_events

```sql
CREATE TABLE domain_events (
    id              BIGSERIAL PRIMARY KEY,
    event_id        VARCHAR(26) NOT NULL UNIQUE,
    event_type      VARCHAR(50)  NOT NULL,
    aggregate_id    VARCHAR(26)  NOT NULL,
    aggregate_type  VARCHAR(30)  NOT NULL,
    project_id      VARCHAR(100) NOT NULL,
    payload         JSONB        NOT NULL,
    occurred_at     TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
```

Append-only event log for auditability. Not a transactional outbox.

### Shared infrastructure

- `set_updated_at()` trigger function applied to all mutable tables
- FTS trigger functions per table using `NEW.fts_language` with weighted fields

---

## 7. Request/Response Contracts

All mutative operations use command objects (Cmd suffix). All composite returns use result objects (Result suffix). String transport fields validate against domain constants. `ErrValidation` wraps `[]FieldError{Field, Message, Value}`.

Get-by-ID semantics across all services: non-existent → `ErrNotFound`, purged → `ErrPurged`, archived → returned normally.

### Domain Error Catalog

```
ErrNotFound            — record does not exist
ErrPurged              — record exists but was purged
ErrValidation          — input validation failed (structured field errors)
ErrAlreadyArchived     — already archived
ErrNotActive           — operation requires active status
ErrConflict            — business rule conflict
ErrDuplicateRelation   — relation already exists
ErrSourceNotFound      — relation source does not exist
ErrTargetNotFound      — relation target does not exist
ErrSourcePurged        — relation source was purged
ErrTargetPurged        — relation target was purged
ErrParentNotFound      — parent for derived provenance not found
ErrEvidenceRefNotFound — evidence reference not found
ErrAlreadyExecuted     — purge already executed
```

Full contract details for each use case (command objects, query objects, response objects, validation rules, error cases) were validated in the design conversation and are authoritative as described in sections M1-M3, D1-D4, H1-H4, R1-R3, RT1-RT2, P1-P2, PP1.

---

## 8. Retrieval Strategy — Phase 1

### Signals

| Signal | Source | Weight |
|---|---|---|
| FTS relevance | `ts_rank(search_vector, query)` | 0.25 |
| Trigram similarity | `similarity(content, query)` | 0.15 |
| Recency | `1 / (1 + days_since(created_at))` | 0.12 |
| Importance | `importance_score` (pre-computed) | 0.13 |
| Type boost | Implicit by record type (decision=0.8, heuristic=0.7, memory=importance_score) | 0.10 |
| Freshness | fresh=1.0, aging=0.7, stale=0.3, expired=0.1 | 0.10 |
| Scope exactness | Computed in application layer | 0.15 |

Recency uses `created_at` in phase 1 for simplicity. Phase 2 may evolve to `COALESCE(valid_from, created_at)`.

Importance and type boost are distinct, explicitly named signals.

Scope exactness computed in application layer (Go), not SQL.

### Search pipeline

1. QUALIFY — scope + status + temporal filters (WHERE)
2. MATCH — FTS tsquery + trigram similarity (scoring)
3. RANK — combine all signals with configurable weights (ORDER BY)
4. LIMIT — pagination
5. EXPLAIN — attach RankingResponse per result

Phase 1 uses UNION ALL across memories, decisions, heuristics in a single query. Ranking weights configurable via `RetrievalConfig`, not hardcoded.

### BuildContext strategy

1. ALLOCATE BUDGET — divide token budget by section (decisions 25%, heuristics 20%, episodic 30%, semantic 15%, related 10%)
2. FETCH — decisions and heuristics fetched by scope (always included regardless of query), memories fetched by FTS query
3. COMPACT — use Summary when available, truncate to fit budget, higher score = higher priority
4. EXPAND GRAPH — optional 1-level relations for top 5 records
5. ASSEMBLE — structured ContextBundle with sections and token counts

Token estimation: `len(content) / 4` (approx). Unused section budget redistributed proportionally.

`ContextDebugInfo` optional field for strategy, records scanned/included, duration.

---

## 9. Testing Strategy

### Levels

| Level | What | Where | Infra |
|---|---|---|---|
| Unit | Domain invariants, VOs, entities | `internal/domain/*/..._test.go` | None |
| Application | Use case orchestration, port contracts | `internal/application/*/..._test.go` | Mocked ports |
| Integration | PG adapters, FTS, recursive CTEs | `test/integration/` | testcontainers-go (real PG) |
| Use-case integration | Full use case with real PG | `test/integration/usecases/` | testcontainers-go |

### Key testing rules

- All temporal tests use `FixedClock` — never `time.Now()` directly
- Every invariant in `docs/domain-invariants.md` must have at least one test
- Transactional consistency of supersede/disable verified (rollback on mid-tx failure)
- FTS spanish stemming + trigram fallback verified with integration tests
- Graph traversal depth bound + cycle prevention verified
- Test factories via functional options pattern in `test/fixtures/`

### Critical test cases

- `ErrNotFound` vs `ErrPurged` distinction for all Get operations
- `FTSLanguage` default spanish vs override simple
- BuildContext always includes active decisions and enabled heuristics regardless of query score
- `RankingResponse.FinalScore` matches recomputed composition of individual signals

### Tools

- `testing` stdlib, `testify/assert+require`, `testcontainers-go`, `moq` or `mockgen`, `golangci-lint`
- No benchmarks, fuzzing, or load testing in phase 1

---

## 10. Phase 1 Exclusions

Conscious exclusions with rationale and activation signals.

| Excluded | Rationale | Phase |
|---|---|---|
| Embeddings + vector search | FTS+trigram sufficient for initial volume | 2 |
| MCP read adapter | HTTP+SDK first, MCP is mechanical mapping | 2 |
| Background workers (freshness, importance, consolidation, contradiction) | System works without; compute on-read or manual | 2 |
| Multi-tenant operational enforcement | tenant_id structural placeholder | 2 |
| gRPC adapter | HTTP+SDK covers MVP | 2-3 |
| Transactional outbox | In-process events sufficient | 2 |
| OpenTelemetry + Prometheus | Structured logging (slog) is minimum viable | 2 |
| TypeScript/Python SDKs | Go SDK + HTTP first | 2-3 |
| SQLite adapter | PostgreSQL is production target | 2 |
| Advanced retrieval (personalized ranking, re-ranking, semantic expansion) | Phase 1 signals sufficient | 2-3 |

### Phase 1 implementation scope

| Priority | Use cases | Count | Status |
|---|---|---|---|
| P0 | IngestMemory, GetMemory, Search, RecordDecision, CreateHeuristic, CreateRelation | 6 | Fully implemented |
| P1-alto | BuildContext, GetRelationsFrom, GetDecision, GetActiveHeuristic | 4 | Fully implemented |
| P1 | GetDecisionHistory, ListHeuristicsByScope, ArchiveMemory | 3 | Fully implemented |
| P2 | RequestPurge, ExecutePurge, ToggleHeuristic, ContradictDecision | 4 | Scaffolded, complete at end of phase 1 or early phase 2 |
| P3 | GenerateProjectProfile | 1 | Manual invocation only |

### Phase 1 includes

- 6 domain aggregates with entities and value objects
- Shared kernel in `internal/domain/shared`
- Application services for P0 through P1 use cases (13 fully implemented)
- All inbound and outbound port interfaces
- PostgreSQL schema with FTS spanish + trigram
- PostgreSQL adapter implementations
- HTTP REST inbound adapter
- Go SDK inbound adapter
- Retrieval: FTS + trigram + scope + temporal + graph + explainable ranking
- Context builder with token budget and sections
- Clock port for deterministic testing
- EventPublisher in-process + domain_events table
- EmbeddingGenerator stub interface (no use case depends on it)
- Structured logging with slog
- Unit + application + integration tests with testcontainers
- PostgreSQL migrations
- `cmd/memory-engine/main.go` HTTP server
