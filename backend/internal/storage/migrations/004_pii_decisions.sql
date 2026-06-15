-- 004_pii_decisions.sql (idempotent)

-- Add PII webhook decision columns to request_log (safe if already applied)
ALTER TABLE request_log
  ADD COLUMN IF NOT EXISTS pii_webhook_request_decision  TEXT DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS pii_webhook_response_decision TEXT DEFAULT NULL;

-- Add index for audit export queries (tenant + timestamp range)
CREATE INDEX IF NOT EXISTS idx_request_log_tenant_ts
  ON request_log (tenant_id, ts);

COMMENT ON COLUMN request_log.pii_webhook_request_decision
  IS 'Decision from external PII webhook (request phase): allow, reject, modify';

COMMENT ON COLUMN request_log.pii_webhook_response_decision
  IS 'Decision from external PII webhook (response phase): allow, reject, modify';