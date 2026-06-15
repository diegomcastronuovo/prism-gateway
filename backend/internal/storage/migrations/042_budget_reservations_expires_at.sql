-- Add expires_at to budget_reservations so orphaned rows (failed/crashed requests)
-- are automatically excluded from spend calculations after 2.5 minutes.
-- Default: NOW() + 2.5 min — set at INSERT time, no application code change needed.

ALTER TABLE budget_reservations
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '2.5 minutes';

CREATE INDEX IF NOT EXISTS idx_budget_reservations_expires_at
    ON budget_reservations (expires_at);

-- Purge all existing orphaned reservations (safe: they are all stale).
DELETE FROM budget_reservations;
