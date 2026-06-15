-- Migration 058: model_catalog pricing v2
--
-- Fixes existing installs that already ran the original migration 054 which
-- added long_context_multiplier instead of explicit pricing columns.
-- All statements are idempotent.

-- Drop old column added by original 054 (no-op when already absent)
ALTER TABLE model_catalog DROP COLUMN IF EXISTS long_context_multiplier;

-- Fix default value for long_context_start_tokens
-- (existing rows keep their stored value; only new rows get 272000)
ALTER TABLE model_catalog ALTER COLUMN long_context_start_tokens SET DEFAULT 272000;

-- Add new pricing columns (idempotent)
ALTER TABLE model_catalog
    ADD COLUMN IF NOT EXISTS cached_input_per_1m              NUMERIC(12,6) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS long_context_prompt_per_1m       NUMERIC(12,6) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS long_context_cached_input_per_1m NUMERIC(12,6) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS long_context_completion_per_1m   NUMERIC(12,6) NOT NULL DEFAULT 0;

-- Re-seed openai models: remove stale rows so the upsert below picks up all
-- new columns cleanly (existing installs may have rows without new columns set).
DELETE FROM model_catalog WHERE provider = 'openai';

INSERT INTO model_catalog
    (provider, id, display_name, type,
     prompt_per_1m, cached_input_per_1m, completion_per_1m, infrastructure_monthly_usd,
     is_active,
     long_context, long_context_start_tokens,
     long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m)
VALUES
    -- LLM models with long context pricing
    ('openai', 'gpt-5.5',       'GPT-5.5',       'LLM',   5.00,  0.50, 30.00, 0, TRUE, TRUE,  272000, 10.00, 1.00, 45.00),
    ('openai', 'gpt-5.5-pro',   'GPT-5.5 Pro',   'LLM',  30.00,  0.00,180.00, 0, TRUE, TRUE,  272000, 60.00, 0.00,270.00),
    ('openai', 'gpt-5.4',       'GPT-5.4',       'LLM',   2.50,  0.25, 15.00, 0, TRUE, TRUE,  272000,  5.00, 0.50, 22.50),
    ('openai', 'gpt-5.4-pro',   'GPT-5.4 Pro',   'LLM',  30.00,  0.00,180.00, 0, TRUE, TRUE,  272000, 60.00, 0.00,270.00),

    -- LLM models without long context
    ('openai', 'gpt-5.4-mini',               'GPT-5.4 Mini',               'LLM',   0.75,  0.075,   4.50, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5.4-nano',               'GPT-5.4 Nano',               'LLM',   0.20,  0.02,    1.25, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5.2',                    'GPT-5.2',                    'LLM',   1.75,  0.175,  14.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5.2-pro',                'GPT-5.2 Pro',                'LLM',  21.00,  0.00,  168.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5.1',                    'GPT-5.1',                    'LLM',   1.25,  0.125,  10.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5',                      'GPT-5',                      'LLM',   1.25,  0.125,  10.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5-mini',                 'GPT-5 Mini',                 'LLM',   0.25,  0.025,   2.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5-nano',                 'GPT-5 Nano',                 'LLM',   0.05,  0.005,   0.40, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-5-pro',                  'GPT-5 Pro',                  'LLM',  15.00,  0.00,  120.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4.1',                    'GPT-4.1',                    'LLM',   2.00,  0.50,    8.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4.1-mini',               'GPT-4.1 Mini',               'LLM',   0.40,  0.10,    1.60, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4.1-nano',               'GPT-4.1 Nano',               'LLM',   0.10,  0.025,   0.40, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4o',                     'GPT-4o',                     'LLM',   2.50,  1.25,   10.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4o-mini',                'GPT-4o Mini',                'LLM',   0.15,  0.075,   0.60, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'o4-mini',                    'o4 Mini',                    'LLM',   1.10,  0.275,   4.40, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'o3',                         'o3',                         'LLM',   2.00,  0.50,    8.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'o3-mini',                    'o3 Mini',                    'LLM',   1.10,  0.55,    4.40, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'o3-pro',                     'o3 Pro',                     'LLM',  20.00,  0.00,   80.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'o1',                         'o1',                         'LLM',  15.00,  7.50,   60.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'o1-mini',                    'o1 Mini',                    'LLM',   1.10,  0.55,    4.40, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'o1-pro',                     'o1 Pro',                     'LLM', 150.00,  0.00,  600.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4o-2024-05-13',          'GPT-4o (2024-05-13)',         'LLM',   5.00,  0.00,   15.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4-turbo-2024-04-09',     'GPT-4 Turbo (2024-04-09)',    'LLM',  10.00,  0.00,   30.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4-0125-preview',         'GPT-4 (0125 preview)',        'LLM',  10.00,  0.00,   30.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4-1106-preview',         'GPT-4 (1106 preview)',        'LLM',  10.00,  0.00,   30.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4-1106-vision-preview',  'GPT-4 (1106 vision preview)', 'LLM',  10.00,  0.00,   30.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4-0613',                 'GPT-4 (0613)',                'LLM',  30.00,  0.00,   60.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4-0314',                 'GPT-4 (0314)',                'LLM',  30.00,  0.00,   60.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-4-32k',                  'GPT-4 32K',                  'LLM',  60.00,  0.00,  120.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-3.5-turbo',              'GPT-3.5 Turbo',              'LLM',   0.50,  0.00,    1.50, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-3.5-turbo-0125',         'GPT-3.5 Turbo (0125)',       'LLM',   0.50,  0.00,    1.50, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-3.5-turbo-1106',         'GPT-3.5 Turbo (1106)',       'LLM',   1.00,  0.00,    2.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-3.5-turbo-0613',         'GPT-3.5 Turbo (0613)',       'LLM',   1.50,  0.00,    2.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-3.5-0301',               'GPT-3.5 (0301)',             'LLM',   1.50,  0.00,    2.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-3.5-turbo-instruct',     'GPT-3.5 Turbo Instruct',     'LLM',   1.50,  0.00,    2.00, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'gpt-3.5-turbo-16k-0613',     'GPT-3.5 Turbo 16K (0613)',   'LLM',   3.00,  0.00,    4.00, 0, TRUE, FALSE, 272000, 0, 0, 0),

    -- Embedding models
    ('openai', 'text-embedding-3-small', 'text-embedding-3-small', 'Embedding', 0.02, 0.00, 0, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'text-embedding-3-large', 'text-embedding-3-large', 'Embedding', 0.13, 0.00, 0, 0, TRUE, FALSE, 272000, 0, 0, 0),
    ('openai', 'text-embedding-ada-002', 'text-embedding-ada-002', 'Embedding', 0.10, 0.00, 0, 0, TRUE, FALSE, 272000, 0, 0, 0)

ON CONFLICT (provider, id) DO UPDATE SET
    display_name                      = EXCLUDED.display_name,
    type                              = EXCLUDED.type,
    prompt_per_1m                     = EXCLUDED.prompt_per_1m,
    cached_input_per_1m               = EXCLUDED.cached_input_per_1m,
    completion_per_1m                 = EXCLUDED.completion_per_1m,
    infrastructure_monthly_usd        = EXCLUDED.infrastructure_monthly_usd,
    is_active                         = EXCLUDED.is_active,
    long_context                      = EXCLUDED.long_context,
    long_context_start_tokens         = EXCLUDED.long_context_start_tokens,
    long_context_prompt_per_1m        = EXCLUDED.long_context_prompt_per_1m,
    long_context_cached_input_per_1m  = EXCLUDED.long_context_cached_input_per_1m,
    long_context_completion_per_1m    = EXCLUDED.long_context_completion_per_1m,
    updated_at                        = NOW();
