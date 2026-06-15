import { NextResponse } from 'next/server'
import { getAdminAuthToken, gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const windowHours = searchParams.get('window_hours') || '24'
    const bucket = searchParams.get('bucket') || 'hour'
    const token = await getAdminAuthToken(request)

    const data = await gatewayAdminFetch(
      `/admin/requests/stats?window_hours=${windowHours}&bucket=${bucket}`,
      { requestAuthToken: token }
    )

    const summary = data?.summary ?? {}
    const trafficOverTime = Array.isArray(data?.traffic_over_time) ? data.traffic_over_time : []
    const providerHealth = Array.isArray(data?.provider_health) ? data.provider_health : []
    const statusBreakdown = data?.status_breakdown ?? {}
    const backendWindowHours = data?.window_hours

    // Preserve existing transformed fields for current consumers
    const transformed = {
      metrics: {
        total_requests_24h: summary.total_requests == null ? null : Number(summary.total_requests),
        success_rate: summary.success_rate == null ? null : Number(summary.success_rate) * 100,
        avg_latency_ms: summary.avg_latency_ms == null ? null : Number(summary.avg_latency_ms),
        cache_hit_rate: null,
        fallback_rate: summary.fallback_rate == null ? null : Number(summary.fallback_rate) * 100,
        fallback_count: summary.fallback_requests == null ? null : Number(summary.fallback_requests),
        total_cost_24h: null,
      },
      traffic_data: trafficOverTime.map((item: Record<string, unknown>) => ({
        time_bucket: String(item.bucket ?? ''),
        requests: Number(item.requests ?? 0),
        successes: Number(item.successes ?? 0),
        errors: Number(item.errors ?? 0),
      })),
      provider_health: providerHealth.map((item: Record<string, unknown>) => ({
        provider: String(item.provider ?? ''),
        success_rate: Number(item.success_rate ?? 0) * 100,
        avg_latency: Number(item.avg_latency_ms ?? 0),
        total_requests: Number(item.total_requests ?? 0),
      })),
    }

    // Also include raw/aggregated backend fields to align with consumers expecting stats shape
    return NextResponse.json({
      ...transformed,
      summary,
      status_breakdown: statusBreakdown,
      traffic_over_time: trafficOverTime,
      window_hours: typeof backendWindowHours === 'number' ? backendWindowHours : Number(windowHours),
      raw_backend: data,
    })
  } catch (error) {
    console.error('Observability metrics error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    // Provide more actionable error context
    try {
      const { searchParams } = new URL(request.url)
      const windowHours = searchParams.get('window_hours') || '24'
      const bucket = searchParams.get('bucket') || 'hour'
      return NextResponse.json(
        {
          error: 'Failed to fetch observability metrics',
          details: `Backend /admin/requests/stats?window_hours=${windowHours}&bucket=${bucket} request failed`,
        },
        { status: 500 }
      )
    } catch {
      return NextResponse.json(
        { error: 'Failed to fetch observability metrics' },
        { status: 500 }
      )
    }
  }
}
