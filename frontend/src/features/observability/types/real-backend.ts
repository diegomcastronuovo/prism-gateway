// Types for REAL backend endpoints only

export interface RealMetrics {
  total_requests: number | null
  total_cost: number | null
  success_rate: number | null
  avg_latency_ms: number | null
}

export interface ModelPerformance {
  model: string
  provider: string
  avg_latency_ms: number
  p95_latency_ms: number
  success_rate: number
  avg_cost_usd: number
  samples: number
}
