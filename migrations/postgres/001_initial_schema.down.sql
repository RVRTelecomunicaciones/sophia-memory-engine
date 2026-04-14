-- Memory Engine — Drop Initial Schema (reverse order)

-- Drop domain_events
DROP TABLE IF EXISTS domain_events CASCADE;

-- Drop project_profiles
DROP TRIGGER IF EXISTS trg_project_profiles_updated_at ON project_profiles;
DROP TABLE IF EXISTS project_profiles CASCADE;

-- Drop purge_records
DROP TRIGGER IF EXISTS trg_purge_records_updated_at ON purge_records;
DROP TABLE IF EXISTS purge_records CASCADE;

-- Drop relations
DROP TRIGGER IF EXISTS trg_relations_updated_at ON relations;
DROP TABLE IF EXISTS relations CASCADE;

-- Drop heuristics
DROP TRIGGER IF EXISTS trg_heuristics_fts ON heuristics;
DROP TRIGGER IF EXISTS trg_heuristics_updated_at ON heuristics;
DROP TABLE IF EXISTS heuristics CASCADE;
DROP FUNCTION IF EXISTS heuristics_fts_update();

-- Drop decisions
DROP TRIGGER IF EXISTS trg_decisions_fts ON decisions;
DROP TRIGGER IF EXISTS trg_decisions_updated_at ON decisions;
DROP TABLE IF EXISTS decisions CASCADE;
DROP FUNCTION IF EXISTS decisions_fts_update();

-- Drop memories
DROP TRIGGER IF EXISTS trg_memories_fts ON memories;
DROP TRIGGER IF EXISTS trg_memories_updated_at ON memories;
DROP TABLE IF EXISTS memories CASCADE;
DROP FUNCTION IF EXISTS memories_fts_update();

-- Drop shared function
DROP FUNCTION IF EXISTS set_updated_at();

-- Drop extensions
DROP EXTENSION IF EXISTS pg_trgm;
