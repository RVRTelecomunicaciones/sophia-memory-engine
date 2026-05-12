-- Memory Engine — API Keys (rollback)

DROP TRIGGER IF EXISTS trg_api_keys_updated_at ON api_keys;
DROP INDEX IF EXISTS idx_api_keys_project;
DROP INDEX IF EXISTS idx_api_keys_tenant;
DROP INDEX IF EXISTS idx_api_keys_status;
DROP TABLE IF EXISTS api_keys;
