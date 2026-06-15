-- SPEC_156: Add cost and api_key_name to anthropic_message_log.
-- cost      NUMERIC(18,8) — per-request computed cost; NULL when pricing unavailable.
-- api_key_name TEXT       — human-readable API key display name; NULL for JWT/YAML flows.
ALTER TABLE anthropic_message_log
    ADD COLUMN IF NOT EXISTS cost         NUMERIC(18,8),
    ADD COLUMN IF NOT EXISTS api_key_name TEXT;
