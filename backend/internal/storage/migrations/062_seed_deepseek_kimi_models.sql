-- Migration 062: Seed DeepSeek and Kimi (Moonshot AI) models.
-- Both providers use OpenAI-compatible /v1/chat/completions with streaming.
--
-- DeepSeek pricing (June 2025, standard rates after promo ended 2026-05-31):
--   deepseek-v4-flash: $0.14 input / $0.28 output / $0.0028 cache hit
--   deepseek-v4-pro:   $1.74 input / $3.48 output / $0.0145 cache hit
--
-- Kimi K2.6 pricing (June 2025):
--   kimi-k2.6: $0.95 input / $0.16 cache hit / $4.00 output / 262K context
INSERT INTO model_catalog
    (provider, id, display_name, type,
     prompt_per_1m, cached_input_per_1m, completion_per_1m,
     cache_write_5m_per_1m, cache_write_1h_per_1m, geo_multiplier_us,
     infrastructure_monthly_usd, is_active,
     long_context, long_context_start_tokens,
     long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m)
VALUES
    -- DeepSeek V4 Flash — fast, low cost
    ('deepseek', 'deepseek-v4-flash',
     'DeepSeek V4 Flash', 'LLM',
     0.14, 0.0028, 0.28,
     0, 0, 1.0,
     0, TRUE,
     FALSE, 272000, 0, 0, 0),

    -- DeepSeek V4 Pro — standard rates (promo ended 2026-05-31)
    ('deepseek', 'deepseek-v4-pro',
     'DeepSeek V4 Pro', 'LLM',
     1.74, 0.0145, 3.48,
     0, 0, 1.0,
     0, TRUE,
     FALSE, 272000, 0, 0, 0),

    -- Kimi K2.6 — 262K context window (below long-context threshold, no tier applies)
    ('kimi', 'kimi-k2.6',
     'Kimi K2.6', 'LLM',
     0.95, 0.16, 4.00,
     0, 0, 1.0,
     0, TRUE,
     FALSE, 272000, 0, 0, 0)
ON CONFLICT (provider, id) DO UPDATE SET
    display_name          = EXCLUDED.display_name,
    type                  = EXCLUDED.type,
    prompt_per_1m         = EXCLUDED.prompt_per_1m,
    cached_input_per_1m   = EXCLUDED.cached_input_per_1m,
    completion_per_1m     = EXCLUDED.completion_per_1m,
    cache_write_5m_per_1m = EXCLUDED.cache_write_5m_per_1m,
    cache_write_1h_per_1m = EXCLUDED.cache_write_1h_per_1m,
    geo_multiplier_us     = EXCLUDED.geo_multiplier_us,
    is_active             = EXCLUDED.is_active;
