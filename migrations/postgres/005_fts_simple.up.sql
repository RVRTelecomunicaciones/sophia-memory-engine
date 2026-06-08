-- 005_fts_simple.up.sql
-- Switch FTS language from 'spanish' to 'simple' across all 3 FTS tables.
-- The existing BEFORE INSERT/UPDATE triggers (trg_memories_fts,
-- trg_decisions_fts, trg_heuristics_fts) read NEW.fts_language per row
-- and rebuild search_vector. A no-op UPDATE forces the trigger to run,
-- rebuilding search_vector with the 'simple' dictionary so English (and
-- any other language) content is indexed without silent token loss.
--
-- Idempotent: WHERE fts_language = 'spanish' makes re-application a no-op.
-- Round-trip safe: paired with 005_fts_simple.down.sql.

BEGIN;

UPDATE memories   SET fts_language = 'simple' WHERE fts_language = 'spanish';
UPDATE decisions  SET fts_language = 'simple' WHERE fts_language = 'spanish';
UPDATE heuristics SET fts_language = 'simple' WHERE fts_language = 'spanish';

ALTER TABLE memories   ALTER COLUMN fts_language SET DEFAULT 'simple';
ALTER TABLE decisions  ALTER COLUMN fts_language SET DEFAULT 'simple';
ALTER TABLE heuristics ALTER COLUMN fts_language SET DEFAULT 'simple';

COMMIT;
