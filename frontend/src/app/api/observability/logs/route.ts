import { NextResponse } from 'next/server'
import { getAdminAuthToken, gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const requestedLimit = Number(searchParams.get('limit') || '50')
    const requestedOffset = Number(searchParams.get('offset') || '0')
    const limit = Math.min(200, Math.max(1, Number.isFinite(requestedLimit) ? requestedLimit : 50))
    const offset = Math.max(0, Number.isFinite(requestedOffset) ? requestedOffset : 0)
    const token = await getAdminAuthToken(request)

    const data = await gatewayAdminFetch(`/admin/requests/recent?limit=${limit}&offset=${offset}`, {
      requestAuthToken: token,
    })

    const rows = Array.isArray(data?.data) ? data.data : []
    const logs = rows.map((item: Record<string, unknown>) => {
      const cache = (item.cache ?? {}) as Record<string, unknown>
      const cacheStatus = String(cache.status ?? '').toLowerCase()
      return {
        timestamp: String(item.timestamp ?? ''),
        tenant_id: String(item.tenant_id ?? ''),
        model: String(item.model ?? ''),
        provider: String(item.provider ?? ''),
        latency_ms: Number(item.latency_ms ?? 0),
        status: String(item.status ?? ''),
        fallback_used: Boolean(item.fallback_used),
        cache_status: cacheStatus || undefined,
      }
    })

    const pagination = {
      limit: Number(data?.pagination?.limit ?? limit),
      offset: Number(data?.pagination?.offset ?? offset),
      returned: Number(data?.pagination?.returned ?? logs.length),
      total: Number(data?.pagination?.total ?? logs.length),
    }

    return NextResponse.json({ logs, total: pagination.total, pagination })
  } catch (error) {
    console.error('Observability logs error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch observability logs' },
      { status: 500 }
    )
  }
}
