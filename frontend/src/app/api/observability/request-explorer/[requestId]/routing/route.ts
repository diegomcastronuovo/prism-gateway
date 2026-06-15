import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function GET(
  request: Request,
  { params }: { params: { requestId: string } }
) {
  try {
    const requestId = decodeURIComponent(params.requestId)
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const data = await gatewayAdminFetch(`/admin/requests/${encodeURIComponent(requestId)}/routing`, {
      requestAuthToken: auth.token,
    })
    return NextResponse.json(data)
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      if (error.statusCode === 404) {
        return NextResponse.json({ error: 'routing snapshot not found' }, { status: 404 })
      }

      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch request routing snapshot' },
      { status: 500 }
    )
  }
}
