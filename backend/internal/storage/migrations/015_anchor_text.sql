ALTER TABLE semantic_anchors
    ADD COLUMN IF NOT EXISTS anchor_text TEXT;
