-- Migration 023: Add model_benchmarks table for automatic benchmarking.
CREATE TABLE IF NOT EXISTS model_benchmarks (
    id               UUID             PRIMARY KEY,
    ts               TIMESTAMP        NOT NULL,
    provider         TEXT             NOT NULL,
    model            TEXT             NOT NULL,
    success          BOOLEAN          NOT NULL,
    latency_ms       BIGINT           NOT NULL,
    prompt_tokens    BIGINT           NOT NULL DEFAULT 0,
    completion_tokens BIGINT          NOT NULL DEFAULT 0,
    total_tokens     BIGINT           NOT NULL DEFAULT 0,
    cost_usd         DOUBLE PRECISION NOT NULL DEFAULT 0,
    error_type       TEXT             NOT NULL DEFAULT '',
    benchmark_name   TEXT             NOT NULL DEFAULT 'default'
);

CREATE INDEX IF NOT EXISTS idx_model_benchmarks_model_ts
    ON model_benchmarks (model, ts DESC);

CREATE INDEX IF NOT EXISTS idx_model_benchmarks_provider_ts
    ON model_benchmarks (provider, ts DESC);
