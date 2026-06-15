-- Migration 037: Widen cost_usd precision from NUMERIC(12,6) to NUMERIC(18,10)
--
-- NUMERIC(12,6) truncates any cost smaller than $0.000001 to exactly 0.
-- For example, embedding calls (text-embedding-3-small at $0.02/1M) with ~20 tokens
-- produce $0.00000042, which was silently stored as $0.00 causing FinOps to show $0.00
-- even when a real (tiny) cost was incurred.
--
-- NUMERIC(18,10) stores up to 10 decimal places, correctly preserving values down to
-- $0.0000000001 ($10^-10), sufficient for any current embedding or LLM pricing.

ALTER TABLE usage
    ALTER COLUMN cost_usd TYPE NUMERIC(18,10);

ALTER TABLE budget_reservations
    ALTER COLUMN estimated_cost_usd TYPE NUMERIC(18,10);

ALTER TABLE model_stats_daily
    ALTER COLUMN total_cost_usd TYPE NUMERIC(18,10);
