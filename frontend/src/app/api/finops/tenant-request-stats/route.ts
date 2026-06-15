import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

export async function GET(request: Request) {
  const auth = await requireAdminBearer(request)
  if ('response' in auth) return auth.response

  try {
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    const windowHours = searchParams.get('window_hours') || '24'
    const bucket = searchParams.get('bucket') || 'hour'

    if (!tenantId) {
      return NextResponse.json({ error: 'tenant_id is required' }, { status: 400 })
    }

    const data = await gatewayAdminFetch(
      `/admin/requests/stats?tenant_id=${encodeURIComponent(tenantId)}&window_hours=${encodeURIComponent(windowHours)}&bucket=${encodeURIComponent(bucket)}`,
      { requestAuthToken: auth.token }
    )

    const summary = data?.summary ?? {}
    const trafficOverTime = Array.isArray(data?.traffic_over_time) ? data.traffic_over_time : []
    const providerHealth = Array.isArray(data?.provider_health) ? data.provider_health : []

    return NextResponse.json({
      data: {
        tenant_id: tenantId,
        total_requests: summary.total_requests == null ? 0 : Number(summary.total_requests),
        success_rate: summary.success_rate == null ? null : Number(summary.success_rate),
        avg_latency_ms: summary.avg_latency_ms == null ? null : Number(summary.avg_latency_ms),
        errors: summary.errors == null ? null : Number(summary.errors),
        traffic_over_time: trafficOverTime.map((item: Record<string, unknown>) => ({
          bucket: String(item.bucket ?? ''),
          requests: Number(item.requests ?? 0),
          successes: Number(item.successes ?? 0),
          errors: Number(item.errors ?? 0),
        })),
        provider_health: providerHealth.map((item: Record<string, unknown>) => ({
          provider: String(item.provider ?? ''),
          requests: Number(item.total_requests ?? 0),
          success_rate: Number(item.success_rate ?? 0),
          avg_latency_ms: Number(item.avg_latency_ms ?? 0),
        })),
      },
    })
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json({ error: 'Failed to fetch tenant request stats' }, { status: 500 })
  }
}
