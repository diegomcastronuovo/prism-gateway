CREATE TABLE IF NOT EXISTS tool_catalog (
    id                TEXT         NOT NULL,
    provider          TEXT         NOT NULL,
    display_name      TEXT         NOT NULL DEFAULT '',
    tool_type         TEXT         NOT NULL DEFAULT '',
    unit              TEXT         NOT NULL DEFAULT 'call',
    price_per_unit    NUMERIC(12,6) NOT NULL DEFAULT 0,
    is_active         BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, id)
);
