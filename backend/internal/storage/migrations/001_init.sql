CREATE TABLE IF NOT EXISTS request_log (
    id UUID PRIMARY KEY,
    ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id TEXT NOT NULL,
    model TEXT NOT NULL,
    provider TEXT NOT NULL,
    strategy TEXT NOT NULL,
    status TEXT NOT NULL,
    latency_ms INT,
    error TEXT,
    fallback_used BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS usage (
    id UUID PRIMARY KEY,
    ts TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id TEXT NOT NULL,
    model TEXT NOT NULL,
    provider TEXT NOT NULL,
    prompt_tokens INT NOT NULL,
    completion_tokens INT NOT NULL,
    total_tokens INT NOT NULL,
    cost_usd NUMERIC(12,6) NOT NULL,
    request_id UUID NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_usage_tenant_ts
ON usage (tenant_id, ts);
