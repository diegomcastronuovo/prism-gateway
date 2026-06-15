import { NextResponse } from 'next/server'
import { getAdminAuthToken, gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const scope = searchParams.get('scope')
    const tenantId = searchParams.get('tenant_id')
    const limit = searchParams.get('limit') || '50'
    const offset = searchParams.get('offset') || '0'

    const token = await getAdminAuthToken(request)

    const qs = new URLSearchParams()
    if (scope) {
      qs.set('scope', scope)
    }
    if (tenantId) {
      qs.set('tenant_id', tenantId)
    }
    qs.set('limit', limit)
    qs.set('offset', offset)

    const data = await gatewayAdminFetch(`/admin/config/history?${qs.toString()}`, {
      requestAuthToken: token,
    })

    return NextResponse.json(data)
  } catch (error) {
    console.error('Config history error:', error)

    if (error instanceof GatewayAdminError) {
      const errorType = (error.details as Record<string, unknown> | undefined)?.type || 'gateway_error'
      return NextResponse.json(
        { error: { message: error.message, type: errorType } },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: { message: 'Failed to fetch config history', type: 'internal_error' } },
      { status: 500 }
    )
  }
}
