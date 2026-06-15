ALTER TABLE model_catalog
    ADD COLUMN IF NOT EXISTS long_context               BOOLEAN      NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS long_context_start_tokens  INTEGER      NOT NULL DEFAULT 270000,
    ADD COLUMN IF NOT EXISTS long_context_multiplier    NUMERIC(5,2) NOT NULL DEFAULT 1;
