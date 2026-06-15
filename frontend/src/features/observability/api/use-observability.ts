import { useQuery } from '@tanstack/react-query'
import type { ObservabilityMetrics, RequestLogEntry, CostAnalytics, TrafficDataPoint, ProviderHealth, RequestLogsFilters } from '../types'
import { parseObservabilityErrorResponse } from './parse-observability-response'

interface ObservabilityMetricsData {
  metrics: ObservabilityMetrics
  traffic_data: TrafficDataPoint[]
  provider_health: ProviderHealth[]
}

interface LogsPagination {
  limit: number
  offset: number
  returned: number
  total: number
}

export interface RequestLogsData {
  logs: RequestLogEntry[]
  total: number
  pagination: LogsPagination
}

export function useObservabilityMetrics(windowHours = 24, bucket = 'hour', enabled = true) {
  return useQuery<ObservabilityMetricsData>({
    queryKey: ['observability', 'metrics', windowHours, bucket],
    queryFn: async () => {
      const params = new URLSearchParams({
        window_hours: String(windowHours),
        bucket,
      })
      const res = await fetch(`/api/observability/metrics?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch metrics')
      }
      return res.json()
    },
    enabled,
    gcTime: 0,
    refetchInterval: 30000,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
    retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 10000),
    staleTime: 10000,
  })
}

export function useRequestLogs(
  limit = 50,
  offset = 0,
  filters?: RequestLogsFilters,
  enabled = true
) {
  return useQuery<RequestLogsData>({
    queryKey: ['observability', 'logs', limit, offset, filters],
    queryFn: async () => {
      const params = new URLSearchParams()
      params.append('limit', String(limit))
      params.append('offset', String(offset))
      if (filters?.tenant_id) params.append('tenant_id', filters.tenant_id)
      if (filters?.model) params.append('model', filters.model)
      if (filters?.provider) params.append('provider', filters.provider)
      if (filters?.status) params.append('status', filters.status)
      if (filters?.time_range) params.append('time_range', filters.time_range)

      const res = await fetch(`/api/observability/logs?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch logs')
      }
      return res.json()
    },
    enabled,
    gcTime: 0,
    refetchInterval: 30000,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}

export function useCostAnalytics(enabled = true) {
  return useQuery<CostAnalytics[]>({
    queryKey: ['observability', 'cost-analytics'],
    queryFn: async () => {
      const res = await fetch('/api/observability/cost-analytics', {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch cost analytics')
      }
      return res.json()
    },
    enabled,
    gcTime: 0,
    refetchInterval: 30000,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}
