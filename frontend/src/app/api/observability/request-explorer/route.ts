import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

type RecentRequestRow = {
  request_id?: string
  created?: number | string
  timestamp?: string
  created_at?: string
  started_at?: string
  completed_at?: string
  tenant_id?: string
  model?: string
  provider?: string
  strategy?: string
  latency_ms?: number
  status?: string
  fallback_used?: boolean
  attempt?: number
  cache?: {
    status?: string
  }
  [key: string]: unknown
}

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const requestedLimit = Number(searchParams.get('limit') || '50')
    const requestedOffset = Number(searchParams.get('offset') || '0')
    const limit = Math.min(200, Math.max(1, Number.isFinite(requestedLimit) ? requestedLimit : 50))
    const offset = Math.max(0, Number.isFinite(requestedOffset) ? requestedOffset : 0)

    const params = new URLSearchParams({
      limit: String(limit),
      offset: String(offset),
    })

    const passthroughFilters = ['tenant_id', 'model', 'provider', 'status', 'fallback_used', 'workflow_id', 'conversation_id']
    for (const key of passthroughFilters) {
      const value = searchParams.get(key)
      if (value) params.set(key, value)
    }

    // Map FE time_range to backend window_hours
    const timeRange = searchParams.get('time_range') || '24h'
    const windowMap: Record<string, number> = { '1h': 1, '24h': 24, '7d': 168, '30d': 720 }
    const windowHours = windowMap[timeRange] ?? 24
    params.set('window_hours', String(windowHours))
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const data = await gatewayAdminFetch(`/admin/requests/recent?${params.toString()}`, {
      requestAuthToken: auth.token,
    })
    const rows: RecentRequestRow[] = Array.isArray(data?.data) ? data.data : []

    const requests = rows.map((item, index) => {
      const createdSeconds = Number(item.created)
      const createdTimestamp =
        Number.isFinite(createdSeconds) && createdSeconds > 0
          ? new Date(createdSeconds * 1000).toISOString()
          : undefined
      const resolvedTimestamp =
        createdTimestamp ??
        item.timestamp ??
        item.created_at ??
        item.started_at ??
        item.completed_at

      return ({
      id: String(item.request_id ?? `${offset + index}`),
      request_id: String(item.request_id ?? ''),
      timestamp: resolvedTimestamp ? String(resolvedTimestamp) : '',
      tenant_id: String(item.tenant_id ?? ''),
      model: String(item.model ?? ''),
      provider: String(item.provider ?? ''),
      strategy: String(item.strategy ?? ''),
      latency_ms: Number(item.latency_ms ?? 0),
      status: String(item.status ?? ''),
      fallback_used: Boolean(item.fallback_used),
      attempt: item.attempt == null ? undefined : Number(item.attempt),
      cache_status: item.cache?.status ? String(item.cache.status) : undefined,
      raw_request: item,
      // SPEC_66: Pass diagnostic fields
      decision_reason: item.decision_reason ? String(item.decision_reason) : undefined,
      error_type: item.error_type ? String(item.error_type) : undefined,
      metadata: item.metadata && typeof item.metadata === 'object' ? item.metadata as Record<string, unknown> : undefined,
      pii_webhook_request_decision: item.pii_webhook_request_decision ? String(item.pii_webhook_request_decision) : undefined,
      pii_webhook_response_decision: item.pii_webhook_response_decision ? String(item.pii_webhook_response_decision) : undefined,
      routing_snapshot: item.routing_snapshot && typeof item.routing_snapshot === 'object' ? item.routing_snapshot as Record<string, unknown> : undefined,
      decision_snapshot: item.decision_snapshot && typeof item.decision_snapshot === 'object' ? item.decision_snapshot as Record<string, unknown> : undefined,
      // SPEC_143: workflow/conversation context
      workflow_id:     item.workflow_id     ? String(item.workflow_id)     : undefined,
      conversation_id: item.conversation_id ? String(item.conversation_id) : undefined,
    })})

    const pagination = {
      limit: Number(data?.pagination?.limit ?? limit),
      offset: Number(data?.pagination?.offset ?? offset),
      returned: Number(data?.pagination?.returned ?? requests.length),
      total: Number(data?.pagination?.total ?? requests.length),
    }

    return NextResponse.json({ requests, total: pagination.total, pagination })
  } catch (error) {
    console.error('Request explorer error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch request explorer data' },
      { status: 500 }
    )
  }
}
