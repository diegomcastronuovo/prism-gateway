-- Tracks daily aggregate statistics per tenant per model
CREATE TABLE IF NOT EXISTS model_stats_daily (
    date DATE NOT NULL,
    tenant_id TEXT NOT NULL,
    model TEXT NOT NULL,
    request_count INT NOT NULL DEFAULT 0,
    success_count INT NOT NULL DEFAULT 0,
    error_count INT NOT NULL DEFAULT 0,
    avg_latency_ms NUMERIC(10,2) NOT NULL DEFAULT 0,
    total_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (date, tenant_id, model)
);

CREATE INDEX IF NOT EXISTS idx_model_stats_tenant_date
ON model_stats_daily (tenant_id, date DESC);

CREATE INDEX IF NOT EXISTS idx_model_stats_date
ON model_stats_daily (date DESC);
