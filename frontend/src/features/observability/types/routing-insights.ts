export interface RoutingInsightsMetrics {
  total_routed_requests: number | null
  smart_routing_usage_pct: number | null
  fallback_usage_pct: number | null
  route_group_usage_pct?: number | null
  tool_routing_matches?: number | null
  semantic_cache_hit_pct?: number | null
}

export interface RoutingDecision {
  id: string
  timestamp: string
  tenant_id: string
  request_id: string
  selected_model: string
  provider: string
  strategy: string
  route_group?: string
  fallback_used: boolean
  fallback_attempts?: number
  status: string
  latency_ms: number
  attempt?: number
  cache_status?: string
  decision_reason?: string
  error_type?: string
  routing_snapshot?: Record<string, unknown>
  raw_request?: Record<string, unknown>
  decision_snapshot?: DecisionSnapshot
}

export interface DecisionSnapshot {
  plan?: string[]
  smart?: {
    preferred_models?: string[]
    stages_evaluated?: string[]
  }
  candidate_models?: string[]
  routing_strategy?: string
  fallback_attempts?: number
}

export interface RoutingSnapshot {
  request_id: string
  routing_snapshot: {
    provider: string
    timestamp: string
    route_group?: string
    selected_model: string
    candidate_models: string[]
    routing_strategy: string
    fallback_attempts: number
  }
  tenant_id: string
}

export interface StrategyDistribution {
  strategy: string
  count: number
}

export interface RouteGroupDistribution {
  route_group: string
  count: number
}

export interface ModelDistribution {
  model: string
  count: number
}

export interface RoutingInsightsFilters {
  tenant_id?: string
  model?: string
  provider?: string
  strategy?: string
  route_group?: string
  status?: string
  fallback_used?: boolean
  window_hours?: number
  time_range?: string
  limit?: number
  offset?: number
}
