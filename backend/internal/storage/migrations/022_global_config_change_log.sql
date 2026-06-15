-- Migration 022: Audit log for global configuration changes.
-- Analogous to config_change_log (per-tenant) but for the global config singleton.
CREATE TABLE IF NOT EXISTS global_config_change_log (
    id             BIGSERIAL    PRIMARY KEY,
    ts             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    actor_sub      TEXT         NOT NULL DEFAULT 'system',
    actor_roles    TEXT[]       NOT NULL DEFAULT '{}',
    from_version   BIGINT,                          -- NULL on first write
    to_version     BIGINT       NOT NULL,
    change_summary TEXT,
    diff_json      JSONB
);

CREATE INDEX IF NOT EXISTS idx_global_config_change_log_ts
    ON global_config_change_log (ts DESC);
