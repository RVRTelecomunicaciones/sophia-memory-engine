-- 005_fts_simple.down.sql
-- Reverse migration 005: restore FTS language from 'simple' back to 'spanish'.
-- The per-row UPDATE triggers rebuild search_vector using 'spanish' dictionary.
--
-- Idempotent: WHERE fts_language::text = 'simple' makes re-application a no-op.
--
-- This migration assigns 'spanish' to fts_language, which requires the
-- 'spanish' configuration to be present in pg_ts_config on the target
-- Postgres instance. If it is not (e.g. the default postgres:16 Docker
-- image), the migration is skipped via the EXISTS guard below rather
-- than failing — preserving rollback compatibility across environments.

BEGIN;

DO $$
DECLARE
    has_spanish boolean;
BEGIN
    SELECT EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname = 'spanish')
      INTO has_spanish;

    IF has_spanish THEN
        UPDATE memories   SET fts_language = 'spanish' WHERE fts_language::text = 'simple';
        UPDATE decisions  SET fts_language = 'spanish' WHERE fts_language::text = 'simple';
        UPDATE heuristics SET fts_language = 'spanish' WHERE fts_language::text = 'simple';

        ALTER TABLE memories   ALTER COLUMN fts_language SET DEFAULT 'spanish';
        ALTER TABLE decisions  ALTER COLUMN fts_language SET DEFAULT 'spanish';
        ALTER TABLE heuristics ALTER COLUMN fts_language SET DEFAULT 'spanish';
    ELSE
        RAISE NOTICE 'spanish FTS config not available in pg_ts_config; down migration is a no-op on this instance';
    END IF;
END $$;

COMMIT;
