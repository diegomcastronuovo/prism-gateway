import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

type RequestRow = {
  request_id?: string
  tenant_id?: string
  model?: string
  provider?: string
  status?: string
  latency_ms?: number
  strategy?: string
  fallback_used?: boolean
  cache_hit?: boolean
  error_type?: string
  decision_reason?: string
  created?: number
  route_group?: string
  [key: string]: unknown
}

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const windowHours = searchParams.get('window_hours') || '24'
    const requestedLimit = Number(searchParams.get('limit') || '200')
    const limit = Math.min(200, Math.max(1, Number.isFinite(requestedLimit) ? requestedLimit : 200))

    const statsParams = new URLSearchParams({
      window_hours: windowHours,
    })

    const requestsParams = new URLSearchParams({
      window_hours: windowHours,
      limit: String(limit),
    })

    const passthroughFilters = ['tenant_id', 'provider', 'status']
    for (const key of passthroughFilters) {
      const value = searchParams.get(key)
      if (value) {
        statsParams.set(key, value)
        requestsParams.set(key, value)
      }
    }

    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const [statsData, requestsData] = await Promise.all([
      gatewayAdminFetch(`/admin/requests/stats?${statsParams.toString()}`, { requestAuthToken: auth.token }),
      gatewayAdminFetch(`/admin/requests?${requestsParams.toString()}`, { requestAuthToken: auth.token }),
    ])

    const summary = statsData?.summary || {}
    const providerHealth = Array.isArray(statsData?.provider_health) ? statsData.provider_health : []
    const statusBreakdown = statsData?.status_breakdown || {}
    const trafficOverTime = Array.isArray(statsData?.traffic_over_time) ? statsData.traffic_over_time : []

    const rows: RequestRow[] = Array.isArray(requestsData?.data) ? requestsData.data : []

    const decisions = rows.map((item, index) => ({
      id: String(item.request_id ?? `req-${index}`),
      timestamp: item.created ? new Date(item.created * 1000).toISOString() : '',
      tenant_id: String(item.tenant_id ?? ''),
      request_id: String(item.request_id ?? ''),
      selected_model: String(item.model ?? ''),
      provider: String(item.provider ?? ''),
      strategy: String(item.strategy ?? ''),
      route_group: item.route_group ? String(item.route_group) : undefined,
      fallback_used: Boolean(item.fallback_used),
      status: String(item.status ?? ''),
      latency_ms: Number(item.latency_ms ?? 0),
      cache_status: item.cache_hit ? 'hit' : 'miss',
      decision_reason: item.decision_reason ? String(item.decision_reason) : undefined,
      error_type: item.error_type ? String(item.error_type) : undefined,
    }))

    const strategyMap = new Map<string, number>()
    const modelMap = new Map<string, number>()
    const routeGroupMap = new Map<string, number>()

    for (const decision of decisions) {
      if (decision.strategy) {
        strategyMap.set(decision.strategy, (strategyMap.get(decision.strategy) || 0) + 1)
      }
      if (decision.selected_model) {
        modelMap.set(decision.selected_model, (modelMap.get(decision.selected_model) || 0) + 1)
      }
      if (decision.route_group) {
        routeGroupMap.set(decision.route_group, (routeGroupMap.get(decision.route_group) || 0) + 1)
      }
    }

    const strategy_distribution = Array.from(strategyMap.entries()).map(([strategy, count]) => ({
      strategy,
      count,
    }))
    const model_distribution = Array.from(modelMap.entries()).map(([model, count]) => ({
      model,
      count,
    }))
    const route_group_distribution = Array.from(routeGroupMap.entries()).map(([route_group, count]) => ({
      route_group,
      count,
    }))

    const metrics = {
      total_routed_requests: Number(summary.total_requests ?? 0),
      smart_routing_usage_pct: null,
      fallback_usage_pct: summary.fallback_rate != null ? summary.fallback_rate * 100 : null,
      avg_latency_ms: Number(summary.avg_latency_ms ?? 0),
      success_rate: summary.success_rate != null ? summary.success_rate * 100 : null,
      cache_hit_rate: summary.cache_hit_rate != null ? summary.cache_hit_rate * 100 : null,
    }

    return NextResponse.json({
      metrics,
      decisions,
      strategy_distribution,
      route_group_distribution,
      model_distribution,
      provider_health: providerHealth,
      status_breakdown: statusBreakdown,
      traffic_over_time: trafficOverTime,
      summary,
      window_hours: Number(statsData?.window_hours ?? windowHours),
    })
  } catch (error) {
    console.error('Routing insights error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch routing insights' },
      { status: 500 }
    )
  }
}
