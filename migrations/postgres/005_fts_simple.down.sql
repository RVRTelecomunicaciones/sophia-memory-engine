-- 005_fts_simple.down.sql
-- Reverse migration 005: restore FTS language from 'simple' back to 'spanish'.
-- The per-row UPDATE triggers rebuild search_vector using 'spanish' dictionary.
--
-- Idempotent: WHERE fts_language = 'simple' makes re-application a no-op.

BEGIN;

UPDATE memories   SET fts_language = 'spanish' WHERE fts_language = 'simple';
UPDATE decisions  SET fts_language = 'spanish' WHERE fts_language = 'simple';
UPDATE heuristics SET fts_language = 'spanish' WHERE fts_language = 'simple';

ALTER TABLE memories   ALTER COLUMN fts_language SET DEFAULT 'spanish';
ALTER TABLE decisions  ALTER COLUMN fts_language SET DEFAULT 'spanish';
ALTER TABLE heuristics ALTER COLUMN fts_language SET DEFAULT 'spanish';

COMMIT;
