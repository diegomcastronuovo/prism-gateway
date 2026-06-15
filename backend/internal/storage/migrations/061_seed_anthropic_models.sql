-- Migration 061: Seed Anthropic models with June 2025 pricing.
-- Source: anthropic.com/pricing (June 2025)
-- Deprecated and retired models excluded (Opus 4, Sonnet 4, Haiku 3.5).

-- Deactivate any previously seeded anthropic entries not in the current list.
UPDATE model_catalog SET is_active = FALSE WHERE provider = 'anthropic';

INSERT INTO model_catalog
    (provider, id, display_name, type,
     prompt_per_1m, cached_input_per_1m, completion_per_1m,
     cache_write_5m_per_1m, cache_write_1h_per_1m, geo_multiplier_us,
     infrastructure_monthly_usd, is_active,
     long_context, long_context_start_tokens,
     long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m)
VALUES
    -- Opus 4.x family ($5 input tier)
    ('anthropic', 'claude-opus-4-8',
     'Claude Opus 4.8', 'LLM',
     5.00, 0.50, 25.00,
     6.25, 10.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0),

    ('anthropic', 'claude-opus-4-7',
     'Claude Opus 4.7', 'LLM',
     5.00, 0.50, 25.00,
     6.25, 10.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0),

    ('anthropic', 'claude-opus-4-6',
     'Claude Opus 4.6', 'LLM',
     5.00, 0.50, 25.00,
     6.25, 10.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0),

    ('anthropic', 'claude-opus-4-5',
     'Claude Opus 4.5', 'LLM',
     5.00, 0.50, 25.00,
     6.25, 10.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0),

    -- Opus 4.1 ($15 input tier)
    ('anthropic', 'claude-opus-4-1',
     'Claude Opus 4.1', 'LLM',
     15.00, 1.50, 75.00,
     18.75, 30.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0),

    -- Sonnet 4.x family
    ('anthropic', 'claude-sonnet-4-6',
     'Claude Sonnet 4.6', 'LLM',
     3.00, 0.30, 15.00,
     3.75, 6.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0),

    ('anthropic', 'claude-sonnet-4-5',
     'Claude Sonnet 4.5', 'LLM',
     3.00, 0.30, 15.00,
     3.75, 6.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0),

    -- Haiku 4.5
    ('anthropic', 'claude-haiku-4-5',
     'Claude Haiku 4.5', 'LLM',
     1.00, 0.10, 5.00,
     1.25, 2.00, 1.0,
     0, TRUE,
     FALSE, 0, 0, 0, 0)

ON CONFLICT (provider, id) DO UPDATE SET
    display_name              = EXCLUDED.display_name,
    prompt_per_1m             = EXCLUDED.prompt_per_1m,
    cached_input_per_1m       = EXCLUDED.cached_input_per_1m,
    completion_per_1m         = EXCLUDED.completion_per_1m,
    cache_write_5m_per_1m     = EXCLUDED.cache_write_5m_per_1m,
    cache_write_1h_per_1m     = EXCLUDED.cache_write_1h_per_1m,
    geo_multiplier_us         = EXCLUDED.geo_multiplier_us,
    is_active                 = EXCLUDED.is_active,
    long_context              = EXCLUDED.long_context,
    long_context_start_tokens = EXCLUDED.long_context_start_tokens,
    long_context_prompt_per_1m = EXCLUDED.long_context_prompt_per_1m,
    long_context_cached_input_per_1m = EXCLUDED.long_context_cached_input_per_1m,
    long_context_completion_per_1m = EXCLUDED.long_context_completion_per_1m;
