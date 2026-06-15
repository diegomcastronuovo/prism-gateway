import { assertFinopsUnauthorized } from '@/lib/finops-fetch'

export type ApiKeyUsageRow = {
  api_key_id: string
  api_key_name: string
  tenant_id: string
  requests: number
  avg_cost_per_request_effective: number | null
  success_rate: number | null
  avg_latency_ms: number | null
  top_model: string | null
  top_provider: string | null
  last_seen_at: string | null
}

export type ApiKeyUsageListResponse = {
  data: ApiKeyUsageRow[]
  pagination: { limit: number; offset: number; returned: number; total: number }
}

export type RecentRequestRow = {
  request_id: string
  timestamp: string
  model: string
  provider: string
  latency_ms: number
  status: string
}

export type ApiKeyUsageDrilldown = {
  api_key_id: string
  requests_by_model: {
    model: string
    requests: number
    effective_spend: number
    avg_cost_per_request_effective: number | null
  }[]
  requests_by_provider: { provider: string; requests: number }[]
  traffic_over_time: { bucket: string; requests: number; errors: number }[]
  recent_requests: RecentRequestRow[]
}

export async function fetchApiKeysUsage(
  windowHours: number,
  limit = 200,
  offset = 0
): Promise<ApiKeyUsageListResponse> {
  const qs = new URLSearchParams()
  qs.set('window_hours', String(windowHours))
  qs.set('limit', String(limit))
  qs.set('offset', String(offset))

  const resp = await fetch(`/api/finops/api-keys/usage?${qs.toString()}`)
  if (!resp.ok) {
    await assertFinopsUnauthorized(resp)
    return { data: [], pagination: { limit, offset, returned: 0, total: 0 } }
  }

  const raw = (await resp.json()) as Record<string, unknown>
  const payload = raw?.data ?? raw
  const apiKeysArray: Record<string, unknown>[] = Array.isArray(payload)
    ? (payload as Record<string, unknown>[])
    : Array.isArray((payload as { data?: unknown } | null)?.data)
      ? (payload as { data: Record<string, unknown>[] }).data
      : []
  const data: ApiKeyUsageRow[] = apiKeysArray.map((row: Record<string, unknown>) => ({
    api_key_id: String(row.api_key_id ?? ''),
    api_key_name: String(row.api_key_name ?? ''),
    tenant_id: String(row.tenant_id ?? ''),
    requests: Number(row.requests ?? 0),
    avg_cost_per_request_effective:
      row.avg_cost_per_request_effective == null ? null : Number(row.avg_cost_per_request_effective),
    success_rate: row.success_rate == null ? null : Number(row.success_rate),
    avg_latency_ms: row.avg_latency_ms == null ? null : Number(row.avg_latency_ms),
    top_model: row.top_model == null ? null : String(row.top_model),
    top_provider: row.top_provider == null ? null : String(row.top_provider),
    last_seen_at: row.last_seen == null ? (row.last_seen_at == null ? null : String(row.last_seen_at)) : String(row.last_seen),
  }))
  const paginationRaw =
    payload && typeof payload === 'object' && !Array.isArray(payload) && 'pagination' in payload
      ? (payload as { pagination?: Record<string, unknown> }).pagination ?? {}
      : (raw.pagination as Record<string, unknown> | undefined) ?? {}
  return {
    data,
    pagination: {
      limit: Number(paginationRaw.limit ?? limit),
      offset: Number(paginationRaw.offset ?? offset),
      returned: Number(paginationRaw.returned ?? data.length),
      total: Number(paginationRaw.total ?? data.length),
    },
  }
}

export async function fetchAllApiKeysUsage(windowHours: number): Promise<ApiKeyUsageRow[]> {
  const limit = 200
  let offset = 0
  const rows: ApiKeyUsageRow[] = []
  const maxPages = 500
  for (let page = 0; page < maxPages; page++) {
    const resp = await fetchApiKeysUsage(windowHours, limit, offset)
    const batch = resp.data
    if (batch.length === 0) break
    rows.push(...batch)
    offset += batch.length
    if (batch.length < limit) break
  }
  return rows
}

export type FetchApiKeyDrilldownOptions = {
  /** Larger values pull more recent_requests rows for aggregation (default 50). */
  limit?: number
}

export async function fetchApiKeyDrilldown(
  apiKeyId: string,
  windowHours: number,
  opts?: FetchApiKeyDrilldownOptions
): Promise<ApiKeyUsageDrilldown | null> {
  const limit = opts?.limit ?? 50
  const qs = new URLSearchParams()
  qs.set('window_hours', String(windowHours))
  qs.set('limit', String(limit))
  qs.set('offset', '0')
  const resp = await fetch(`/api/finops/api-keys/${encodeURIComponent(apiKeyId)}/usage?${qs.toString()}`)
  if (!resp.ok) {
    await assertFinopsUnauthorized(resp)
    return null
  }
  const raw = await resp.json()
  const payload = raw?.data ?? raw
  const requestsByModelRaw = Array.isArray(payload?.requests_by_model) ? payload.requests_by_model : []
  const requestsByProviderRaw = Array.isArray(payload?.requests_by_provider) ? payload.requests_by_provider : []
  const trafficOverTimeRaw = Array.isArray(payload?.traffic_over_time) ? payload.traffic_over_time : []
  const recentRequestsRaw = Array.isArray(payload?.recent_requests) ? payload.recent_requests : []
  return {
    api_key_id: String(payload?.api_key_id ?? apiKeyId),
    requests_by_model: requestsByModelRaw.map((row: Record<string, unknown>) => ({
      model: String(row.model ?? '-'),
      requests: Number(row.requests ?? 0),
      effective_spend:
        row.effective_spend == null
          ? Number(row.avg_cost_per_request_effective ?? 0) * Number(row.requests ?? 0)
          : Number(row.effective_spend),
      avg_cost_per_request_effective:
        row.avg_cost_per_request_effective == null ? null : Number(row.avg_cost_per_request_effective),
    })),
    requests_by_provider: requestsByProviderRaw.map((row: Record<string, unknown>) => ({
      provider: String(row.provider ?? '-'),
      requests: Number(row.requests ?? 0),
    })),
    traffic_over_time: trafficOverTimeRaw.map((row: Record<string, unknown>) => ({
      bucket: String(row.bucket ?? ''),
      requests: Number(row.requests ?? 0),
      errors: Number(row.errors ?? 0),
    })),
    recent_requests: recentRequestsRaw.map((row: Record<string, unknown>) => ({
      request_id: String(row.request_id ?? ''),
      timestamp: String(row.timestamp ?? ''),
      model: String(row.model ?? '-'),
      provider: String(row.provider ?? '-'),
      latency_ms: Number(row.latency_ms ?? 0),
      status: String(row.status ?? ''),
    })),
  }
}
