-- 014_anchor_unique.sql
-- Add unique constraint on (tenant_id, name) to semantic_anchors.
-- Required by POST /v1/semantic/anchors to detect duplicate anchor names per tenant.
BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS uq_semantic_anchors_tenant_name
ON semantic_anchors (tenant_id, name);

COMMIT;
