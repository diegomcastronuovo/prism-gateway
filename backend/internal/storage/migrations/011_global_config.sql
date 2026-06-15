-- 011_global_config.sql

CREATE TABLE IF NOT EXISTS global_config_versions (
  id          BIGSERIAL PRIMARY KEY,
  config_json JSONB NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by  TEXT NOT NULL DEFAULT 'system'
);

CREATE TABLE IF NOT EXISTS global_active_config (
  id             INT PRIMARY KEY DEFAULT 1,
  active_version BIGINT NOT NULL REFERENCES global_config_versions(id) ON DELETE RESTRICT,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'global_active_config_singleton'
  ) THEN
    ALTER TABLE global_active_config
      ADD CONSTRAINT global_active_config_singleton CHECK (id = 1);
  END IF;
END
$$;