-- 035_tenant_environment.sql
-- Add environment field (DEV | STAGING | PROD) to existing tenant configs.
-- Tenants that do not already have an environment value are defaulted to DEV.
-- Only the active tenant config table (tenants_config) is updated; version history
-- is left as-is per SPEC_133 (only current active config must be normalised).

UPDATE tenants_config
SET    config_json = jsonb_set(config_json, '{environment}', '"DEV"'::jsonb, true),
       updated_at  = now()
WHERE  NOT (config_json ? 'environment');
