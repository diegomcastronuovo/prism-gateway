-- Migration 063: Add deepseek and kimi to the active global config providers.
-- Targets existing installations where global_active_config already has a row.
-- New installations get these via the config.yaml seed (if_empty / always mode).
UPDATE global_config_versions
SET config_json = jsonb_set(
    jsonb_set(
        config_json,
        '{providers,deepseek}',
        '{"type":"openai","base_url":"https://api.deepseek.com/v1","api_key_env":"DEEPSEEK_API_KEY"}',
        true
    ),
    '{providers,kimi}',
    '{"type":"openai","base_url":"https://api.moonshot.cn/v1","api_key_env":"KIMI_API_KEY"}',
    true
)
WHERE id = (
    SELECT active_version FROM global_active_config WHERE id = 1
);
