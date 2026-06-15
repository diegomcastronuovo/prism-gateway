-- Migration 064: Seed xAI Grok chat models.
-- Image, video, and voice models are excluded (non-token pricing, not applicable to LLM routing).
-- xAI publishes a single price tier even for 1M-context models — long_context = false.
-- Prices in USD per 1M tokens (June 2025, docs.x.ai/developers/models).
INSERT INTO model_catalog
    (provider, id, display_name, type,
     prompt_per_1m, cached_input_per_1m, completion_per_1m,
     cache_write_5m_per_1m, cache_write_1h_per_1m, geo_multiplier_us,
     infrastructure_monthly_usd, is_active,
     long_context, long_context_start_tokens,
     long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m)
VALUES
    ('xai', 'grok-4.3',
     'Grok 4.3', 'LLM',
     1.25, 0, 2.50,
     0, 0, 1.0,
     0, TRUE,
     FALSE, 272000, 0, 0, 0),

    ('xai', 'grok-4.20-0309-reasoning',
     'Grok 4.20 Reasoning', 'LLM',
     1.25, 0, 2.50,
     0, 0, 1.0,
     0, TRUE,
     FALSE, 272000, 0, 0, 0),

    ('xai', 'grok-4.20-0309-non-reasoning',
     'Grok 4.20', 'LLM',
     1.25, 0, 2.50,
     0, 0, 1.0,
     0, TRUE,
     FALSE, 272000, 0, 0, 0),

    ('xai', 'grok-4.20-multi-agent-0309',
     'Grok 4.20 Multi-Agent', 'LLM',
     1.25, 0, 2.50,
     0, 0, 1.0,
     0, TRUE,
     FALSE, 272000, 0, 0, 0),

    ('xai', 'grok-build-0.1',
     'Grok Build 0.1', 'LLM',
     1.00, 0, 2.00,
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
