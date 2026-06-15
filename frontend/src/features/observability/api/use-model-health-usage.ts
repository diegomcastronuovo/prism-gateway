import { useQuery } from '@tanstack/react-query'
import type { Model } from '@/features/models/api/use-models'
import type { ModelPerformance } from '@/features/observability/types/real-backend'
import type { ApiKeyUsageDrilldown } from '@/features/observability/api/fetch-api-keys-usage'
import { fetchAllApiKeysUsage, fetchApiKeyDrilldown } from '@/features/observability/api/fetch-api-keys-usage'

function toNumber(value: unknown, fallback = 0): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

/** Aggregated real-usage row for Model Health (SPEC_113). */
export type ModelHealthUsageRow = ModelPerformance & {
  effective_spend: number
  model_type: string
}

function normalizeModelType(raw: string | undefined): string {
  const t = (raw ?? '').trim().toLowerCase()
  if (!t) return 'llm'
  if (t === 'ml') return 'ml'
  if (t === 'embedding') return 'embedding'
  return 'llm'
}

/**
 * When API-key usage aggregation returns no rows (e.g. JWT-only traffic, finops empty, or drilldown
 * failures), fall back to the same benchmark endpoint as the main Observability model table so the
 * screen is not blank. Real usage remains preferred when it produces rows.
 */
async function fetchBenchmarkModelHealthRows(windowHours: number, models: Model[]): Promise<ModelHealthUsageRow[]> {
  const res = await fetch(`/api/observability/model-performance?window_hours=${windowHours}`, {
    credentials: 'include',
    cache: 'no-store',
  })
  if (!res.ok) return []

  const json = (await res.json()) as Record<string, unknown>
  const rawItems: Record<string, unknown>[] = Array.isArray(json)
    ? json
    : Array.isArray(json?.models)
      ? (json.models as Record<string, unknown>[])
      : Array.isArray(json?.items)
        ? (json.items as Record<string, unknown>[])
        : Array.isArray(json?.data)
          ? (json.data as Record<string, unknown>[])
          : []

  const catalogById = new Map(models.map((m) => [m.id, m]))

  return rawItems.map((item) => {
    const successRateRaw = toNumber(item.success_rate ?? item.successRate ?? 0)
    const normalizedSuccessRate = successRateRaw > 1 ? successRateRaw / 100 : successRateRaw
    const model = String(item.model ?? item.model_name ?? 'unknown')
    const samples = Math.max(0, Math.floor(toNumber(item.samples ?? item.requests ?? item.count)))
    const avg_cost_usd = toNumber(item.avg_cost_usd ?? item.avg_cost ?? item.cost_usd)

    return {
      model,
      provider: String(item.provider ?? 'unknown'),
      avg_latency_ms: toNumber(item.avg_latency_ms ?? item.avg_latency ?? item.latency_ms),
      p95_latency_ms: toNumber(item.p95_latency_ms ?? item.p95_latency ?? item.p95_ms),
      success_rate: normalizedSuccessRate,
      avg_cost_usd,
      samples,
      effective_spend: avg_cost_usd * samples,
      model_type: normalizeModelType(catalogById.get(model)?.type),
    }
  })
}

function isOkStatus(status: string): boolean {
  return String(status).trim().toLowerCase() === 'ok'
}

function percentile95Sorted(sorted: number[]): number {
  if (sorted.length === 0) return 0
  const idx = Math.ceil(0.95 * sorted.length) - 1
  return sorted[Math.max(0, idx)]
}

async function fetchModelsCatalog(): Promise<Model[]> {
  const res = await fetch('/api/models', { credentials: 'include', cache: 'no-store' })
  if (!res.ok) return []
  const data = (await res.json()) as { data?: Model[] }
  return Array.isArray(data.data) ? data.data : []
}

export function aggregateModelHealthFromDrilldowns(
  drilldowns: ApiKeyUsageDrilldown[],
  models: Model[]
): ModelHealthUsageRow[] {
  const catalogById = new Map(models.map((m) => [m.id, m]))

  const aggRequests = new Map<string, { requests: number; effective_spend: number }>()
  for (const dd of drilldowns) {
    for (const row of dd.requests_by_model) {
      const m = row.model
      if (!m || m === '-') continue
      const prev = aggRequests.get(m) ?? { requests: 0, effective_spend: 0 }
      prev.requests += row.requests
      prev.effective_spend += row.effective_spend
      aggRequests.set(m, prev)
    }
  }

  const recentByModel = new Map<string, ApiKeyUsageDrilldown['recent_requests']>()
  const seenRequestIds = new Set<string>()
  for (const dd of drilldowns) {
    for (const r of dd.recent_requests) {
      if (r.request_id && seenRequestIds.has(r.request_id)) continue
      if (r.request_id) seenRequestIds.add(r.request_id)
      const m = r.model
      if (!m || m === '-') continue
      const list = recentByModel.get(m) ?? []
      list.push(r)
      recentByModel.set(m, list)
    }
  }

  const providerVotes = new Map<string, Map<string, number>>()
  for (const [model, list] of Array.from(recentByModel.entries())) {
    const provMap = new Map<string, number>()
    for (const r of list) {
      const p = r.provider || 'unknown'
      provMap.set(p, (provMap.get(p) ?? 0) + 1)
    }
    providerVotes.set(model, provMap)
  }

  const rows: ModelHealthUsageRow[] = []
  for (const [model, sums] of Array.from(aggRequests.entries())) {
    if (sums.requests <= 0) continue

    const cat = catalogById.get(model)
    const votes = providerVotes.get(model)
    let provider = cat?.provider ?? 'unknown'
    if ((provider === 'unknown' || !provider) && votes && votes.size > 0) {
      provider = Array.from(votes.entries()).sort((a, b) => b[1] - a[1])[0][0]
    }

    const recentList = recentByModel.get(model) ?? []
    const latencies = recentList.map((r) => r.latency_ms).filter((n) => Number.isFinite(n) && n >= 0)
    const sortedLat = [...latencies].sort((a, b) => a - b)
    const avg_latency_ms =
      latencies.length > 0 ? latencies.reduce((a, b) => a + b, 0) / latencies.length : 0
    const p95_latency_ms = percentile95Sorted(sortedLat)

    let success_rate = 1
    if (recentList.length > 0) {
      const ok = recentList.filter((r) => isOkStatus(r.status)).length
      success_rate = ok / recentList.length
    }

    const avg_cost_usd = sums.effective_spend / sums.requests

    rows.push({
      model,
      provider,
      avg_latency_ms,
      p95_latency_ms,
      success_rate,
      avg_cost_usd,
      samples: sums.requests,
      effective_spend: sums.effective_spend,
      model_type: normalizeModelType(cat?.type),
    })
  }

  rows.sort((a, b) => a.model.localeCompare(b.model))
  return rows
}

/**
 * Real production Model Health from API key usage drill-downs (SPEC_113).
 * Does not use /admin/benchmarks/models.
 */
export function useModelHealthUsage(windowHours: number, enabled = true) {
  return useQuery<ModelHealthUsageRow[]>({
    queryKey: ['model-health-usage', windowHours],
    queryFn: async () => {
      const models = await fetchModelsCatalog()
      const apiKeys = await fetchAllApiKeysUsage(windowHours)
      const drilldowns = await Promise.all(
        apiKeys.map((k) =>
          // Same default limit as main Observability (50); larger limits may be rejected by the gateway.
          fetchApiKeyDrilldown(k.api_key_id, windowHours, { limit: 50 }).catch(() => null)
        )
      )
      const ok = drilldowns.filter((d): d is ApiKeyUsageDrilldown => d !== null)
      const usageRows = aggregateModelHealthFromDrilldowns(ok, models)
      if (usageRows.length > 0) return usageRows

      return fetchBenchmarkModelHealthRows(windowHours, models)
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
