-- Fix column name mismatch in anthropic_message_log introduced during SPEC_154 development.
-- The table was initially created with wt_sub; code now uses jwt_sub.
-- Handles three cases idempotently:
--   1. Column exists as wt_sub  → rename to jwt_sub
--   2. Column already jwt_sub   → no-op
--   3. Column missing entirely  → add jwt_sub
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'anthropic_message_log' AND column_name = 'wt_sub'
    ) THEN
        ALTER TABLE anthropic_message_log RENAME COLUMN wt_sub TO jwt_sub;
    ELSIF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'anthropic_message_log' AND column_name = 'jwt_sub'
    ) THEN
        ALTER TABLE anthropic_message_log ADD COLUMN jwt_sub TEXT NULL;
    END IF;
END $$;
