import { NextResponse } from 'next/server'
import { getAdminAuthToken, gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const params = new URLSearchParams()

    const from = searchParams.get('from')
    const to = searchParams.get('to')
    const tenantId = searchParams.get('tenant_id')
    const jwtSub = searchParams.get('jwt_sub')
    const status = searchParams.get('status')
    const requestedLimit = Number(searchParams.get('limit') || '50')
    const requestedOffset = Number(searchParams.get('offset') || '0')

    const limit = Math.min(200, Math.max(1, Number.isFinite(requestedLimit) ? requestedLimit : 50))
    const offset = Math.max(0, Number.isFinite(requestedOffset) ? requestedOffset : 0)

    if (from) params.set('from', from)
    if (to) params.set('to', to)
    if (tenantId) params.set('tenant_id', tenantId)
    if (jwtSub) params.set('jwt_sub', jwtSub)
    if (status) params.set('status', status)
    params.set('limit', String(limit))
    params.set('offset', String(offset))

    const token = await getAdminAuthToken(request)
    const data = await gatewayAdminFetch(`/admin/audit/requests?${params.toString()}`, {
      requestAuthToken: token,
    })

    const rows = Array.isArray((data as any)?.data) ? (data as any).data : []
    const pagination = (data as any)?.pagination ?? {
      limit,
      offset,
      returned: rows.length,
      total: offset + rows.length,
    }

    return NextResponse.json({ data: rows, pagination })
  } catch (error) {
    console.error('Audit logs error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json({ error: 'Failed to fetch audit logs' }, { status: 500 })
  }
}
