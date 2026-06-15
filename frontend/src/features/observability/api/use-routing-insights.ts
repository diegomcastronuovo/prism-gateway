import { useQuery } from '@tanstack/react-query'
import { parseObservabilityErrorResponse } from './parse-observability-response'
import type {
  RoutingInsightsMetrics,
  RoutingDecision,
  StrategyDistribution,
  RouteGroupDistribution,
  ModelDistribution,
  RoutingInsightsFilters,
} from '../types/routing-insights'

interface RoutingInsightsData {
  metrics: RoutingInsightsMetrics
  decisions: RoutingDecision[]
  strategy_distribution: StrategyDistribution[]
  route_group_distribution: RouteGroupDistribution[]
  model_distribution: ModelDistribution[]
  pagination?: {
    limit: number
    offset: number
    returned: number
    total: number
  }
  traffic_over_time?: Array<{
    bucket: string
    requests: number
    successes: number
    errors: number
  }>
  summary?: {
    total_requests?: number
    success_rate?: number
    avg_latency_ms?: number
    fallback_rate?: number
    fallback_requests?: number
    cache_hit_rate?: number
  }
  status_breakdown?: {
    error?: number
    success?: number
  }
  provider_health?: Array<{
    provider: string
    avg_latency_ms: number
    success_rate: number
    total_requests: number
  }>
  window_hours?: number
}

export function useRoutingInsights(filters: RoutingInsightsFilters) {
  return useQuery<RoutingInsightsData>({
    queryKey: ['routing-insights', filters],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (filters.tenant_id) params.append('tenant_id', filters.tenant_id)
      if (filters.model) params.append('model', filters.model)
      if (filters.provider) params.append('provider', filters.provider)
      if (filters.status) params.append('status', filters.status)
      if (filters.fallback_used !== undefined) params.append('fallback_used', String(filters.fallback_used))
      if (filters.window_hours) params.append('window_hours', String(filters.window_hours))
      if (filters.limit) params.append('limit', String(filters.limit))
      if (filters.offset) params.append('offset', String(filters.offset))

      const res = await fetch(`/api/observability/routing-insights?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch routing insights')
      }
      return res.json()
    },
    gcTime: 0,
    refetchInterval: 30000,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}
