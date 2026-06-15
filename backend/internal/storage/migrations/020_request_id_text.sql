-- Migration 020: Allow arbitrary request IDs (e.g. "chatcmpl-mock-1772923005").
-- Existing UUID values are preserved as canonical UUID strings.
-- Postgres adjusts btree indexes on these columns automatically; no DROP/RECREATE needed.

ALTER TABLE request_log
    ALTER COLUMN request_id TYPE TEXT USING request_id::TEXT;

ALTER TABLE usage
    ALTER COLUMN request_id TYPE TEXT USING request_id::TEXT;
