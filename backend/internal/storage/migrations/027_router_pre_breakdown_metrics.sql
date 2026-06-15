-- Migration 027: router_pre breakdown metrics on request_log (SPEC_85)
-- Additive and nullable to preserve existing rows/queries.

ALTER TABLE request_log
ADD COLUMN IF NOT EXISTS pre_decode_ms integer NULL,
ADD COLUMN IF NOT EXISTS pre_authz_ms integer NULL,
ADD COLUMN IF NOT EXISTS pre_tenant_config_ms integer NULL,
ADD COLUMN IF NOT EXISTS pre_pii_ms integer NULL,
ADD COLUMN IF NOT EXISTS pre_rate_limit_ms integer NULL,
ADD COLUMN IF NOT EXISTS pre_model_filter_ms integer NULL,
ADD COLUMN IF NOT EXISTS pre_routing_ms integer NULL,
ADD COLUMN IF NOT EXISTS pre_request_build_ms integer NULL;
