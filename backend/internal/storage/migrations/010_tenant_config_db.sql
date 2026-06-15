-- 010_tenant_config_db.sql
-- Per-tenant dynamic config with versioning + active pointer.

BEGIN;

-- 1) Versioned configs (append-only)
CREATE TABLE IF NOT EXISTS tenant_config_versions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id        TEXT NOT NULL,

  -- Monotonic version per tenant (1..n). Enforce uniqueness per tenant.
  version          BIGINT NOT NULL,

  -- Raw config (store as YAML text for now). You can add a parsed JSONB later if needed.
  config_yaml      TEXT NOT NULL,

  -- Integrity / traceability
  config_sha256    TEXT NOT NULL,

  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by       TEXT NOT NULL DEFAULT 'system',
  comment          TEXT NOT NULL DEFAULT '',

  CONSTRAINT tenant_config_versions_unique UNIQUE (tenant_id, version),
  CONSTRAINT tenant_config_versions_sha256_len CHECK (char_length(config_sha256) = 64)
);

CREATE INDEX IF NOT EXISTS idx_tenant_config_versions_tenant_created
  ON tenant_config_versions (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_tenant_config_versions_tenant_version
  ON tenant_config_versions (tenant_id, version DESC);

-- 2) Active config pointer (1 row per tenant)
CREATE TABLE IF NOT EXISTS tenant_active_config (
  tenant_id              TEXT PRIMARY KEY,
  active_config_id        UUID NOT NULL REFERENCES tenant_config_versions(id) ON DELETE RESTRICT,
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by             TEXT NOT NULL DEFAULT 'system',
  change_reason          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_tenant_active_config_active_id
  ON tenant_active_config (active_config_id);

-- 3) Ensure active_config_id belongs to same tenant (strong correctness)
-- We do this via a trigger because PG doesn't support cross-table CHECK constraints.
CREATE OR REPLACE FUNCTION enforce_active_config_same_tenant()
RETURNS trigger AS $$
DECLARE
  cfg_tenant TEXT;
BEGIN
  SELECT tenant_id INTO cfg_tenant
  FROM tenant_config_versions
  WHERE id = NEW.active_config_id;

  IF cfg_tenant IS NULL THEN
    RAISE EXCEPTION 'active_config_id % does not exist', NEW.active_config_id;
  END IF;

  IF cfg_tenant <> NEW.tenant_id THEN
    RAISE EXCEPTION 'active_config_id tenant_id % does not match row tenant_id %', cfg_tenant, NEW.tenant_id;
  END IF;

  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_enforce_active_config_same_tenant ON tenant_active_config;
CREATE TRIGGER trg_enforce_active_config_same_tenant
BEFORE INSERT OR UPDATE OF tenant_id, active_config_id
ON tenant_active_config
FOR EACH ROW
EXECUTE FUNCTION enforce_active_config_same_tenant();

COMMIT;