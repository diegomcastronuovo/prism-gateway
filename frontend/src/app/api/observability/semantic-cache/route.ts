import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const qs = new URLSearchParams()
    const tenantId = searchParams.get('tenant_id')
    const limit = searchParams.get('limit')

    if (tenantId) qs.set('tenant_id', tenantId)
    if (limit) qs.set('limit', limit)

    const queryString = qs.toString()
    const endpoint = queryString
      ? `/admin/observability/semantic-cache?${queryString}`
      : '/admin/observability/semantic-cache'

    const data = await gatewayAdminFetch(endpoint, { requestAuthToken: auth.token })

    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic cache error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch semantic cache analytics' },
      { status: 500 }
    )
  }
}
