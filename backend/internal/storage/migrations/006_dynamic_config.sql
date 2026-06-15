-- Dynamic tenant configuration table
CREATE TABLE IF NOT EXISTS tenants_config (
    tenant_id TEXT PRIMARY KEY,
    version INT NOT NULL DEFAULT 1,
    config_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by TEXT NOT NULL DEFAULT 'system'
);

CREATE INDEX IF NOT EXISTS idx_tenants_config_updated_at
ON tenants_config(updated_at DESC);

-- Config change log table (for audit trail)
-- Note: Uses TEXT[] for actor_roles to match Postgres array type
CREATE TABLE IF NOT EXISTS config_change_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id TEXT NOT NULL,
    actor_sub TEXT NOT NULL,
    actor_roles TEXT[] NOT NULL DEFAULT '{}',
    from_version INT NOT NULL,
    to_version INT NOT NULL,
    change_summary TEXT NOT NULL,
    diff_json JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_config_change_log_tenant_ts
ON config_change_log(tenant_id, ts DESC);
