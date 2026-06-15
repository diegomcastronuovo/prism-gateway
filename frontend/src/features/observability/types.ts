export interface ObservabilityMetrics {
  total_requests_24h: number | null
  success_rate: number | null
  avg_latency_ms: number | null
  cache_hit_rate: number | null
  fallback_rate: number | null
  fallback_count: number | null
  total_cost_24h: number | null
  trends?: {
    requests_trend_pct?: number
    success_rate_trend_pct?: number
    latency_trend_pct?: number
    cache_hit_trend_pct?: number
    fallback_trend_pct?: number
    cost_trend_pct?: number
  }
}

export interface RequestLogEntry {
  timestamp: string
  tenant_id: string
  model: string
  provider: string
  latency_ms: number
  status: string
  fallback_used: boolean
  cache_hit?: boolean
  cache_status?: string
}

export interface CostAnalytics {
  model: string
  requests: number
  avg_latency: number
  total_cost: number
  success_rate: number
}

export interface TimeSeriesDataPoint {
  timestamp: string
  value: number
}

export interface LatencyDistribution {
  bucket: string
  count: number
}

export interface ProviderErrorRate {
  provider: string
  error_rate: number
  total_requests: number
}

export interface CostByModel {
  model: string
  cost: number
}

export interface RequestLogsFilters {
  tenant_id?: string
  model?: string
  provider?: string
  status?: string
  time_range?: string
}

export interface TrafficDataPoint {
  time_bucket: string
  requests: number
  successes?: number
  errors?: number
}

export interface ProviderHealth {
  provider: string
  success_rate: number
  avg_latency: number
  total_requests: number
}
