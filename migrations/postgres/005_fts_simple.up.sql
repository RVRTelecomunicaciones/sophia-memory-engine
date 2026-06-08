-- 005_fts_simple.up.sql
-- Switch FTS language from 'spanish' to 'simple' across all 3 FTS tables.
-- The existing BEFORE INSERT/UPDATE triggers (trg_memories_fts,
-- trg_decisions_fts, trg_heuristics_fts) read NEW.fts_language per row
-- and rebuild search_vector. A no-op UPDATE forces the trigger to run,
-- rebuilding search_vector with the 'simple' dictionary so English (and
-- any other language) content is indexed without silent token loss.
--
-- Idempotent: WHERE fts_language::text = 'spanish' makes re-application a
-- no-op.
--
-- The ::text cast on the LHS avoids forcing Postgres to resolve 'spanish'
-- as a regconfig via pg_ts_config. Some Postgres installations (e.g. the
-- default postgres:16 Docker image used in CI) ship without the 'spanish'
-- FTS configuration. Comparing fts_language::text avoids that lookup; the
-- migration is a no-op there because no row will ever have been stored
-- with fts_language = 'spanish' on a host that cannot resolve it.
--
-- The 'simple' configuration is always available (built-in), so the SET
-- expression and the ALTER ... SET DEFAULT cannot fail on the same grounds.
--
-- Round-trip safe: paired with 005_fts_simple.down.sql.

BEGIN;

UPDATE memories   SET fts_language = 'simple' WHERE fts_language::text = 'spanish';
UPDATE decisions  SET fts_language = 'simple' WHERE fts_language::text = 'spanish';
UPDATE heuristics SET fts_language = 'simple' WHERE fts_language::text = 'spanish';

ALTER TABLE memories   ALTER COLUMN fts_language SET DEFAULT 'simple';
ALTER TABLE decisions  ALTER COLUMN fts_language SET DEFAULT 'simple';
ALTER TABLE heuristics ALTER COLUMN fts_language SET DEFAULT 'simple';

COMMIT;
