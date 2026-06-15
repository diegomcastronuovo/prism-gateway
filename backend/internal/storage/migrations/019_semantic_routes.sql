CREATE TABLE IF NOT EXISTS semantic_routes (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    tenant_id   TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    action      TEXT        NOT NULL,
    utterance   TEXT        NOT NULL,
    embedding   vector(1536) NOT NULL,
    threshold   FLOAT8      NOT NULL DEFAULT 0.80,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_semantic_routes_tenant
    ON semantic_routes (tenant_id);

-- Non-unique: multiple rows per (tenant_id, name) for multiple utterances
CREATE INDEX IF NOT EXISTS idx_semantic_routes_tenant_name
    ON semantic_routes (tenant_id, name);

CREATE INDEX IF NOT EXISTS idx_semantic_routes_hnsw
    ON semantic_routes USING hnsw (embedding vector_cosine_ops);
