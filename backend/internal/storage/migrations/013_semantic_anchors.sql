-- 013_semantic_anchors.sql
BEGIN;

-- Needed for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS semantic_anchors (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  -- NULL = global anchor (shared across tenants)
  tenant_id        TEXT,

  -- human readable anchor name
  name             TEXT NOT NULL,

  -- optional routing target
  route_group      TEXT NOT NULL DEFAULT '',

  -- optional preferred models
  preferred_models JSONB NOT NULL DEFAULT '[]'::jsonb,

  -- embedding vector (OpenAI text-embedding-3-small = 1536 dims)
  embedding        vector(1536) NOT NULL,

  -- arbitrary metadata (intent label, description, etc)
  metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,

  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- helpful lookup index
CREATE INDEX IF NOT EXISTS idx_semantic_anchors_tenant
ON semantic_anchors (tenant_id);

-- ANN index (HNSW cosine similarity)
CREATE INDEX IF NOT EXISTS idx_semantic_anchors_hnsw
ON semantic_anchors
USING hnsw (embedding vector_cosine_ops);

COMMIT;