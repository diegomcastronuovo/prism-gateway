-- Migration 060: Add Anthropic-specific pricing columns to model_catalog.
-- cache_write_5m_per_1m: price per 1M tokens for 5-min ephemeral cache writes
-- cache_write_1h_per_1m: price per 1M tokens for 1-hour ephemeral cache writes
-- geo_multiplier_us: cost multiplier when inference_geo == "us" (1.0 = no surcharge)
ALTER TABLE model_catalog
    ADD COLUMN IF NOT EXISTS cache_write_5m_per_1m  NUMERIC(14,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_write_1h_per_1m  NUMERIC(14,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS geo_multiplier_us       NUMERIC(5,4)  NOT NULL DEFAULT 1.0;
