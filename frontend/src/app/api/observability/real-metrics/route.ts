import { NextResponse } from 'next/server'
import { getAdminAuthToken, gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'

type ModelStatRow = {
  requests?: number
  successes?: number
  avg_latency_ms?: number
}

type Accumulator = {
  requests: number
  successes: number
  latencyWeightedSum: number
}

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id') || 'tenant_a'
    const month = searchParams.get('month') || new Date().toISOString().slice(0, 7)
    const windowDays = searchParams.get('window_days') || '7'
    const token = await getAdminAuthToken(request)

    // Fetch from real backend endpoints
    const [usageSummary, modelsStats] = await Promise.all([
      gatewayAdminFetch(`/admin/tenants/${tenantId}/usage/summary?month=${month}`, {
        requestAuthToken: token,
      }),
      gatewayAdminFetch(`/admin/tenants/${tenantId}/models/stats?window_days=${windowDays}`, {
        requestAuthToken: token,
      }),
    ])

    const statsRows: ModelStatRow[] = Array.isArray(modelsStats?.stats)
      ? modelsStats.stats
      : []

    const totals = statsRows.reduce<Accumulator>(
      (acc, row) => {
        const requests = Number(row.requests ?? 0)
        const successes = Number(row.successes ?? 0)
        const avgLatency = Number(row.avg_latency_ms ?? 0)

        const safeRequests = Number.isFinite(requests) ? requests : 0
        const safeSuccesses = Number.isFinite(successes) ? successes : 0
        const safeAvgLatency = Number.isFinite(avgLatency) ? avgLatency : 0

        return {
          requests: acc.requests + safeRequests,
          successes: acc.successes + safeSuccesses,
          latencyWeightedSum: acc.latencyWeightedSum + safeAvgLatency * safeRequests,
        }
      },
      { requests: 0, successes: 0, latencyWeightedSum: 0 }
    )

    const successRate =
      totals.requests > 0 ? (totals.successes / totals.requests) * 100 : null
    const avgLatencyMs =
      totals.requests > 0 ? totals.latencyWeightedSum / totals.requests : null

    return NextResponse.json({
      total_requests: usageSummary.total_requests ?? null,
      total_cost: usageSummary.total_cost_usd ?? null,
      success_rate: successRate,
      avg_latency_ms: avgLatencyMs,
    })
  } catch (error) {
    console.error('Real metrics error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch real metrics' },
      { status: 500 }
    )
  }
}
