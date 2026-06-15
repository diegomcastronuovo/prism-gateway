-- Migration 007: Rotatable API Keys
-- Creates api_keys table with SHA256 hashing, scopes, expiration, and audit trail

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,  -- sha256 hex (64 chars)
    prefix TEXT NOT NULL,             -- first 12 chars for display (e.g., "rk_live_abc")
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,           -- NULL = never expires
    revoked_at TIMESTAMPTZ,           -- NULL = active, NOT NULL = revoked
    last_used_at TIMESTAMPTZ,         -- Updated asynchronously on each use
    metadata JSONB DEFAULT '{}'::jsonb -- Extensible: rotation history, tags, etc.
);

-- Fast lookup for active keys by hash (most common query)
-- Partial index excludes revoked keys from index
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash
    ON api_keys(key_hash) WHERE revoked_at IS NULL;

-- List keys by tenant (admin UI)
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_created
    ON api_keys(tenant_id, created_at DESC);

-- Filter revoked keys (admin UI)
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_revoked
    ON api_keys(tenant_id, revoked_at) WHERE revoked_at IS NOT NULL;

-- Prevent duplicate active key names per tenant
-- Partial unique index: same name allowed only if one is revoked
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_tenant_name_active
    ON api_keys(tenant_id, name) WHERE revoked_at IS NULL;

-- Add comment for documentation
COMMENT ON TABLE api_keys IS 'Rotatable API keys with SHA256 hashing, scopes (inference, admin_read, admin_write), expiration, and revocation support';
COMMENT ON COLUMN api_keys.key_hash IS 'SHA256 hash of plaintext key (never store plaintext)';
COMMENT ON COLUMN api_keys.prefix IS 'First 12 chars of key for display (e.g., rk_live_abc), safe to show in logs/UI';
COMMENT ON COLUMN api_keys.scopes IS 'JSON array of scopes: ["inference"] for /v1/*, ["admin_read"] or ["admin_write"] for /admin/*';
COMMENT ON COLUMN api_keys.last_used_at IS 'Updated asynchronously via background goroutine (best-effort, non-blocking)';
COMMENT ON COLUMN api_keys.metadata IS 'Extensible JSON field for rotation history, tags, custom attributes';
