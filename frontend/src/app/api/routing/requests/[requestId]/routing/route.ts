import { NextRequest, NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function GET(
  request: NextRequest,
  { params }: { params: { requestId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const requestId = params.requestId
    const url = `/admin/requests/${encodeURIComponent(requestId)}/routing`

    const response = await gatewayAdminFetch(url, {
      method: 'GET',
      requestAuthToken: auth.token,
    })

    return NextResponse.json(response)
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: { message: error.message, type: 'gateway_error' } },
        { status: error.statusCode || 500 }
      )
    }
    console.error('Error fetching routing snapshot:', error)
    return NextResponse.json(
      { error: { message: 'Failed to fetch routing snapshot', type: 'internal_error' } },
      { status: 500 }
    )
  }
}
