import { useQuery } from '@tanstack/react-query'
import type { RealMetrics, ModelPerformance } from '../types/real-backend'
import { parseObservabilityErrorResponse } from './parse-observability-response'

function toNumber(value: unknown, fallback = 0): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

export function useRealMetrics(
  tenantId: string = 'tenant_a',
  month: string = '2026-03',
  windowDays: number = 7,
  enabled = true
) {
  return useQuery<RealMetrics>({
    queryKey: ['real-metrics', tenantId, month, windowDays],
    queryFn: async () => {
      const params = new URLSearchParams({
        tenant_id: tenantId,
        month,
        window_days: String(windowDays),
      })
      const res = await fetch(`/api/observability/real-metrics?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch real metrics')
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

export function useModelPerformance(windowHours: number = 24, enabled = true) {
  return useQuery<ModelPerformance[]>({
    queryKey: ['model-performance', windowHours],
    queryFn: async () => {
      const res = await fetch(`/api/observability/model-performance?window_hours=${windowHours}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch model performance')
      }
      const json = await res.json()

      const rawItems: Record<string, unknown>[] = Array.isArray(json)
        ? json
        : Array.isArray(json?.models)
          ? json.models
          : Array.isArray(json?.items)
            ? json.items
            : Array.isArray(json?.data)
              ? json.data
              : []

      return rawItems.map((item) => {
        const successRateRaw = toNumber(item.success_rate ?? item.successRate ?? 0)
        const normalizedSuccessRate = successRateRaw > 1 ? successRateRaw / 100 : successRateRaw

        return {
          model: String(item.model ?? item.model_name ?? 'unknown'),
          provider: String(item.provider ?? 'unknown'),
          avg_latency_ms: toNumber(item.avg_latency_ms ?? item.avg_latency ?? item.latency_ms),
          p95_latency_ms: toNumber(item.p95_latency_ms ?? item.p95_latency ?? item.p95_ms),
          success_rate: normalizedSuccessRate,
          avg_cost_usd: toNumber(item.avg_cost_usd ?? item.avg_cost ?? item.cost_usd),
          samples: Math.max(0, Math.floor(toNumber(item.samples ?? item.requests ?? item.count))),
        } satisfies ModelPerformance
      })
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
