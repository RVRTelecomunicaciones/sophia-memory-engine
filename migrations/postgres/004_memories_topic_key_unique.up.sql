-- Memory Engine — P1.3 of ADR-0005: topic_key uniqueness on active memories.
--
-- Closes the silent-duplicate race: prior to this migration, two concurrent
-- ingest calls with the same (project_id, tenant_id, topic_key) could both
-- create active rows. The application Save path had no UNIQUE constraint to
-- serialize them, so race interleavings produced N>1 active rows for the
-- "same" topic.
--
-- Decision: a (project_id, COALESCE(tenant_id,''), topic_key) tuple is unique
-- ONLY among active rows. Archived / purged rows are exempt and may coexist
-- with a newer active row sharing the same topic_key — that is the documented
-- supersession semantics.
--
-- COALESCE(tenant_id, '') is required because PostgreSQL treats NULL as
-- distinct in unique indexes; without it, two NULL-tenant rows in the same
-- project with the same topic_key would both be allowed.
--
-- Partial unique indexes (WHERE clause) are supported in PG 11+; we target 16.

-- ---------------------------------------------------------------------------
-- Step 1 — Backfill: resolve any pre-existing duplicate active rows.
-- ---------------------------------------------------------------------------
-- For each (project_id, COALESCE(tenant_id,''), topic_key) collision among
-- active rows, keep the most recently created row active and archive the
-- rest with an explicit reason so audit/forensics can identify backfill
-- decisions later.
--
-- Idempotent: on a clean DB this UPDATE affects zero rows.

WITH dupes AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY project_id, COALESCE(tenant_id, ''), topic_key
            ORDER BY created_at DESC, id DESC
        ) AS rn
    FROM memories
    WHERE topic_key IS NOT NULL
      AND status = 'active'
)
UPDATE memories m
SET status = 'archived',
    archive_reason = 'P1.3 backfill: duplicate topic_key superseded',
    archived_by = 'migration:004_memories_topic_key_unique',
    updated_at = NOW()
FROM dupes d
WHERE m.id = d.id
  AND d.rn > 1;

-- ---------------------------------------------------------------------------
-- Step 2 — Create the partial unique index.
-- ---------------------------------------------------------------------------
-- After Step 1, this DDL must succeed unconditionally.

CREATE UNIQUE INDEX idx_memories_topic_key_active_unique
    ON memories (project_id, COALESCE(tenant_id, ''), topic_key)
    WHERE topic_key IS NOT NULL AND status = 'active';
