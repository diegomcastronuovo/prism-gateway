-- Migration 029: second-level breakdown for cfg_tool_routes_ms (SPEC_88)
-- Additive and nullable to preserve existing rows/queries.

ALTER TABLE request_log
ADD COLUMN IF NOT EXISTS tool_routes_embedding_model_ms integer NULL,
ADD COLUMN IF NOT EXISTS tool_routes_embedding_generate_ms integer NULL,
ADD COLUMN IF NOT EXISTS tool_routes_semantic_db_ms integer NULL,
ADD COLUMN IF NOT EXISTS tool_routes_match_eval_ms integer NULL;
