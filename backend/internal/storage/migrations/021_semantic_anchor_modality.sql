-- Migration 021: Add modality column to semantic_anchors.
-- All existing anchors are 'text' by default.
ALTER TABLE semantic_anchors
    ADD COLUMN IF NOT EXISTS modality TEXT NOT NULL DEFAULT 'text';

-- Allow filtering anchors by modality in nearest-neighbor queries.
CREATE INDEX IF NOT EXISTS idx_semantic_anchors_modality
    ON semantic_anchors (tenant_id, modality);
