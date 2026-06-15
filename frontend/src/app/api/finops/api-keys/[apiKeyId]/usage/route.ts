import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

export async function GET(
  request: Request,
  context: { params: Promise<{ apiKeyId: string }> }
) {
  const auth = await requireAdminBearer(request)
  if ('response' in auth) return auth.response

  try {
    const { apiKeyId } = await context.params
    const { searchParams } = new URL(request.url)

    const qs = new URLSearchParams()
    const windowHours = searchParams.get('window_hours') || '720'
    const limit = searchParams.get('limit') || '50'
    const offset = searchParams.get('offset') || '0'

    qs.set('window_hours', windowHours)
    qs.set('limit', limit)
    qs.set('offset', offset)

    const data = await gatewayAdminFetch(
      `/admin/api-keys/${encodeURIComponent(apiKeyId)}/usage?${qs.toString()}`,
      { requestAuthToken: auth.token }
    )

    return NextResponse.json({ data })
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }
    return NextResponse.json({ error: 'Failed to fetch API key drilldown' }, { status: 500 })
  }
}
