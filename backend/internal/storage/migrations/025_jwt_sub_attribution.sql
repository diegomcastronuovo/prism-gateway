-- Migration 025: JWT subject attribution for request_log and usage (SPEC_59)
-- Nullable columns; historical rows remain valid.

ALTER TABLE request_log
ADD COLUMN IF NOT EXISTS jwt_sub text NULL;

ALTER TABLE usage
ADD COLUMN IF NOT EXISTS jwt_sub text NULL;
