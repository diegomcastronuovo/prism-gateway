-- Migration 008: Add request_id and attempt columns to request_log
-- Purpose: Support multiple log entries per logical request (retry/fallback scenarios)

-- 1) Add columns idempotently
ALTER TABLE request_log
    ADD COLUMN IF NOT EXISTS request_id UUID;

ALTER TABLE request_log
    ADD COLUMN IF NOT EXISTS attempt INT;

-- 2) Backfill existing rows (safe to re-run).
-- Both sides cast to TEXT so this statement is type-safe whether request_id is
-- still UUID (first run on a fresh DB) or has already been widened to TEXT by
-- migration 020 (any subsequent restart).  Postgres applies the assignment cast
-- TEXT→UUID automatically when the column is still UUID.
UPDATE request_log
SET
  request_id = COALESCE(request_id::text, id::text)::uuid,
  attempt    = COALESCE(attempt, 1)
WHERE request_id IS NULL OR attempt IS NULL;

-- 3) Enforce NOT NULL (safe to re-run)
ALTER TABLE request_log
    ALTER COLUMN request_id SET NOT NULL;

ALTER TABLE request_log
    ALTER COLUMN attempt SET NOT NULL;

-- 4) Indexes are already idempotent with IF NOT EXISTS
CREATE INDEX IF NOT EXISTS idx_request_log_request_id
    ON request_log(request_id);

CREATE INDEX IF NOT EXISTS idx_request_log_tenant_request
    ON request_log(tenant_id, request_id, attempt);