-- Enterprise metadata columns on request_log (all nullable; set by enterprise enrichment middleware).
ALTER TABLE request_log
    ADD COLUMN IF NOT EXISTS customer_id       TEXT NULL,
    ADD COLUMN IF NOT EXISTS channel           TEXT NULL,
    ADD COLUMN IF NOT EXISTS interaction_type  TEXT NULL,
    ADD COLUMN IF NOT EXISTS agent_id          TEXT NULL,
    ADD COLUMN IF NOT EXISTS department        TEXT NULL,
    ADD COLUMN IF NOT EXISTS ticket_id         TEXT NULL,
    ADD COLUMN IF NOT EXISTS customer_segment  TEXT NULL,
    ADD COLUMN IF NOT EXISTS language          TEXT NULL,
    ADD COLUMN IF NOT EXISTS intent            TEXT NULL,
    ADD COLUMN IF NOT EXISTS experiment_id     TEXT NULL,
    ADD COLUMN IF NOT EXISTS autonomy_level    TEXT NULL,
    ADD COLUMN IF NOT EXISTS policy_id         TEXT NULL,
    ADD COLUMN IF NOT EXISTS risk_level        TEXT NULL,
    ADD COLUMN IF NOT EXISTS revenue_impact    TEXT NULL,
    ADD COLUMN IF NOT EXISTS currency          TEXT NULL;
