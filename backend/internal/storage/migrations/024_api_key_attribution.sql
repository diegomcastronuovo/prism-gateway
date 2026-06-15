-- Migration 024: API key attribution for request_log and usage
-- Enables per-key usage analytics, cost attribution, and request attribution.
-- Historical rows remain valid with NULL attribution.

ALTER TABLE request_log
ADD COLUMN IF NOT EXISTS api_key_id uuid NULL,
ADD COLUMN IF NOT EXISTS api_key_name text NULL;

ALTER TABLE usage
ADD COLUMN IF NOT EXISTS api_key_id uuid NULL,
ADD COLUMN IF NOT EXISTS api_key_name text NULL;
