-- Migration 017: Add routing_snapshot column to request_log
-- Only populated for successful requests (status = 'ok').
-- Never contains prompts, responses, API keys, or PII.

ALTER TABLE request_log
    ADD COLUMN IF NOT EXISTS routing_snapshot JSONB DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_request_log_routing_snapshot
    ON request_log(request_id)
    WHERE routing_snapshot IS NOT NULL;

COMMENT ON COLUMN request_log.routing_snapshot IS
    'Flat routing decision: strategy, model, provider, candidates, fallback count, semantic fields. Only on success.';
