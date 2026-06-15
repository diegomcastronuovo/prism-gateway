/**
 * FinOps API routes forward the admin Bearer token; 401/403 must surface as errors
 * so the UI does not show empty placeholder data (RBAC).
 */
export async function assertFinopsUnauthorized(resp: Response): Promise<void> {
  if (resp.status !== 401 && resp.status !== 403) return
  const body = (await resp.json().catch(() => ({}))) as {
    error?: string | { message?: string }
  }
  const msg =
    typeof body?.error === 'string'
      ? body.error
      : typeof body?.error === 'object' && body.error && typeof body.error.message === 'string'
        ? body.error.message
        : resp.status === 403
          ? 'Access denied'
          : 'Unauthorized'
  const e = new Error(msg) as Error & { status?: number }
  e.status = resp.status
  throw e
}

/** Per-tenant request/traffic stats (FinOps dashboard / analytics). */
export type TenantRequestStatsRow = {
  tenant_id: string
  total_requests: number
  successful_requests: number
  success_rate: number | null
  avg_latency_ms: number | null
}

function parseTenantStatsPayload(tenantId: string, payload: unknown): TenantRequestStatsRow {
  const row = (payload as { data?: Record<string, unknown> })?.data ?? {}
  const traffic = Array.isArray(row.traffic_over_time) ? row.traffic_over_time : []
  const trafficTotals = traffic.reduce(
    (acc: { requests: number; successes: number }, item: Record<string, unknown>) => {
      acc.requests += Number(item.requests ?? 0)
      acc.successes += Number(item.successes ?? 0)
      return acc
    },
    { requests: 0, successes: 0 }
  )
  const totalRequests = trafficTotals.requests > 0 ? trafficTotals.requests : Number(row.total_requests ?? 0)
  const successfulRequests = trafficTotals.requests > 0
    ? trafficTotals.successes
    : Math.round(Number(row.total_requests ?? 0) * Number(row.success_rate ?? 0))
  const computedSuccessRate = totalRequests > 0 ? successfulRequests / totalRequests : null
  return {
    tenant_id: tenantId,
    total_requests: totalRequests,
    successful_requests: successfulRequests,
    success_rate: computedSuccessRate,
    avg_latency_ms: row.avg_latency_ms == null ? null : Number(row.avg_latency_ms),
  }
}

/**
 * Loads request stats per tenant. 401/403 on a tenant is NOT fatal: that tenant id is listed in
 * `forbiddenTenantIds` so the UI can hide budget/usage for tenants the user must not see
 * (avoids showing stale React Query cache or mixing allowed + denied rows).
 * Other HTTP errors fail the whole batch (surface as query error).
 */
export async function fetchTenantRequestStatsBatch(
  tenantIds: string[],
  windowHours: number,
  credentials: RequestCredentials = 'include'
): Promise<{ rows: TenantRequestStatsRow[]; forbiddenTenantIds: string[] }> {
  const forbidden: string[] = []
  const rows: TenantRequestStatsRow[] = []

  for (const tenantId of tenantIds) {
    const resp = await fetch(
      `/api/finops/tenant-request-stats?tenant_id=${encodeURIComponent(tenantId)}&window_hours=${encodeURIComponent(String(windowHours))}&bucket=hour`,
      { credentials }
    )
    if (resp.status === 401 || resp.status === 403) {
      forbidden.push(tenantId)
      continue
    }
    if (!resp.ok) {
      const errBody = await resp.json().catch(() => ({}))
      const msg =
        typeof errBody.error === 'string' ? errBody.error : `Request failed (${resp.status})`
      throw new Error(msg)
    }
    const data = await resp.json()
    rows.push(parseTenantStatsPayload(tenantId, data))
  }

  return { rows, forbiddenTenantIds: forbidden }
}
