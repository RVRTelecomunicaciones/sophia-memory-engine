-- Memory Engine — API Keys
-- Phase 1.2: API-key authentication table with dev bootstrap seed.

-- =============================================================================
-- Table: api_keys
-- =============================================================================

CREATE TABLE api_keys (
    id              VARCHAR(26)  PRIMARY KEY,           -- ULID (key_id)
    key_hash        CHAR(64)     NOT NULL UNIQUE,        -- sha256(plaintext_secret) hex, lowercase
    tenant_id       VARCHAR(100),
    project_id      VARCHAR(100) NOT NULL,
    description     TEXT,
    status          VARCHAR(20)  NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'revoked')),
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Indexes: api_keys
CREATE INDEX idx_api_keys_status  ON api_keys (status)    WHERE status = 'active';
CREATE INDEX idx_api_keys_tenant  ON api_keys (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_api_keys_project ON api_keys (project_id);

-- Trigger: reuse set_updated_at() defined in 001_initial_schema.up.sql
CREATE TRIGGER trg_api_keys_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- Dev bootstrap key — LOCAL DEV ONLY
--
-- Plaintext secret (use this value in the X-API-Key header locally):
--   memdev_a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef
--
-- SHA-256 of the above (verify with):
--   printf "memdev_a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef" | shasum -a 256
--   => a91fcf692498eb1732105be597142d4653a19821b1fde29e3cbf306b6edc7ad5
--
-- WARNING: This key MUST be revoked before any non-local deployment.
--   UPDATE api_keys SET status = 'revoked' WHERE id = '01J0DEVKEY00000000000000XX';
-- =============================================================================

INSERT INTO api_keys (id, key_hash, tenant_id, project_id, description, status)
VALUES (
    '01J0DEVKEY00000000000000XX',
    'a91fcf692498eb1732105be597142d4653a19821b1fde29e3cbf306b6edc7ad5',
    'sophia-dev',
    'sophia-dev',
    'Local dev bootstrap key — ROTATE BEFORE ANY NON-LOCAL DEPLOYMENT',
    'active'
);
