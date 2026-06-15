-- Migration 026: router-only performance metrics on request_log (SPEC_router_performance_metrics_backend)
-- Additive and nullable to preserve existing rows/queries.

ALTER TABLE request_log
ADD COLUMN IF NOT EXISTS router_pre_ms integer NULL,
ADD COLUMN IF NOT EXISTS llm_latency_ms integer NULL,
ADD COLUMN IF NOT EXISTS router_post_ms integer NULL;
