-- Migration 018: Semantic cache for tenant-scoped prompt similarity caching.
-- Requires pgvector (migration 012).

CREATE TABLE IF NOT EXISTS semantic_cache (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     TEXT         NOT NULL,
    model         TEXT         NOT NULL,
    route_group   TEXT         NOT NULL DEFAULT '',
    embedding     vector(1536) NOT NULL,
    request_text  TEXT         NOT NULL DEFAULT '',
    response_json JSONB        NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ  NOT NULL,
    last_hit_at   TIMESTAMPTZ,
    hit_count     INT          NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_semantic_cache_tenant_expires
    ON semantic_cache (tenant_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_semantic_cache_tenant_model
    ON semantic_cache (tenant_id, model);
CREATE INDEX IF NOT EXISTS idx_semantic_cache_tenant_rg
    ON semantic_cache (tenant_id, route_group);
CREATE INDEX IF NOT EXISTS idx_semantic_cache_hnsw
    ON semantic_cache USING hnsw (embedding vector_cosine_ops);
