CREATE TABLE IF NOT EXISTS model_catalog (
    id                          TEXT            NOT NULL,
    provider                    TEXT            NOT NULL,
    display_name                TEXT            NOT NULL DEFAULT '',
    type                        TEXT            NOT NULL DEFAULT 'chat',
    prompt_per_1m               NUMERIC(12,6)   NOT NULL DEFAULT 0,
    completion_per_1m           NUMERIC(12,6)   NOT NULL DEFAULT 0,
    infrastructure_monthly_usd  NUMERIC(10,2)   NOT NULL DEFAULT 0,
    is_active                   BOOLEAN         NOT NULL DEFAULT TRUE,
    created_at                  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, id)
);
CREATE INDEX IF NOT EXISTS idx_model_catalog_provider ON model_catalog(provider);
CREATE INDEX IF NOT EXISTS idx_model_catalog_is_active ON model_catalog(is_active);
