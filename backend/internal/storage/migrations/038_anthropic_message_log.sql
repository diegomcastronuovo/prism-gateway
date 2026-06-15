-- SPEC_154: Dedicated audit table for POST /v1/messages (Anthropic native passthrough).
-- Completely isolated from request_log, conversation_log, compliance_event_log.

CREATE TABLE IF NOT EXISTS anthropic_message_log (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    tenant_id TEXT NULL,
    request_id TEXT NOT NULL,

    api_key_id TEXT NULL,
    api_key_value TEXT NULL,      -- masked: prefix + ****last4
    jwt_sub TEXT NULL,

    provider TEXT NOT NULL DEFAULT 'anthropic',
    endpoint TEXT NOT NULL,
    http_method TEXT NOT NULL DEFAULT 'POST',

    model_requested TEXT NULL,
    model_used TEXT NULL,

    anthropic_message_id TEXT NULL,
    upstream_request_id TEXT NULL,

    status_code INTEGER NULL,
    success BOOLEAN NOT NULL DEFAULT FALSE,

    input_tokens INTEGER NULL,
    output_tokens INTEGER NULL,
    total_tokens INTEGER NULL,

    stop_reason TEXT NULL,
    stop_sequence TEXT NULL,

    prompt_text TEXT NULL,
    response_text TEXT NULL,

    raw_request_json JSONB NULL,
    raw_response_json JSONB NULL,

    error_type TEXT NULL,
    error_message TEXT NULL,

    latency_ms INTEGER NULL
);

CREATE INDEX IF NOT EXISTS idx_anthropic_message_log_created_at
    ON anthropic_message_log (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_anthropic_message_log_request_id
    ON anthropic_message_log (request_id);

CREATE INDEX IF NOT EXISTS idx_anthropic_message_log_tenant_id_created_at
    ON anthropic_message_log (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_anthropic_message_log_model_used_created_at
    ON anthropic_message_log (model_used, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_anthropic_message_log_success_created_at
    ON anthropic_message_log (success, created_at DESC);
