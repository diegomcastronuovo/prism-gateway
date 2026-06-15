CREATE TABLE IF NOT EXISTS budget_reservations (
    id UUID PRIMARY KEY,
    ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id TEXT NOT NULL,
    estimated_cost_usd NUMERIC(12,6) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_budget_reservations_tenant_ts
ON budget_reservations (tenant_id, ts);
