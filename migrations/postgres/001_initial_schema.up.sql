-- Memory Engine — Initial Schema
-- Phase 1 MVP: domain tables, FTS, trigram, scope indexes, temporal support

-- =============================================================================
-- Extensions
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- =============================================================================
-- Trigger function: auto-update updated_at
-- =============================================================================

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- Table: memories
-- =============================================================================

CREATE TABLE memories (
    id              VARCHAR(26) PRIMARY KEY,
    type            VARCHAR(20)  NOT NULL CHECK (type IN ('episodic', 'semantic')),
    content         TEXT         NOT NULL,
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

-- Indexes: memories
CREATE INDEX idx_memories_project_id ON memories (project_id);
CREATE INDEX idx_memories_project_type ON memories (project_id, type);
CREATE INDEX idx_memories_topic_key ON memories (topic_key) WHERE topic_key IS NOT NULL;
CREATE INDEX idx_memories_scope ON memories (project_id, repo_id, agent_id, session_id, environment);
CREATE INDEX idx_memories_status ON memories (status) WHERE status != 'active';
CREATE INDEX idx_memories_freshness ON memories (freshness) WHERE freshness != 'fresh';
CREATE INDEX idx_memories_created_at ON memories (created_at DESC);
CREATE INDEX idx_memories_tenant_id ON memories (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_memories_fts ON memories USING GIN (search_vector);
CREATE INDEX idx_memories_trgm ON memories USING GIN (content gin_trgm_ops);

-- Trigger: updated_at
CREATE TRIGGER trg_memories_updated_at
    BEFORE UPDATE ON memories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- FTS trigger function: memories
CREATE OR REPLACE FUNCTION memories_fts_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.summary, '')), 'A') ||
        setweight(to_tsvector(NEW.fts_language, COALESCE(array_to_string(NEW.tags, ' '), '')), 'B') ||
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.content, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_memories_fts
    BEFORE INSERT OR UPDATE ON memories
    FOR EACH ROW EXECUTE FUNCTION memories_fts_update();

-- =============================================================================
-- Table: decisions
-- =============================================================================

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

-- Version scope unique index
CREATE UNIQUE INDEX idx_decisions_version_scope
    ON decisions (decision_key, project_id, COALESCE(repo_id, ''), COALESCE(agent_id, ''), COALESCE(environment, ''), version);

-- Indexes: decisions
CREATE INDEX idx_decisions_project_id ON decisions (project_id);
CREATE INDEX idx_decisions_scope ON decisions (project_id, repo_id, agent_id, session_id, environment);
CREATE INDEX idx_decisions_status ON decisions (status) WHERE status != 'active';
CREATE INDEX idx_decisions_tenant_id ON decisions (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_decisions_fts ON decisions USING GIN (search_vector);
CREATE INDEX idx_decisions_decision_key ON decisions (decision_key);

-- Trigger: updated_at
CREATE TRIGGER trg_decisions_updated_at
    BEFORE UPDATE ON decisions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- FTS trigger function: decisions
CREATE OR REPLACE FUNCTION decisions_fts_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.rationale, '')), 'B') ||
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.description, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_decisions_fts
    BEFORE INSERT OR UPDATE ON decisions
    FOR EACH ROW EXECUTE FUNCTION decisions_fts_update();

-- =============================================================================
-- Table: heuristics
-- =============================================================================

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

-- Version scope unique index
CREATE UNIQUE INDEX idx_heuristics_version_scope
    ON heuristics (heuristic_key, project_id, COALESCE(repo_id, ''), COALESCE(agent_id, ''), COALESCE(environment, ''), version);

-- Partial unique index: only one enabled version per key + scope
CREATE UNIQUE INDEX idx_heuristics_active
    ON heuristics (heuristic_key, project_id, COALESCE(repo_id, ''), COALESCE(agent_id, ''))
    WHERE enabled = true;

-- Indexes: heuristics
CREATE INDEX idx_heuristics_project_id ON heuristics (project_id);
CREATE INDEX idx_heuristics_scope ON heuristics (project_id, repo_id, agent_id, session_id, environment);
CREATE INDEX idx_heuristics_enabled ON heuristics (enabled) WHERE enabled = true;
CREATE INDEX idx_heuristics_tenant_id ON heuristics (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_heuristics_fts ON heuristics USING GIN (search_vector);
CREATE INDEX idx_heuristics_heuristic_key ON heuristics (heuristic_key);

-- Trigger: updated_at
CREATE TRIGGER trg_heuristics_updated_at
    BEFORE UPDATE ON heuristics
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- FTS trigger function: heuristics
CREATE OR REPLACE FUNCTION heuristics_fts_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.condition, '')), 'A') ||
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.action, '')), 'B') ||
        setweight(to_tsvector(NEW.fts_language, COALESCE(NEW.rationale, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_heuristics_fts
    BEFORE INSERT OR UPDATE ON heuristics
    FOR EACH ROW EXECUTE FUNCTION heuristics_fts_update();

-- =============================================================================
-- Table: relations
-- =============================================================================

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

COMMENT ON TABLE relations IS
    'No foreign keys on source_id/target_id: cross-aggregate references (memories, decisions, heuristics). '
    'Referential integrity enforced at application layer. Documented trade-off — see docs/superpowers/specs/2026-04-14-memory-engine-mvp-phase1-design.md section 6.';

-- Indexes: relations
CREATE INDEX idx_relations_source_id ON relations (source_id);
CREATE INDEX idx_relations_target_id ON relations (target_id);
CREATE INDEX idx_relations_project_id ON relations (project_id);
CREATE INDEX idx_relations_scope ON relations (project_id, repo_id, agent_id, session_id, environment);
CREATE INDEX idx_relations_type ON relations (relation_type);
CREATE INDEX idx_relations_tenant_id ON relations (tenant_id) WHERE tenant_id IS NOT NULL;

-- Trigger: updated_at
CREATE TRIGGER trg_relations_updated_at
    BEFORE UPDATE ON relations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- Table: purge_records
-- =============================================================================

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

-- Indexes: purge_records
CREATE INDEX idx_purge_records_project_id ON purge_records (project_id);
CREATE INDEX idx_purge_records_target_id ON purge_records (target_id);
CREATE INDEX idx_purge_records_status ON purge_records (status) WHERE status = 'pending';
CREATE INDEX idx_purge_records_tenant_id ON purge_records (tenant_id) WHERE tenant_id IS NOT NULL;

-- Trigger: updated_at
CREATE TRIGGER trg_purge_records_updated_at
    BEFORE UPDATE ON purge_records
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- Table: project_profiles
-- =============================================================================

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

-- Indexes: project_profiles
CREATE INDEX idx_project_profiles_project_id ON project_profiles (project_id);
CREATE INDEX idx_project_profiles_tenant_id ON project_profiles (tenant_id) WHERE tenant_id IS NOT NULL;

-- Trigger: updated_at
CREATE TRIGGER trg_project_profiles_updated_at
    BEFORE UPDATE ON project_profiles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- Table: domain_events (append-only)
-- =============================================================================

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

-- Indexes: domain_events
CREATE INDEX idx_domain_events_aggregate ON domain_events (aggregate_id, aggregate_type);
CREATE INDEX idx_domain_events_project_id ON domain_events (project_id);
CREATE INDEX idx_domain_events_event_type ON domain_events (event_type);
CREATE INDEX idx_domain_events_occurred_at ON domain_events (occurred_at DESC);
