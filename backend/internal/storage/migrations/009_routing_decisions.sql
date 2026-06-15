-- Migration 009: Add routing decision tracking columns to request_log
-- This migration adds observability fields for the routing engine v2:
--   - decision_reason: Human-readable routing decision explanation
--   - error_type: Classified error type (timeout, rate_limited, etc.)
--   - decision_snapshot: Full decision context as JSON

-- Add decision tracking columns
ALTER TABLE request_log
    ADD COLUMN IF NOT EXISTS decision_reason TEXT;

ALTER TABLE request_log
    ADD COLUMN IF NOT EXISTS error_type TEXT;

ALTER TABLE request_log
    ADD COLUMN IF NOT EXISTS decision_snapshot JSONB;

-- Index for error type analytics
CREATE INDEX IF NOT EXISTS idx_request_log_error_type
    ON request_log(tenant_id, error_type, ts)
    WHERE error_type IS NOT NULL;

-- Backfill existing rows with sensible defaults (idempotent)
UPDATE request_log
SET
  decision_reason = COALESCE(decision_reason, 'legacy'),
  error_type      = CASE
                      WHEN status = 'error' AND error LIKE '%timeout%' THEN 'timeout'
                      WHEN status = 'error' AND error LIKE '%429%' THEN 'rate_limited'
                      WHEN status = 'error' AND (error LIKE '%5__' OR error LIKE '%500%' OR error LIKE '%502%' OR error LIKE '%503%') THEN 'upstream_5xx'
                      WHEN status = 'error' THEN 'unknown'
                      ELSE NULL
                    END
WHERE decision_reason IS NULL OR (status = 'error' AND error_type IS NULL);

-- Add comment for documentation
COMMENT ON COLUMN request_log.decision_reason IS 'Human-readable routing decision (e.g., "explicit_header|group:cheap|strategy:smart")';
COMMENT ON COLUMN request_log.error_type IS 'Classified error type: timeout, rate_limited, upstream_5xx, network, auth, invalid_request, unknown';
COMMENT ON COLUMN request_log.decision_snapshot IS 'Full decision context as JSON (precedence, smart, fallback)';
