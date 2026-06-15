-- Migration 028: config-stage breakdown metrics on request_log (SPEC_86)
-- Additive and nullable to preserve existing rows/queries.

ALTER TABLE request_log
ADD COLUMN IF NOT EXISTS cfg_tool_routes_ms integer NULL,
ADD COLUMN IF NOT EXISTS cfg_dynamic_routes_ms integer NULL,
ADD COLUMN IF NOT EXISTS cfg_budget_pressure_ms integer NULL,
ADD COLUMN IF NOT EXISTS cfg_semantic_ms integer NULL,
ADD COLUMN IF NOT EXISTS cfg_model_resolution_ms integer NULL;
